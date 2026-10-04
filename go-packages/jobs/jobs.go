// Package jobs runs named periodic tasks exactly once per interval across any number of
// replicas.
//
// A Scheduler wakes up for each Task, asks a Store to claim the run atomically and only the
// replica that wins the claim runs it. The claim is a lease: a replica that crashes mid-run
// releases the task when the lease (the task's Timeout) expires. A failed run is retried sooner
// with a capped exponential backoff and then falls back to the normal interval.
//
// The package reads no environment and keeps no globals. The Postgres Store is in pgstore, an
// in-memory Store and a fake Clock for tests are in jobstest.
package jobs

import (
	"context"
	"errors"
	"time"
)

// Task is a named periodic unit of work.
type Task struct {
	// Name identifies the task across replicas and restarts: it is the key of the claim, so
	// renaming a task starts a new schedule.
	Name string
	// Every is the interval between runs. Must be positive.
	Every time.Duration
	// Timeout cancels the run's context and is the lease after which another replica may take
	// over a run whose holder crashed. Zero means DefaultTimeout (never more than Every).
	Timeout time.Duration
	// Run does the work. It must honor ctx and be safe to run again after a partial failure.
	// A returned error (or a panic) marks the run failed. Errors are logged at Error level and
	// reach error tracking, and the message is stored: never put secrets or addresses in it.
	Run func(ctx context.Context) error
}

// DefaultTimeout bounds a run of a task that sets no Timeout.
const DefaultTimeout = 10 * time.Minute

// Sentinel errors a failed run's error wraps.
var (
	// ErrTimeout marks a run that was still going when its Timeout expired.
	ErrTimeout = errors.New("jobs: task timed out")
	// ErrPanic marks a run that panicked.
	ErrPanic = errors.New("jobs: task panicked")
	// ErrInvalidTask is returned by New for an unusable Task.
	ErrInvalidTask = errors.New("jobs: invalid task")
	// ErrStarted is returned by Start when the scheduler already started.
	ErrStarted = errors.New("jobs: scheduler already started")
)

// Status is how the last claimed run of a task ended.
type Status string

// The values stored in last_status.
const (
	StatusRunning     Status = "running"
	StatusSucceeded   Status = "succeeded"
	StatusFailed      Status = "failed"
	StatusInterrupted Status = "interrupted" // cut short by a shutdown; due again immediately
)

// Store is the shared state that makes a run happen once per interval. Implementations must make
// Claim atomic: of any number of concurrent callers for the same due task, exactly one is granted.
type Store interface {
	// Claim grants the run of name when it is due at now (no row yet, or next_run_at <= now) and
	// holds it for lease: next_run_at becomes now+lease, run_count is incremented, last_started_at
	// becomes now. When not due it returns Granted=false with the stored NextRunAt.
	Claim(ctx context.Context, name string, now time.Time, lease time.Duration) (Claim, error)
	// Finish records the outcome of a granted run, but only if res.Token still identifies the
	// current claim (a run whose lease expired and was reclaimed must not overwrite the new one).
	// It reports whether the result was applied.
	Finish(ctx context.Context, name string, res Result) (applied bool, err error)
}

// Claim is the answer to Store.Claim.
type Claim struct {
	Granted bool
	// Token fences Finish; the Store's run_count after this claim. Meaningful when Granted.
	Token int64
	// Failures is the number of consecutive failed runs before this one. Meaningful when Granted.
	Failures int
	// NextRunAt is when the task is next due. Meaningful when not Granted.
	NextRunAt time.Time
}

// Result is the outcome of one run, as stored.
type Result struct {
	Token      int64
	Status     Status
	Err        string // empty unless Status is failed
	FinishedAt time.Time
	// Failures is the consecutive failure count after this run (0 after a success).
	Failures  int
	NextRunAt time.Time
}

// Clock is the scheduler's time source; the default is the system clock.
type Clock interface {
	Now() time.Time
	// NewTimer returns a timer that fires once d has elapsed.
	NewTimer(d time.Duration) Timer
}

// Timer is the part of *time.Timer the scheduler uses.
type Timer interface {
	// C receives the time once the timer fires.
	C() <-chan time.Time
	// Stop releases the timer; it reports whether it stopped before firing.
	Stop() bool
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

func (systemClock) NewTimer(d time.Duration) Timer { return sysTimer{time.NewTimer(d)} }

type sysTimer struct{ t *time.Timer }

func (s sysTimer) C() <-chan time.Time { return s.t.C }
func (s sysTimer) Stop() bool          { return s.t.Stop() }
