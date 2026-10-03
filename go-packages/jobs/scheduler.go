package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"runtime/debug"
	"sync"
	"time"
)

const (
	// leaseGrace is added to a task's Timeout to form its lease, so a run that ends exactly at
	// its timeout still finishes before another replica can reclaim it.
	leaseGrace = 5 * time.Second
	// minWait keeps a misbehaving store from turning the loop into a busy spin.
	minWait = time.Second
	// maxErrorLen bounds the stored last_error.
	maxErrorLen = 500
	// finishTimeout bounds recording a result, which must outlive a cancelled run.
	finishTimeout = 10 * time.Second
)

var errShutdown = errors.New("jobs: scheduler stopping")

type options struct {
	clock      Clock
	log        *slog.Logger
	jitter     time.Duration
	retryBase  time.Duration
	maxRetries int
	maxPoll    time.Duration
}

// Option customizes New.
type Option func(*options)

// WithClock replaces the system clock (tests).
func WithClock(c Clock) Option { return func(o *options) { o.clock = c } }

// WithLogger sets the logger; the default is slog.Default().
func WithLogger(l *slog.Logger) Option { return func(o *options) { o.log = l } }

// WithJitter sets the upper bound of the random delay before a task's first claim attempt, which
// spreads replicas that start together. Capped at half the task's interval; the default is 10s.
func WithJitter(max time.Duration) Option { return func(o *options) { o.jitter = max } }

// WithRetry sets the first retry delay after a failure (doubled on each further failure, never
// more than the task's interval) and how many consecutive failures are retried early before the
// task falls back to its normal interval. Defaults: 30s and 5.
func WithRetry(base time.Duration, maxRetries int) Option {
	return func(o *options) { o.retryBase, o.maxRetries = base, maxRetries }
}

// WithPollInterval caps how long a task loop sleeps between claim attempts (default 1m), which
// bounds how late a replica notices a lease that expired or a schedule another replica changed.
func WithPollInterval(max time.Duration) Option { return func(o *options) { o.maxPoll = max } }

// Scheduler runs tasks on their schedules. Build it with New, then Start and Stop it once.
type Scheduler struct {
	store Store
	tasks []Task
	opt   options

	mu       sync.Mutex
	started  bool
	runCtx   context.Context
	cancelRn context.CancelCauseFunc
	cancelLp context.CancelFunc
	wg       sync.WaitGroup
}

// New validates the tasks and builds a Scheduler over store.
func New(store Store, tasks []Task, opts ...Option) (*Scheduler, error) {
	if store == nil {
		return nil, errors.New("jobs: nil store")
	}
	o := options{clock: systemClock{}, jitter: 10 * time.Second, retryBase: 30 * time.Second, maxRetries: 5, maxPoll: time.Minute}
	for _, opt := range opts {
		opt(&o)
	}
	if o.log == nil {
		o.log = slog.Default()
	}
	seen := map[string]bool{}
	norm := make([]Task, len(tasks))
	for i, t := range tasks {
		switch {
		case t.Name == "":
			return nil, fmt.Errorf("%w: empty name", ErrInvalidTask)
		case t.Every <= 0:
			return nil, fmt.Errorf("%w: %q needs a positive interval", ErrInvalidTask, t.Name)
		case t.Run == nil:
			return nil, fmt.Errorf("%w: %q has no Run", ErrInvalidTask, t.Name)
		case seen[t.Name]:
			return nil, fmt.Errorf("%w: duplicate name %q", ErrInvalidTask, t.Name)
		}
		seen[t.Name] = true
		if t.Timeout <= 0 {
			t.Timeout = min(DefaultTimeout, t.Every)
		}
		norm[i] = t
	}
	return &Scheduler{store: store, tasks: norm, opt: o}, nil
}

// Start launches one loop per task and returns immediately. Cancelling ctx stops the scheduler
// hard (running tasks are cancelled too); prefer Stop for a graceful shutdown.
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return ErrStarted
	}
	s.started = true
	s.runCtx, s.cancelRn = context.WithCancelCause(ctx)
	var loopCtx context.Context
	loopCtx, s.cancelLp = context.WithCancel(s.runCtx)
	for _, t := range s.tasks {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.loop(loopCtx, t)
		}()
	}
	return nil
}

// Stop stops scheduling, lets a run in progress finish and returns once every loop has exited.
// If ctx ends first, the running tasks' contexts are cancelled, their runs are recorded as
// interrupted (due again at once) and Stop waits for them to return, then reports ctx.Err().
// Stop before Start, or a second Stop, is a no-op.
func (s *Scheduler) Stop(ctx context.Context) error {
	s.mu.Lock()
	cancelLp, cancelRn := s.cancelLp, s.cancelRn
	s.mu.Unlock()
	if cancelLp == nil {
		return nil
	}
	cancelLp()
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
		cancelRn(errShutdown)
		return nil
	case <-ctx.Done():
		cancelRn(errShutdown)
		<-done
		return ctx.Err()
	}
}

func (s *Scheduler) loop(ctx context.Context, t Task) {
	if !s.sleep(ctx, s.firstDelay(t)) {
		return
	}
	for ctx.Err() == nil {
		if !s.sleep(ctx, s.tick(t)) {
			return
		}
	}
}

