// Package jobs is the gateway's background-job adapter: it defines the three periodic tasks
// (expired invitation cleanup, session and token cleanup, trial-ending reminders), holds their SQL
// and runs them with go-packages/jobs over the shared Postgres, so any number of gateway replicas
// run each tick once.
//
// It runs inside the gateway process for now. To move it into its own service (when jobs need a
// different scaling profile, or a long job should not share the API's pods), scaffold one with
// `pnpm gen service`, move this package and its wiring from main.go there, and set
// JOBS_DISABLED=true on the gateway; the schedule lives in the database, so the switch needs no
// data migration. See go-packages/jobs/README.md.
package jobs

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/bernardoforcillo/drops/pg"

	jobslib "github.com/bernardoforcillo/ignition/go-packages/jobs"
	"github.com/bernardoforcillo/ignition/go-packages/jobs/pgstore"
)

// Deps are the collaborators the tasks need, built by the composition root.
type Deps struct {
	DB     *pg.DB
	Owners OwnerDirectory
	Mail   TrialMailer
	// AppURL is the public web app URL; the reminder links to {AppURL}/app/billing.
	AppURL string
	Logger *slog.Logger
	// Now is the clock the tasks read; nil means time.Now.
	Now func() time.Time
}

// Runner owns the scheduler.
type Runner struct{ sched *jobslib.Scheduler }

// Option customizes New. The options are for tests; production uses the defaults.
type Option func(*settings)

type settings struct {
	every     time.Duration
	scheduler []jobslib.Option
}

// WithEvery overrides every task's interval (tests drive the real tasks with a short one).
func WithEvery(d time.Duration) Option { return func(s *settings) { s.every = d } }

// WithSchedulerOptions passes options (clock, jitter, retry) through to the scheduler.
func WithSchedulerOptions(opts ...jobslib.Option) Option {
	return func(s *settings) { s.scheduler = append(s.scheduler, opts...) }
}

// New builds the runner. Its tables are created by saas.Migrations().
func New(d Deps, opts ...Option) (*Runner, error) {
	if d.DB == nil || d.Owners == nil || d.Mail == nil {
		return nil, errors.New("jobs: DB, Owners and Mail are required")
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	var set settings
	for _, o := range opts {
		o(&set)
	}
	sched, err := jobslib.New(pgstore.New(d.DB), tasks(d, set.every),
		append([]jobslib.Option{jobslib.WithLogger(d.Logger)}, set.scheduler...)...)
	if err != nil {
		return nil, err
	}
	return &Runner{sched: sched}, nil
}

// Start begins scheduling and returns at once.
func (r *Runner) Start(ctx context.Context) error { return r.sched.Start(ctx) }

// Stop lets a running task finish, or cancels it when ctx ends; see jobs.Scheduler.Stop.
func (r *Runner) Stop(ctx context.Context) error { return r.sched.Stop(ctx) }
