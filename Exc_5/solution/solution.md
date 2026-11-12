# Exercise 5

**Requirements**

| | Requirement |
|---|---|
| R1 | **Scale:** 600 readings/s in the lab; the design must grow to 100,000 readings/s. |
| R2 | **Complete and ordered:** the model gets *every* reading of a machine, in `seq` order (the model counts `missing` and `out-of-order` readings). |
| R3 | **Fresh:** anomalies at most 5 s (p95) after the reading, at full rate. |
| R4 | **Independent consumers:** the raw readings also feed a training-data archive and a live dashboard. Adding a consumer must not slow down, change or break the others, and must not add load to PostgreSQL. |

## Stage A - Today's system
### Step 1: missing readings
Two Terminals in ```guardian/```, start detector first then a small plant:

```bash
    python3 baseline_detector.py
    python3 plant.py --sink postgres --machines 20
```
After ~90 s (the first fault starts at 40 s), stop the plant, wait until
   the detector's backlog is 0, and stop the detector (`Ctrl+C`). Compare the
   number of readings the detector read with the table:

   ```bash
   docker exec -it postgres psql -U postgres -d factory \
     -c "SELECT count(*) FROM readings WHERE run = (SELECT max(run) FROM readings);"
   ```
The detector:

```[baseline] stopped: read 18,004 readings, last id 21,900, 3,896 readings missing, 346 anomalies```

Table:

```
count 
-------
 21900
(1 row)
```

18,004 read + 3,896 missing = **21,900**

**Are there any missing readings?**
The missing readings are in the table. Detector has just never read them. 

**What does the detector see when it polls in between?**
Gateway 2 has committed, gateway 1 has not. So it sees only ids 115 to 129. 

**Which id does it remember?**
It stores ```last_id = 129```, the highest id it saw.

**What happens next?**
Gateway 1 commits ids 100-114. The next poll asks ```WHERE id > 129```, so ids 100-114 are skipped and they will never be read.

This happens because the id is assigned at INSERT but a row is visible only after COMMIT. In ```PostgresSink```, each gateway sleeps (0-120 ms) before committing, so a gateway with lower ids can commit later than ones with higher ids. The detector's ```last_id``` then jumps past the rows that are still uncommitted and ```id > last_id``` never returns them.

### Step 2: full plant (60 machines, 600 readings per sec.)
Start the detector again, then the full plant
   (`python3 plant.py --sink postgres`), and let both run for ~60 s. Watch
   the detector's lines **while the plant runs**: what happens to `delay`,
   `backlog` and `missing`, and why? Stop the detector right after the
   plant: afterwards it only works through its backlog.

**Detector:**

```
[baseline] polling table readings for id > 107700 (Ctrl+C to stop)
[baseline] read        0  backlog       0 rows  delay   0.0 s  missing      0  anomalies 0
[baseline] read        0  backlog       0 rows  delay   0.0 s  missing      0  anomalies 0
[baseline] read    1,185  backlog   1,455 rows  delay   2.5 s  missing      0  anomalies 0
[baseline] read    2,640  backlog   3,255 rows  delay   5.5 s  missing     15  anomalies 0
[baseline] read    4,640  backlog   5,670 rows  delay   9.5 s  missing     15  anomalies 0
[baseline] read    6,640  backlog   8,098 rows  delay  13.5 s  missing     15  anomalies 0
[baseline] read    8,640  backlog  10,515 rows  delay  17.6 s  missing     15  anomalies 0
[baseline] read   10,640  backlog  12,993 rows  delay  21.6 s  missing     15  anomalies 0
[baseline] read   12,640  backlog  15,419 rows  delay  25.8 s  missing     15  anomalies 0
[baseline] read   14,640  backlog  17,865 rows  delay  29.9 s  missing     15  anomalies 0
[baseline] read   16,640  backlog  20,335 rows  delay  33.9 s  missing     15  anomalies 0
[baseline] read   18,640  backlog  20,435 rows  delay  38.0 s  missing     15  anomalies 0
^C[baseline] stopped: read 18,640 readings, last id 126,869, 15 readings missing, 0 anomalies
```

