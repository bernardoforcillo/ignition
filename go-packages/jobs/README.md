# go-packages/jobs

Named periodic tasks that run **once per interval across any number of replicas**. Pure Go
(stdlib for the scheduler; Postgres through [drops](https://github.com/bernardoforcillo/drops) in
`pgstore`). It reads no environment and keeps no globals.

```
Scheduler (one loop per task) --Claim/Finish--> Store (port) <-- pgstore (Postgres) | jobstest (memory)
```

## Use

```go
store := pgstore.New(db)                       // *pg.DB; migrations: pgstore.Migrations()
sched, err := jobs.New(store, []jobs.Task{
    {Name: "reports.daily", Every: 24 * time.Hour, Run: func(ctx context.Context) error {
        return buildReports(ctx)               // honor ctx; safe to run again
    }},
}, jobs.WithLogger(logger))
err = sched.Start(context.Background())        // returns at once
...
err = sched.Stop(shutdownCtx)                  // before closing the database
```

Run `pgstore.Migrations()` (`{ID, SQL}`, ordered, idempotent) through your migration runner, for
example `database.SQL(m.ID, m.SQL)` in the gateway's `saas.Migrations()`.

## Model and guarantees

One row per task in `scheduled_job_runs` (`name` PK, `next_run_at`, `last_started_at`,
`last_finished_at`, `last_status`, `last_error`, `run_count`, `failures`).

- **Claim.** A scheduler runs a task only after one atomic statement
  (`INSERT ... ON CONFLICT (name) DO UPDATE ... WHERE next_run_at <= now`) grants it. The row lock
  makes a concurrent replica see the winner's new `next_run_at`, so **two replicas can never run
  the same tick**.
- **Lease.** The claim moves `next_run_at` to `now + Timeout + 5s`. A replica that crashes mid-run
  never finishes it, and the task is claimable again when the lease ends. `run_count` is the
  fencing token: a run that outlives its lease cannot overwrite the result of the run that took
  over (`Finish` reports `applied=false`, logged as a warning).
- **Schedule.** After a success the next run is `start + Every` (a fixed cadence, not drifting by
  the run's duration; never earlier than the finish).
- **Failure.** An error, a panic (recovered, stack in the log) or a timeout marks the run failed and
  retries after `base`, `2*base`, `4*base`... (`WithRetry`, default 30s and 5 retries, never more
  than `Every`). After the retries are used up the task waits its normal interval and starts afresh.
- **Timeout.** The run's context is cancelled after `Task.Timeout` (default
  `min(10m, Every)`); the error wraps `ErrTimeout`. A task that ignores its context keeps running,
  but is no longer exclusive once the lease ends: make `Run` idempotent.
- **Jitter.** Each task's first claim attempt is delayed by a random time up to `min(10s, Every/2)`
  (`WithJitter`), so replicas that start together do not stampede.
- **Shutdown.** `Stop(ctx)` stops scheduling and waits for a running task. If `ctx` ends first, the
  task's context is cancelled, the run is recorded `interrupted` and left due at once for the next
  replica, and `Stop` returns `ctx.Err()`. Cancelling the context passed to `Start` is a hard stop.
- **Clocks.** Due times come from the scheduler's clock (`WithClock`, default system), not from
  Postgres: replicas need roughly synchronized clocks (any NTP-synced cluster is within
  milliseconds, and intervals are minutes or more).
- **Logs.** `slog`, one Info per finished run with its duration, one Error per failed run. Error
  records reach PostHog error tracking through `go-packages/telemetry`, and `last_error` is stored:
  **never put secrets or email addresses in an error a task returns**. Store hiccups are Warn.

## Add a task

1. Write `func(ctx) error` that is idempotent and honors `ctx`.
2. Add a `jobs.Task` with a **stable** unique `Name` (renaming starts a new schedule).
3. A task that must act once per subject (an email) records that in its own table with a unique
   key, claims before acting and releases on failure; see the gateway's `trials.remind` and
   `job_notifications`. The scheduler guarantees one run per tick, not one effect per subject.
4. Test it with fakes for its collaborators, and the scheduling with `jobstest.Clock` and
   `jobstest.Store`.

## Tests

`jobstest` has a fake `Clock` (`Advance`, `WaitPending`), an in-memory `Store` with the Postgres
semantics (two schedulers sharing one are two replicas) and `TestStore`, the contract every
`Store` must pass (`pgstore` runs it against Postgres; set `TEST_DATABASE_URL`, the tests
truncate `scheduled_job_runs`).

## Running it in its own service

Today the gateway starts the scheduler in its process (`apps/gateway/internal/adapter/jobs`). To
extract: `pnpm gen service` for a new `apps/<name>`, give it a `main.go` that opens the database
(`go-packages/database`), runs the migrations and builds the scheduler as above, handles SIGTERM
with `Stop`, and set `JOBS_DISABLED=true` on the gateway. Nothing in the schedule is in memory,
so replicas of the new service (or the old and new during the switch) coordinate through the
table. Scale it by replicas for availability, not throughput: each tick runs on one.
