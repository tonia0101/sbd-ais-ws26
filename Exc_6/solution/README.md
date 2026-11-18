
## Guide notes

- **Key:** Debezium uses the table's primary key (`{"id": 1}`) as the event key by default,
  so all changes to one row land in the same partition, in order.
- **REPLICA IDENTITY:** an audit log for compliance needs the full old row, because it has to
  record what a value changed *from*, not just what it changed to. The same applies to a
  service that undoes accidental changes. `REPLICA IDENTITY FULL` makes PostgreSQL write the
  whole old row into the WAL.
- **Tombstone:** a tombstone means the row with this key was deleted. A consumer that keeps a
  copy of the table (a cache, a search index, a vector index) removes the entry for that key,
  so the copy stays the same as the database.



# Exercise 6 - *Card Sentinel* - fraud detection on a system of record



Everything is in the folder `fraud/`:

| File | What | Change it? |
|---|---|---|
| `card_processor.py` | the payment path: commits batches of payments into the table `transactions` of database `mydb`; injects frauds | no |
| `models.py` | the data science team's models: `anomaly` (account takeover) and `velocity` (card testing) | no |
| `fraud_agent.py` | a fraud agent service that runs one model; complete, read it | no |
| `agents.py` | starts 1 anomaly and 3 velocity agents in one terminal | no |
| `fraud-connector.json` | the Debezium connector | **yes**, in Part 2 |
| `connector.py` | registers the connector, or replaces it after a change | no |
| `scoreboard.py` | plays case management: scores a run against the ground truth | no |

**The two models** (details at the top of `models.py`). Both are
**stateful per cardholder**: they remember something about every
cardholder, and only work if they see *all* of that cardholder's payments.

- `anomaly` (account takeover): remembers each cardholder's last 50 amounts
  and flags a payment far above the cardholder's average. 0.3 ms per payment.
- `velocity` (card testing): a thief tries a stolen card with many small
  payments (here: 12 within ~20 s). The model counts each cardholder's
  payments in the last 60 s and flags the 8th. 2 ms per payment.

**Requirements**

| | Requirement |
|---|---|
| R1 | **System of record untouched:** the payment path (`card_processor.py`) stays unchanged, never waits for a fraud check, and gets no extra query load. |
| R2 | **Every payment to every model:** each model gets every committed payment, none is missed. Adding another consumer (e.g. a training-data archive for the data science team) must not change what the others get. |
| R3 | **Correct at any scale:** each model catches ≥ 90 % of its frauds with few false alerts, *however many instances of it run*. |
| R4 | **Fresh:** an alert at most 5 s (p95) after the payment was committed, at full rate. |

R1 you argue in Part 1; the scoreboard measures R2–R4. Do not change the
files marked "no", and do not read `ground_truth.jsonl` in your code (in
real life, fraud is only confirmed after the chargeback).

## Part 1 - The decision (in the session)

There are four common ways to get every committed payment out of a
database and to the models (lecture: *Getting data out of a system of
record: four options* and *Why two writes are not one*):