**Plant:**

```
run 1794396975: 60 machines x 10 Hz = 600 readings/s -> postgres (Ctrl+C to stop)
  t=    5s      3,060 readings sent (611/s)
  t=   10s      6,060 readings sent (606/s)
  t=   15s      9,060 readings sent (604/s)
  t=   20s     12,060 readings sent (603/s)
  t=   25s     15,060 readings sent (602/s)
  t=   30s     18,060 readings sent (602/s)
  t=   35s     21,060 readings sent (602/s)
  t=   40s  fault starts on M-029 (repaired in 45 s)
  t=   40s     24,060 readings sent (601/s)  faulty: M-029
  t=   45s     27,060 readings sent (601/s)  faulty: M-029
  t=   50s     30,060 readings sent (601/s)  faulty: M-029
  t=   55s     33,060 readings sent (601/s)  faulty: M-029
  t=   60s     36,060 readings sent (601/s)  faulty: M-029
  t=   65s     39,060 readings sent (601/s)  faulty: M-029
^Crun 1794396975: sent 39,120 readings in 66 s
```

**What happens to `delay`, `backlog` & `missing`, and why?**


- `delay` grew from 2.5 s to 38 s (R3 allows 5 s), `backlog` grew past
20,000 rows and never shrank, `anomalies` stayed 0 although a fault
started at t=40 s.

- **Why:** the model costs 3 ms per reading, so one detector scores at most
about 333 readings/s (1 / 0.003 s). In my run it managed about 270 readings/s
(2,000 rows per output line, about 7.4 s per line), a bit less than the
theoretical value because polling the table also takes time. The plant sends
600/s, so the backlog grows by roughly 330 rows/s (600 - 270) and the
detector falls further behind every second.

- `missing` stopped growing at 15: the detector is now so far behind that it
reads old, long-committed rows, so no gaps appear. The problem is hidden,
not solved.

- **Consequence:** an alert comes far too late for the technician to stop the
machine, so R3 (and R2, because of the missing readings) is violated.


## Stage B - Decision record

Compare these two options against R1–R4 (add your own idea if you have
one):

1. keep PostgreSQL, but poll it better (e.g. several pollers, each for a
   part of the machines);
2. the gateways write every reading into Kafka (or another log-based
   platform) instead of into PostgreSQL.

For each option: which requirements does it meet, which are at risk? Then
decide, and estimate for your choice: how many detector instances does
the model (3 ms per reading) need for 600 readings/s, and for 100,000? A
small office that logs one temperature reading per minute is perfectly
served by option 1: where, roughly, is the tipping point?

Write it down as a short decision record (half a page: options, decision,
reasons, numbers). PostgreSQL stays the system of record for the machine
registry in any case.






### Comparison against R1-R4

Reference numbers: lab = 60 machines x 10 Hz = 600 readings/s (about 52 million rows/day).

Rollout = 2,000 machines x 50 Hz = 100,000 readings/s (about 8.6 billion rows/day).

| | Requirement |
|---|---|
| R1 | **Scale:** 600 readings/s in the lab; the design must grow to 100,000 readings/s. |
| R2 | **Complete and ordered:** the model gets *every* reading of a machine, in `seq` order (the model counts `missing` and `out-of-order` readings). |
| R3 | **Fresh:** anomalies at most 5 s (p95) after the reading, at full rate. |
| R4 | **Independent consumers:** the raw readings also feed a training-data archive and a live dashboard. Adding a consumer must not slow down, change or break the others, and must not add load to PostgreSQL. |

#### R1 Scale 
- **Option 1 (PostgreSQL + more pollers): at risk.**
  - One database has to ingest 100,000 INSERTs/s, maintain the indexes on every insert, and answer many polling SELECTs at the same time. It can only scale vertically.

  - The table grows by billions of rows per day. Deleting old rows is expensive, and the table keeps growing until something removes them.

  - About 300-400 pollers (see numbers below) means 300-400 connections running queries all the time.

