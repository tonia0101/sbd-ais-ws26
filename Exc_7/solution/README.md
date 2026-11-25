# Exercise 7: From the log to the lake

**Chosen architecture: B, "the lake is the truth"** (staging folder + atomic publish,
position read from the lake itself).

```python
def run_once():
    start = lake_position()          # 1. where did the last run stop? -> ask the lake
    new = read_new(start)            # 2. read the records after that position, up to the end of the topic now
    run = write_files(new, STAGING)  # 3a. write them into STAGING (invisible to readers)
    publish(run)                     # 3b. move the finished run into the lake: one rename, all or nothing
                                     # 4. nothing to remember: the lake itself is the position
```

---

## 0. Why the example fails

The example job calls `read_new(None)` on every run, so it always reads the topic **from the
beginning** and writes everything straight into the lake. It has no memory of where it stopped.

| | in lake | duplicates | missing | check |
|---|---:|---:|---:|---|
| after run 1 | 2,000,000 | 0 | 0 | PASS |
| after run 2 | 4,000,000 | 2,000,000 | 0 | **FAIL** |

The first run is correct by accident (the lake was empty). The second run copies the same
2,000,000 records again, so every record is in the lake twice. To be correct, a run must start
exactly where the previous run stopped: not earlier (duplicates) and not later (missing records).
Even when a run crashes, this "where it stopped" (the **position** = the next offset per Kafka
partition) has to stay consistent with what is actually in the lake.

---

## 1. Decision: which architecture and why

A run has to do two things: **put the data into the lake** and **remember the position**. The
question is what happens if the job stops between these two, or in the middle of one of them.
`check_all` tests exactly this: a stop *while* writing and a stop *after* writing.

### A: State file (rejected)

In A the job loads its position from a state file, reads the new records, writes them into the
lake and finally saves the new position with `save_state(new.end)`.

- **Stop after writing, before `save_state()`:** the records are already in the lake, but the
  state file still contains the old position. The next run reads the same records again and
  writes them a second time → **duplicates**.
- **Stop while writing:** some of the run's files are already in the lake (and visible to
  readers), but the position was not saved → the next run writes those records again →
  **duplicates**.

Changing the order (saving the state *before* writing) does not help: then a stop after
`save_state()` but before writing skips those records forever → **missing records**.

**Root cause:** the data (lake) and the position (state file) live in **two different places**
that cannot be updated in one single step. Whatever order you choose, there is a moment in which
one is updated and the other one is not.

### C: Remove duplicates when reading (rejected)

C accepts that the lake may contain duplicates (it is A plus a crash-tolerant read side) and
gives readers a view (`DEDUP_SQL`) that keeps each `(kafka_partition, kafka_offset)` only once.
It passes the check, but:

- the lake keeps growing with duplicate data after every crash, so storage is wasted;
- **every** query pays for the deduplication (`DISTINCT ON` over the whole lake), and that gets
  slower as the lake grows, even though the copying problem happens only once, in the job;
- every reader has to know about and use the view. A reader that reads the Parquet files
  directly (a dashboard, another team, a different tool) gets wrong numbers;
- half-written runs are still visible to readers until the next run finishes.

So C does not fix the problem. It moves the problem from one writer to all readers.

### B: The lake is the truth (chosen)

B removes the root cause of A: there is **only one place**. The job does not store its position
separately. It derives it from the data with `lake_position()`, which asks the lake for the
highest `kafka_offset` per partition (+1). Data and position can therefore never disagree: if a
record is in the lake, the position is past it; if it is not, the position is before it.

The remaining risk is a **half-written run** in the lake. Readers would see incomplete data, and
`lake_position()` would compute a position from incomplete files. B solves this with a
**staging folder**:

1. The run writes its files into `STAGING`, a folder that readers and `lake_position()` never
   look at.
2. Only when all files are complete, `publish(run)` moves the whole run folder into `LAKE` with
   **one `os.rename`**. A rename on the same file system is atomic: it either happened completely
   or not at all. There is no "half published" state.

The rename is the **commit point** of a run: before it, the run does not exist for anyone; after
it, the data and the new position appear together, in the same single step.

**Why B over C:** B gives *exactly once* in the lake itself, so readers can use plain
`read_parquet(...)` without knowing anything about the job, there is no wasted storage, and no
read-time deduplication cost. B is also the idea behind modern table formats such as Delta Lake
and Apache Iceberg: write the data files first, then make them visible with one atomic commit.

### Summary: A vs. B vs. C

| | A: state file | B: lake is the truth | C: dedup when reading |
|---|---|---|---|
| where is the position? | separate state file | derived from the lake | separate state file |
| stop **while** writing | half run visible, then duplicates | lake untouched, run repeated | half run visible, duplicates in files |
| stop **after** writing | duplicates | lake untouched, run repeated | duplicates in files |
| exactly once in the lake files? | no | **yes** | no (only through the view) |
| readers can use plain Parquet? | – | **yes** | no, they must use the view |
| extra cost | – | one rename per run | dedup on every query, wasted storage |

---

## 2. Explanation of the result

How B behaves in the five situations of `check_all`:

| situation | what happens with B | result |
|---|---|---|
| 1 first run | lake empty → `lake_position()` = `None` → reads from the beginning, writes to staging, publishes | all records exactly once |
| 2 new records arrive | position = highest offset in the lake + 1 → reads only the 100,000 new records | no duplicates, nothing missing |
| 3 nothing new | `read_new` returns 0 records → `write_files` returns `None` → `publish(None)` does nothing | lake unchanged |
| 4 stop **while** writing | some files exist only in `STAGING`; the lake is untouched. Right after the stop the check shows the new records as *not loaded* (not an error). The next run gets the same position from the lake, `write_files` clears the leftovers in `STAGING`, rewrites the run and publishes it | no duplicates, nothing missing |
| 5 stop **after** writing (before `publish`) | the complete run sits in `STAGING` but was never published, so for the lake it does not exist. The next run reads the same records again, rewrites them and publishes once | no duplicates, nothing missing |

The important insight: in B a stopped run simply **never happened** from the lake's point of
view. The next run repeats it from the same position. Repeating is safe because nothing of the
stopped run became visible. Records that arrived while a run was working are left for the next
run (`read_new` fixes `new.end` when the run starts), so they show up as "not loaded", never as
"missing".

---

## 3. Output of `check_all(run_once)`

```text
situation                                 in lake  duplicates  missing  not loaded    run  check
-----------------------------------------------------------------------------------------------
1 first run                             2,000,000           0        0           0   6.0s  PASS
2 new records arrive, run again         2,100,000           0        0           0   2.9s  PASS
3 nothing new, run again                2,100,000           0        0           0   0.1s  PASS
4 run stops WHILE writing: right after  2,100,000           0        0     100,000         PASS
  ... then the next run                 2,200,000           0        0           0   2.7s  PASS
5 run stops AFTER writing: right after  2,200,000           0        0     100,000         PASS
  ... then the next run                 2,300,000           0        0           0   2.8s  PASS
-----------------------------------------------------------------------------------------------
Exactly once after every run: YES
```

Every row is PASS and the last line is `Exactly once after every run: YES`: no record is
missing or duplicated, even when a run stops while or after writing.

How to read the two "stop" situations: right after the stop, the lake still has the old number
of records (2,100,000 in situation 4, 2,200,000 in situation 5), and the 100,000 new records are
only *not loaded*, which is not an error. Nothing of the stopped run reached the lake, because it
was only in `STAGING`. The next run then loads exactly those 100,000 records once
(2,200,000 and 2,300,000, with 0 duplicates and 0 missing).
