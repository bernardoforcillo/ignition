package pgstore

import (
	"context"
	"fmt"
	"time"

	"github.com/bernardoforcillo/drops/pg"

	"github.com/bernardoforcillo/ignition/go-packages/jobs"
)

// Store is jobs.Store over scheduled_job_runs.
type Store struct{ db *pg.DB }

// New builds the store over an open database.
func New(db *pg.DB) *Store { return &Store{db: db} }

var _ jobs.Store = (*Store)(nil)

// Claim is one statement, so two replicas can never both be granted the same tick: the upsert
// takes the row lock, and a loser re-evaluates the WHERE against the winner's new next_run_at
// (the lease), which is in the future, so it updates nothing.
func (s *Store) Claim(ctx context.Context, name string, now time.Time, lease time.Duration) (jobs.Claim, error) {
	rows, err := s.db.Query(ctx, `
		INSERT INTO scheduled_job_runs (name, next_run_at, last_started_at, last_status, run_count)
		VALUES ($1, $2, $3, 'running', 1)
		ON CONFLICT (name) DO UPDATE
			SET next_run_at = EXCLUDED.next_run_at, last_started_at = EXCLUDED.last_started_at,
			    last_status = 'running', run_count = scheduled_job_runs.run_count + 1
			WHERE scheduled_job_runs.next_run_at <= EXCLUDED.last_started_at
		RETURNING run_count, failures`,
		name, now.Add(lease).UTC(), now.UTC())
	if err != nil {
		return jobs.Claim{}, fmt.Errorf("jobs: claiming %q: %w", name, err)
	}
	var c jobs.Claim
	if rows.Next() {
		if err := rows.Scan(&c.Token, &c.Failures); err != nil {
			_ = rows.Close()
			return jobs.Claim{}, fmt.Errorf("jobs: scanning claim: %w", err)
		}
		c.Granted = true
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return jobs.Claim{}, fmt.Errorf("jobs: claiming %q: %w", name, err)
	}
	if c.Granted {
		return c, nil
	}
	return s.notDue(ctx, name)
}

func (s *Store) notDue(ctx context.Context, name string) (jobs.Claim, error) {
	rows, err := s.db.Query(ctx, `SELECT next_run_at FROM scheduled_job_runs WHERE name = $1`, name)
	if err != nil {
		return jobs.Claim{}, fmt.Errorf("jobs: reading next run of %q: %w", name, err)
	}
	defer func() { _ = rows.Close() }()
	var c jobs.Claim
	if rows.Next() {
		if err := rows.Scan(&c.NextRunAt); err != nil {
			return jobs.Claim{}, fmt.Errorf("jobs: scanning next run: %w", err)
		}
	}
	return c, rows.Err()
}

// Finish applies only to the claim it fences (run_count) and only while that claim is running.
func (s *Store) Finish(ctx context.Context, name string, res jobs.Result) (bool, error) {
	out, err := s.db.Exec(ctx, `
		UPDATE scheduled_job_runs
		SET last_finished_at = $3, last_status = $4, last_error = $5, failures = $6, next_run_at = $7
		WHERE name = $1 AND run_count = $2 AND last_status = 'running'`,
		name, res.Token, res.FinishedAt.UTC(), string(res.Status), res.Err, res.Failures, res.NextRunAt.UTC())
	if err != nil {
		return false, fmt.Errorf("jobs: recording result of %q: %w", name, err)
	}
	n, err := out.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("jobs: recording result of %q: %w", name, err)
	}
	return n == 1, nil
}