- **Option 2 (Kafka): met.**
  - Writes are appends to a log, and they are spread over partitions (and over brokers, if we add more).

  - Reading scales with the number of partitions and consumers.

  - The risk: the partition count must be chosen well in advance, because changing it later changes which partition a key goes to.

#### R2 Complete and ordered
- **Option 1: at risk.**
  - Stage A showed the problem: `id` is assigned at INSERT, but a row is visible only after COMMIT, so a poller using `id > last_id` skips rows.Several pollers with the same logic have the same flaw.

  - It can be fixed, e.g. by polling per machine with `seq > last_seq` (a gateway commits its own batches in order), and by detecting gaps in `seq` and waiting. But that is extra logic we have to write and trust.


- **Option 2: met.**
  - Key = `machine_id`, so all readings of a machine go to one partition, in the order they were produced, and one consumer per group reads that partition. The model sees every reading in `seq` order.

#### R3 Fresh (anomaly within 5 s, p95)
- **Option 1: at risk.**
  - Delay = polling interval + time to score the backlog. Shorter intervals mean more load on the table. In Stage A the detector was 38 s late with one instance.

- **Option 2: met, if the detector keeps up.**
  - Events arrive without polling. As long as the number of instances matches the load, lag stays near 0 and delay is the model time plus some milliseconds. We can watch it with `LAG` in `kafka-consumer-groups.sh --describe`.

#### R4 Independent consumers
- **Option 1: failed (or only partly met).**
  - Archive and dashboard would poll the same table, so every new consumer adds load on PostgreSQL, and a slow consumer can disturb the detector.

- **Option 2: met.**
  - Each consumer has its own group and its own offsets. Reading deletes nothing, so archive, dashboard and detector read the same topic without affecting each other, and PostgreSQL is not touched.

  - A new group can also replay the history (`--to-earliest`) within the retention time.

### Summary
| | Option 1 | Option 2 |
|---|---|---|
| R1 | at risk | met |
| R2 | at risk (fixable with extra logic) | met (with a key and safe producer settings) |
| R3 | at risk | met (with enough instances) |
| R4 | failed | met |



### Decision
Option 2: the gateways write every reading to Kafka, with `machine_id` as the key. R4 is the deciding point: with PostgreSQL every extra consumer is extra load on the same table, while Kafka consumers read independently in their own groups. The reasons per requirement are in the comparison above, and the instance numbers are below. PostgreSQL stays the system of record for the machine registry.

### Numbers
- One detector: 1 / 0.003 s ≈ 333 readings/s in theory; I measured about
  270 readings/s in Stage A, so I plan with 270.
- Lab (600/s): 600 / 270 ≈ 2.2, so at least 3 instances.
- Rollout (100,000/s): 100,000 / 270 ≈ 370, so about 400 instances with headroom.
  With the theoretical rate it would be 300.
- Partitions: at least as many as instances, because one partition is read by
  only one consumer per group. There are only 2,000 machines (keys), so more
  than 2,000 partitions would not help. The machines are not spread evenly over
  partitions, so the busiest instance gets more than the average: that is why
  I add headroom.

### Tipping point
One reading per minute is about 0.017 readings/s, which PostgreSQL handles
easily. As a rough estimate (not measured), polling a table stops being
enough around the capacity of one detector (a few hundred readings/s) or
as soon as several independent consumers need the same stream, strict
per-key order, or results within seconds. My lab setup (600/s) is already
past that point.

## Stage C (optional)

### Topics
Created before starting the plant, so they are not auto-created with 1 partition:

```bash
docker exec -it kafka kafka-topics.sh --bootstrap-server localhost:9092 \
  --create --topic readings --partitions 6 --replication-factor 1

docker exec -it kafka kafka-topics.sh --bootstrap-server localhost:9092 \
  --create --topic anomalies --partitions 3 --replication-factor 1
```

I used 6 partitions instead of the minimum of 3 from Stage B. Kafka assigns whole
partitions, and the 60 machines hash unevenly over them, so with 3 instances one
instance could get more machines than it can score (~270 readings/s). With 6
partitions and 6 instances, each instance gets one partition.