func (s *Scheduler) firstDelay(t Task) time.Duration {
	limit := min(s.opt.jitter, t.Every/2)
	if limit <= 0 {
		return 0
	}
	return rand.N(limit)
}

// sleep waits d on the scheduler's clock; false means ctx ended first.
func (s *Scheduler) sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	timer := s.opt.clock.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C():
		return true
	}
}

// tick makes one claim attempt for t, runs it when granted and returns how long to sleep.
func (s *Scheduler) tick(t Task) time.Duration {
	now := s.opt.clock.Now()
	// The claim uses the run context, not the loop's: a claim that lands while Stop begins must
	// not be orphaned until its lease expires; the run below completes it.
	claim, err := s.store.Claim(s.runCtx, t.Name, now, t.Timeout+leaseGrace)
	if err != nil {
		if s.runCtx.Err() == nil {
			s.opt.log.Warn("job claim failed", "job", t.Name, "error", err)
		}
		return s.clampWait(t, 30*time.Second)
	}
	if !claim.Granted {
		return s.clampWait(t, claim.NextRunAt.Sub(now))
	}
	s.execute(t, claim, now)
	return minWait
}

func (s *Scheduler) clampWait(t Task, d time.Duration) time.Duration {
	return max(minWait, min(d, t.Every, s.opt.maxPoll))
}

// execute runs t under its timeout, converts a panic into an error and records the outcome.
func (s *Scheduler) execute(t Task, claim Claim, start time.Time) {
	runCtx, cancel := context.WithCancel(s.runCtx)
	timedOut := make(chan struct{})
	watcherDone := make(chan struct{})
	timer := s.opt.clock.NewTimer(t.Timeout) // on the injected clock, so tests can drive it
	go func() {
		defer close(watcherDone)
		select {
		case <-timer.C():
			close(timedOut)
			cancel()
		case <-runCtx.Done():
		}
	}()
	err := safeRun(runCtx, t)
	cancel()
	<-watcherDone
	timer.Stop()

	end := s.opt.clock.Now()
	took := end.Sub(start)
	res := Result{Token: claim.Token, FinishedAt: end}
	select {
	case <-timedOut:
		if err != nil {
			err = fmt.Errorf("%w after %s: %v", ErrTimeout, t.Timeout, err)
		}
	default:
	}

	switch {
	case err == nil:
		res.Status = StatusSucceeded
		res.NextRunAt = start.Add(t.Every) // a fixed cadence, not drifting by the run's duration
		if res.NextRunAt.Before(end) {
			res.NextRunAt = end
		}
		s.opt.log.Info("job finished", "job", t.Name, "duration", took.String())
	case s.runCtx.Err() != nil:
		res.Status, res.Failures, res.NextRunAt = StatusInterrupted, claim.Failures, end
		s.opt.log.Info("job interrupted by shutdown", "job", t.Name, "duration", took.String())
	default:
		res.Status, res.Err = StatusFailed, truncate(err.Error())
		res.Failures = claim.Failures + 1
		delay := s.retryDelay(t, res.Failures)
		if res.Failures > s.opt.maxRetries {
			res.Failures = 0 // retries exhausted: wait the normal interval, then start afresh
		}
		res.NextRunAt = end.Add(delay)
		attrs := []any{"job", t.Name, "duration", took.String(), "error", err, "retry_in", delay.String()}
		var pe *panicError
		if errors.As(err, &pe) {
			attrs = append(attrs, "stack", string(pe.stack))
		}
		s.opt.log.Error("job failed", attrs...)
	}

	// A result must be recorded even when the run was cancelled.
	fctx, fcancel := context.WithTimeout(context.WithoutCancel(s.runCtx), finishTimeout)
	defer fcancel()
	applied, ferr := s.store.Finish(fctx, t.Name, res)
	switch {
	case ferr != nil:
		s.opt.log.Warn("job result not recorded", "job", t.Name, "error", ferr)
	case !applied:
		s.opt.log.Warn("job lease lost before the run finished; another replica may have run it", "job", t.Name)
	}
}

// retryDelay is base*2^(n-1) capped at the interval, or the interval once n exceeds maxRetries.
func (s *Scheduler) retryDelay(t Task, failures int) time.Duration {
	if failures > s.opt.maxRetries {
		return t.Every
	}
	d := s.opt.retryBase
	for i := 1; i < failures && d < t.Every; i++ {
		d *= 2
	}
	return min(d, t.Every)
}

// panicError carries a recovered panic; it matches ErrPanic.
type panicError struct {
	value any
	stack []byte
}

func (e *panicError) Error() string        { return fmt.Sprintf("%v: %v", ErrPanic, e.value) }
func (e *panicError) Is(target error) bool { return target == ErrPanic }

func safeRun(ctx context.Context, t Task) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = &panicError{value: r, stack: debug.Stack()}
		}
	}()
	return t.Run(ctx)
}

func truncate(s string) string {
	if len(s) <= maxErrorLen {
		return s
	}
	return s[:maxErrorLen]
}
