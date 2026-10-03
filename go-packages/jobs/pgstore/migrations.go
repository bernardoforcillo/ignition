// Package pgstore implements jobs.Store over Postgres, using github.com/bernardoforcillo/drops.
package pgstore

// Migration is one numbered, ordered schema step. The ID is unique across the whole application
// (hence the "jobs_" prefix) and sorts in apply order; hand the list to the service's migration
// runner (database.SQL(m.ID, m.SQL)).
type Migration struct {
	ID  string
	SQL string
}

// Migrations returns the schema this package needs, in apply order. Every statement is idempotent.
func Migrations() []Migration {
	return []Migration{{ID: "jobs_0001_scheduled_job_runs", SQL: createRuns}}
}

// One row per task name. next_run_at is both "when is it next due" and, while a run is in
// progress, the lease: a crashed holder's row becomes claimable again once it passes. run_count
// doubles as the fencing token of the current claim. failures counts consecutive failed runs.
const createRuns = `
CREATE TABLE IF NOT EXISTS scheduled_job_runs (
    name             TEXT        PRIMARY KEY,
    next_run_at      TIMESTAMPTZ NOT NULL,
    last_started_at  TIMESTAMPTZ,
    last_finished_at TIMESTAMPTZ,
    last_status      TEXT        NOT NULL DEFAULT '',
    last_error       TEXT        NOT NULL DEFAULT '',
    run_count        BIGINT      NOT NULL DEFAULT 0,
    failures         INTEGER     NOT NULL DEFAULT 0
)`