### Design
- **`KafkaSink` (`plant.py`):** each reading is sent as JSON to `readings` with
  key = `machine_id`, so all readings of a machine go to the same partition, in
  order (R2). Settings: `enable.idempotence=True` (retries cannot duplicate or
  reorder), `linger.ms=10` and `compression.type=lz4` (small batches instead of
  one request per reading). `flush()` is only called on shutdown, so the plant
  does not wait for every message.
- **`detector_service.py`:** all instances use the same group (`detector`), so each
  partition is read by exactly one instance and a machine's readings are never
  split. `auto.offset.reset=latest`: a live detector should start at the newest
  readings, not replay old runs, which would add delay (R3). The producer for
  `anomalies` uses idempotence (no duplicate events), and the key is `machine_id`,
  so the alerts of one machine stay in order.

### Run
6 detector instances (`det-1` to `det-6`, started first), then
`python3 plant.py --sink kafka`, scoreboard after ~5 minutes:

```text
Run 1794406031: started 15:07:11 (5.2 min ago), 60 machines x 10 Hz, 5 faults so far

Detection  (topic anomalies: 1,880 anomaly events, models ['v1'])
   #  machine   start  detected   delay
   1  M-029       40s       yes   7.9 s
   2  M-026      100s       yes   8.2 s
   3  M-031      160s       yes   6.0 s
   4  M-036      220s       yes   7.4 s
   5  M-004      280s  (just started, not scored yet)

  faults detected: 4/4, mean delay 7.4 s; false alarms (episodes on healthy machines): 0
  latency reading -> anomaly event: p50 0.03 s, p95 0.05 s, max 0.05 s
  duplicate anomaly events (same anomaly_id): 0

Detector instances  (model telemetry, this run)
  det-1          pid 53284   v1  readings    38,602  missing       0  out-of-order      0  machines   60  alive
  det-2          pid 53820   v1  readings    39,878  missing       0  out-of-order      0  machines   22  alive
  det-3          pid 54346   v1  readings    27,525  missing       0  out-of-order      0  machines    9  alive
  det-4          pid 54843   v1  readings    24,473  missing       0  out-of-order      0  machines    8  alive
  det-5          pid 55334   v1  readings    33,637  missing       0  out-of-order      0  machines   11  alive
  det-6          pid 55820   v1  readings    27,530  missing       0  out-of-order      0  machines    9  alive

Requirements (measurable part)
  [PASS] R2 complete and ordered: 0 readings missing, 0 out of order
  [PASS] R3 fresh: p95 latency < 5 s
  [PASS] detection: all faults found, at most 2 false alarms
```

### Explanation
- **R2 passes:** 0 missing and 0 out-of-order readings on every instance. The key
  keeps each machine on one partition, and there is no commit-order problem as
  in Stage A.
- **R3 passes:** p95 latency is 0.05 s (Stage A: 38 s). The 6 instances keep up
  with 600 readings/s, so there is no backlog.
- **Detection delay (~7 s) vs. latency (0.05 s):** latency is the time from a
  reading to its anomaly event, which is what R3 measures. Detection delay is the
  time from the start of a fault until the model notices it: the wear rises slowly,
  and the model averages over its last 50 readings (5 s at 10 Hz). That delay
  comes from the model, not from the pipeline.
- **Uneven load:** the instances hold between 8 and 22 machines, because keys hash
  unevenly over partitions. The busiest one (22 machines, ~220 readings/s) is still
  below the ~270 readings/s one detector can score.
- **`det-1` shows 60 machines:** it was alone in the group for a moment, got all
  partitions, and its model kept the state of all 60 machines after the other
  instances took partitions away. This did not cause missing readings, because
  those partitions did not come back to `det-1`. If they had, its model would
  compare against old state and count gaps. Fix: an `on_revoke` callback in
  `consumer.subscribe()` that calls `model.forget()` for the machines it loses.
  I did not implement it.