package jobstest

import (
	"context"
	"sync"
	"time"

	"github.com/bernardoforcillo/ignition/go-packages/jobs"
)

// Row mirrors one scheduled_job_runs row.
type Row struct {
	NextRunAt      time.Time
	LastStartedAt  time.Time
	LastFinishedAt time.Time
	LastStatus     jobs.Status
	LastError      string
	RunCount       int64
	Failures       int
}

// Store is an in-memory jobs.Store with the same claim semantics as the Postgres one. Several
// schedulers sharing one Store behave like several replicas sharing a database.
type Store struct {
	mu   sync.Mutex
	rows map[string]*Row
}

var _ jobs.Store = (*Store)(nil)

// NewStore returns an empty store.
func NewStore() *Store { return &Store{rows: map[string]*Row{}} }

// Claim implements jobs.Store.
func (s *Store) Claim(_ context.Context, name string, now time.Time, lease time.Duration) (jobs.Claim, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.rows[name]
	if ok && r.NextRunAt.After(now) {
		return jobs.Claim{NextRunAt: r.NextRunAt}, nil
	}
	if !ok {
		r = &Row{}
		s.rows[name] = r
	}
	r.NextRunAt, r.LastStartedAt, r.LastStatus = now.Add(lease), now, jobs.StatusRunning
	r.RunCount++
	return jobs.Claim{Granted: true, Token: r.RunCount, Failures: r.Failures}, nil
}

// Finish implements jobs.Store.
func (s *Store) Finish(_ context.Context, name string, res jobs.Result) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.rows[name]
	if !ok || r.RunCount != res.Token || r.LastStatus != jobs.StatusRunning {
		return false, nil
	}
	r.LastFinishedAt, r.LastStatus, r.LastError = res.FinishedAt, res.Status, res.Err
	r.Failures, r.NextRunAt = res.Failures, res.NextRunAt
	return true, nil
}

// Row returns a copy of the stored row for name.
func (s *Store) Row(name string) (Row, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.rows[name]
	if !ok {
		return Row{}, false
	}
	return *r, true
}