![Four options. 1. Polling: the agents query PostgreSQL again and again (SELECT … WHERE id > last_id); it misses rows that commit out of id order and adds queries for every agent. 2. Dual write: the card processor commits to PostgreSQL (1) and then sends to Kafka (2); a crash after 1 and before 2 loses the event, and the payment path changes. 3. Outbox: the card processor writes the payment and an outbox row in one transaction, a relay publishes the outbox to Kafka; correct, but it changes the payment path, the best choice if you own the application. 4. Log-based CDC: Debezium reads PostgreSQL's WAL and publishes to Kafka; no code change, no queries, every commit in order; cost: the replication slot keeps WAL while the connector is down.](../images/part1-options.png)

The table compares them for this scenario:

| Option | How it works | What goes wrong here | What it costs the database |
|---|---|---|---|
| 1. **Polling** | the agents query the table again and again (`WHERE id > last_id`) | misses payments that commit out of `id` order, as the poller of *Line Guardian* did (R2); every agent adds queries on the system of record (R1) | query load, for every reader |
| 2. **Dual write** | the card processor also sends every payment to Kafka after its commit | changes the certified payment path (R1); a crash between the commit and the send loses the event, a send before the commit invents a payment that was rolled back (R2) | none, but the application changes |
| 3. **Outbox** | the card processor writes the payment and an "outbox" row in the *same* database transaction; a relay publishes the outbox to Kafka | correct and complete, but it also changes the payment path (R1) | one extra row per transaction |
| 4. **Log-based CDC** | Debezium reads PostgreSQL's write-ahead log (WAL), as in the Guide, and publishes every committed change | nothing for R1 or R2: no code change, no queries, every commit, in commit order | a *replication slot*: PostgreSQL keeps all WAL the connector has not read yet. If the connector stops for a weekend, the disk can fill up, and the database (and with it the payment path) stops |

Dual write is also the most common mistake in AI systems: an application
that writes a product to its database *and* to a vector index for RAG gets
the same lost or invented updates. CDC (or an outbox) is the usual fix.

**Your task (for the write-up):** one paragraph, 5–8 sentences. Which
option do you choose for this scenario, and why? What does it cost, and how
would you notice in production before it hurts? Which option would you
choose if your team *owned* the payment path?

**Answer:**
Choose option 4, log-based CDC because it is the only option that satisfies R1 and R2. Polling adds query load from every agent and misses payments that commit out of `id` order; dual write and outbox both require changing the certified payment path, and dual write can also lose a payment if the process crashes between the commit and the send to Kafka. CDC reads the committed changes from PostgreSQL's write-ahead log, so the payment code stays unchanged, there are no extra queries, and every commit arrives in commit order. The cost is the replication slot: PostgreSQL keeps all WAL that Debezium has not read yet, so if the connector stops (for example over a weekend), the WAL fills the disk and the database, and with it the payment path, stops. To notice this before something happens -> monitor the connector and task status (alert if not RUNNING), the amount of WAL retained by the slot (in pg_replication_slots), and the free disk space of the database server, with alerts well before the disk is full. If my team owned the payment path, I would choose the outbox, because writing the payment and the outbox row in the same transaction is correct and complete, and it doesn't depend on the database's WAL. 


## Part 2 - The pipeline in three runs (in the session)

### Run 1:

**Prediction**
Do all 3 velocity instances share the work, and does R4 pass?
 
- Only 1 of the 3 velocity instances will do the work, because Debezium creates the topic with 1 partition (one partition is read only by one instance per group).

- With only one partition `hash(key) mod 1` is always 0. Every event lands in that partition, whatever the key is.

- R4 will fail, because one instance handles only about 500 of the 1,000 payments/s, so the lag (queue) grows every second and alerts arrive later and later.

**Actual Run 1**

```
Run 1794998089: started 11:34:49, 220,253 transactions in 220 s (1,001/s), 25 frauds so far (still running)

Consumer groups on the CDC topic(s)   [R2]
  group                    topic                              partitions  members       lag
  anomaly-agent            fraud.public.transactions                   1        1     1,002
  velocity-agent           fraud.public.transactions                   1        3   106,776

Agent instances (telemetry of this run)   [R3, R4]
  instance         pid model      transactions  cardholders  out-of-order  lag (s)      seen
  ano-1          74460 anomaly         221,655       89,022             0      1.0       2s ago
  vel-1          74461 velocity         69,227       49,905             0    106.5       1s ago

Alerts on 'fraud-alerts' (committed, this run)

  velocity model (card testing)
    R3  frauds caught          4/12  (33%)   FAIL  (>= 90 %)
    R3  false alerts           0   PASS  (at most 10 % of the flagged transactions)
    R4  alert latency p50/p95  68.3 s / 101.8 s   FAIL  (p95 <= 5 s after the commit)
        duplicate alerts       0   (info: alerts case management got twice)

  anomaly model (account takeover)
    R3  frauds caught          12/12  (100%)   PASS  (>= 90 %)
    R3  false alerts           0   PASS  (at most 10 % of the flagged transactions)
    R4  alert latency p50/p95  1.0 s / 27.1 s   FAIL  (p95 <= 5 s after the commit)
        duplicate alerts       0   (info: alerts case management got twice)
```

- Debezium created the topic with 1 partition and within consumer group each partition is read by 1 instance, so only vel-1 got the work while vel-2 and vel-3 did not.

- R4 failed because vel-1 can process only around 400 payments/s, but 1,000 payments/s arrive, so the lag grew to 106.5 s and the alerts came far too late.

- R3 also failed, although the model itself works: the scoreboard counts a fraud as soon as its payments are committed, but vel-1 was 106.5 s behind. The payments of the most recent frauds were still waiting in the backlog, so vel-1 had not read them yet and could not raise an alert (only 4 of 12 caught).

### Run 2:

**Prediction**
Now all three instances work. Will the velocity model catch ≥ 90 % of the card-testing frauds (R3)? Think of the key you saw in Run 1.

- All three velocity instances will work, because the topic now has 6 partitions (2 per instance)

- R4 will pass, because 3 instances × ~400 payments/s = 1,200 payments/s, which is more than the 1,000 payments/s that arrive.

- R3 will fail, because the key is the payment `id`. Each payment of a cardholder has a different key, so the 12 test payments of a stolen card are spread over all partitions. Each instance sees only about 4 of them, which is below the threshold of 8, this means that no instance raises an alert.

**Actual Run 2:**

```
Run 1795001171: started 12:26:11, 395,395 transactions in 395 s (1,001/s), 47 frauds so far (still running)

Consumer groups on the CDC topic(s)   [R2]
  group                    topic                              partitions  members       lag
  anomaly-agent            fraud.public.transactions                   6        1     1,000
  velocity-agent           fraud.public.transactions                   6        3     1,289

Agent instances (telemetry of this run)   [R3, R4]
  instance         pid model      transactions  cardholders  out-of-order  lag (s)      seen
  ano-1          85484 anomaly         393,095       97,990             0      0.8       5s ago
  vel-1          85485 velocity        128,285       72,007             1      0.7       0s ago
  vel-2          85486 velocity        127,544       71,802             0      0.7       0s ago
  vel-3          85487 velocity        128,081       72,303             1      0.7       0s ago

Alerts on 'fraud-alerts' (committed, this run)

  velocity model (card testing)
    R3  frauds caught          2/23  (9%)   FAIL  (>= 90 %)
    R3  false alerts           0   PASS  (at most 10 % of the flagged transactions)
    R4  alert latency p50/p95  0.7 s / 0.7 s   PASS  (p95 <= 5 s after the commit)
        duplicate alerts       0   (info: alerts case management got twice)

  anomaly model (account takeover)
    R3  frauds caught          23/23  (100%)   PASS  (>= 90 %)
    R3  false alerts           0   PASS  (at most 10 % of the flagged transactions)
    R4  alert latency p50/p95  0.5 s / 0.8 s   PASS  (p95 <= 5 s after the commit)
        duplicate alerts       0   (info: alerts case management got twice)
```
- R4 passed, because with 6 partitions, each velocity instance got 2 partitions, so all 3 worked together and kept up with the 1,000 payments/s. The lag stayed at 0.7 s.

- As predicted R3 failed, only 2 of 23 frauds were caught. Because the key was payment `id`, every velocity instance kept its own count for the same cardholder, and no instance saw the whole attack.

- The two caught frauds must have been luck, meaning the spread is random so sometimes one instance received at least 8 of the 12 payments and therefore reached the threshold of 8 and was able to alert. 

The anomaly model still passed because its single instance read all 6 partitions and saw every payment. 

### Run 3 - the cardholder as the key

**Prediction**

What will the key of an event look like in the Kafka UI, and what changes on the scoreboard? 

- now it will have the `user_id` as a key instead and will look like {"user_id": ...}

- all payments of one cardholder have the same key, so they land in the same partition and are read by the same instance. 

- R3 will pass, because that instance counts all 12 payments and will then reach the threshold of 8.

- R4 will also pass, because they are fast enough together when each instance gets 2 partitions. So together they handle ~1,200 payments/s, more than 1,000 payments/s that arrive. 


**Actual Run**

``` 
Run 1795003349: started 13:02:29, 145,193 transactions in 145 s (1,001/s), 15 frauds so far (still running)

Consumer groups on the CDC topic(s)   [R2]
  group                    topic                              partitions  members       lag
  anomaly-agent            fraud.public.transactions                   6        1     4,104
  velocity-agent           fraud.public.transactions                   6        3     4,141

Agent instances (telemetry of this run)   [R3, R4]
  instance         pid model      transactions  cardholders  out-of-order  lag (s)      seen
  ano-1          93757 anomaly         146,295       76,849             0      1.0       0s ago
  vel-1          93758 velocity         43,044       24,249             0      1.0       3s ago
  vel-2          93759 velocity         42,288       23,880             0      1.0       3s ago
  vel-3          93760 velocity         42,370       24,021             0      0.9       3s ago

Alerts on 'fraud-alerts' (committed, this run)

  velocity model (card testing)
    R3  frauds caught          7/7  (100%)   PASS  (>= 90 %)
    R3  false alerts           0   PASS  (at most 10 % of the flagged transactions)
    R4  alert latency p50/p95  0.8 s / 1.1 s   PASS  (p95 <= 5 s after the commit)
        duplicate alerts       0   (info: alerts case management got twice)

  anomaly model (account takeover)
    R3  frauds caught          7/7  (100%)   PASS  (>= 90 %)
    R3  false alerts           0   PASS  (at most 10 % of the flagged transactions)
    R4  alert latency p50/p95  0.9 s / 1.0 s   PASS  (p95 <= 5 s after the commit)
        duplicate alerts       0   (info: alerts case management got twice)
```

- As predicted R3 passed with 7 out of 7 frauds caught. With `user_id` as the key, all payments of one cardholder landed in the same partition and were read by the same instance, so that instance counted the whole attack and reached the threshold of 8.

- R4 still passes (p95 1.1 s), because the 6 partitions were split between all 3 instances, so together they kept up with the 1,000 payments/s.

The key has to be an entity whose state the model keeps (or whatever the model remembers things about), here the cardholder (`user_id`) not the PK of the table (payment `id`).

### Closing Answer
Debezium chose two defaults that you had to change.
   What would have happened in production if nobody had checked them, and
   how could the team have noticed?

- Debezium chose 1 partition and the id as the key. If nobody had checked them, nothing would have crashed and there would have been no error message. The velocity model would simply have caught far too few frauds, so stolen cards would not have been blocked and thieves could keep using them. 

- The team could have noticed by monitoring the consumer lag and the alert latency of each agent (lag kept growing run 1), by checking that every instance in a consumer group has partitions assigned (vel-2 and vel-3 had assigned partitions []) and by comparing the number of alerts with the frauds confirmed later through chargebacks (run 2, the pipeline was fast but caught almost nothing). 







