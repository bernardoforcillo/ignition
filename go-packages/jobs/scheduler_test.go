package jobs_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bernardoforcillo/ignition/go-packages/jobs"
	"github.com/bernardoforcillo/ignition/go-packages/jobs/jobstest"
)

var t0 = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// env is one "cluster": a shared store, a fake clock and a log.
type env struct {
	t     *testing.T
	store *jobstest.Store
	clock *jobstest.Clock
	logs  *syncBuffer
}

func newEnv(t *testing.T) *env {
	t.Helper()
	return &env{t: t, store: jobstest.NewStore(), clock: jobstest.NewClock(t0), logs: &syncBuffer{}}
}

// start builds, starts and (on cleanup) stops one scheduler: one replica.
func (e *env) start(tasks []jobs.Task, opts ...jobs.Option) *jobs.Scheduler {
	e.t.Helper()
	base := []jobs.Option{
		jobs.WithClock(e.clock), jobs.WithJitter(0), jobs.WithRetry(10*time.Second, 5),
		jobs.WithLogger(slog.New(slog.NewJSONHandler(e.logs, nil))),
	}
	s, err := jobs.New(e.store, tasks, append(base, opts...)...)
	if err != nil {
		e.t.Fatal(err)
	}
	if err := s.Start(context.Background()); err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.Stop(ctx)
	})
	return s
}

// eventually polls cond in real time; the scheduler's goroutines are what it waits for.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// settle waits until the named task's last claimed run has finished and sleepers loops are
// asleep on the fake clock, so the next Advance reaches them.
func (e *env) settle(name string, sleepers int) {
	e.t.Helper()
	eventually(e.t, name+" to finish", func() bool {
		r, ok := e.store.Row(name)
		return ok && r.LastStatus != jobs.StatusRunning
	})
	if !e.clock.WaitPending(sleepers) {
		e.t.Fatalf("scheduler loops did not go to sleep (pending=%d, want %d)", e.clock.Pending(), sleepers)
	}
}

func counting(n *atomic.Int32) jobs.Task {
	return jobs.Task{Name: "count", Every: time.Hour, Run: func(context.Context) error { n.Add(1); return nil }}
}

func TestScheduler_RunsOncePerInterval(t *testing.T) {
	e := newEnv(t)
	var n atomic.Int32
	e.start([]jobs.Task{counting(&n)})

	eventually(t, "first run", func() bool { return n.Load() == 1 })
	e.settle("count", 1)
	e.clock.Advance(30 * time.Minute)
	e.settle("count", 1)
	if n.Load() != 1 {
		t.Fatalf("ran %d times before the interval elapsed", n.Load())
	}
	e.clock.Advance(30 * time.Minute)
	eventually(t, "second run", func() bool { return n.Load() == 2 })
	e.settle("count", 1)

	row, _ := e.store.Row("count")
	if row.RunCount != 2 || row.LastStatus != jobs.StatusSucceeded || !row.NextRunAt.Equal(t0.Add(2*time.Hour)) {
		t.Errorf("row = %+v", row)
	}
	if !strings.Contains(e.logs.String(), `"msg":"job finished"`) || strings.Contains(e.logs.String(), `"level":"ERROR"`) {
		t.Errorf("logs = %s", e.logs.String())
	}
}

func TestScheduler_TwoReplicasNeverDoubleRun(t *testing.T) {
	e := newEnv(t)
	var n atomic.Int32
	e.start([]jobs.Task{counting(&n)})
	e.start([]jobs.Task{counting(&n)})

	for i := 1; i <= 5; i++ {
		eventually(t, "run", func() bool { return n.Load() >= int32(i) })
		e.settle("count", 2)
		if i < 5 {
			e.clock.Advance(time.Hour)
		}
	}
	if n.Load() != 5 {
		t.Fatalf("two replicas ran the task %d times over 5 ticks, want 5", n.Load())
	}
}

func TestScheduler_FirstTickIsJittered(t *testing.T) {
	e := newEnv(t)
	var n atomic.Int32
	e.start([]jobs.Task{counting(&n)}, jobs.WithJitter(10*time.Second))

	if !e.clock.WaitPending(1) {
		t.Fatal("no jitter timer")
	}
	if n.Load() != 0 {
		t.Fatal("ran before the jitter elapsed")
	}
	e.clock.Advance(10 * time.Second)
	eventually(t, "run after jitter", func() bool { return n.Load() == 1 })
}

func TestScheduler_FailureRetriesWithBackoffThenResumesSchedule(t *testing.T) {
	e := newEnv(t)
	var calls atomic.Int32
	task := jobs.Task{Name: "flaky", Every: time.Hour, Run: func(context.Context) error {
		if calls.Add(1) < 3 {
			return errors.New("upstream down")
		}
		return nil
	}}
	e.start([]jobs.Task{task})

	eventually(t, "first attempt", func() bool { return calls.Load() == 1 })
	e.settle("flaky", 1)
	row, _ := e.store.Row("flaky")
	if row.LastStatus != jobs.StatusFailed || row.LastError != "upstream down" || row.Failures != 1 || !row.NextRunAt.Equal(t0.Add(10*time.Second)) {
		t.Fatalf("after 1st failure: %+v", row)
	}

	e.clock.Advance(10 * time.Second) // base delay
	eventually(t, "second attempt", func() bool { return calls.Load() == 2 })
	e.settle("flaky", 1)
	row, _ = e.store.Row("flaky")
	if row.Failures != 2 || !row.NextRunAt.Equal(t0.Add(10*time.Second).Add(20*time.Second)) {
		t.Fatalf("after 2nd failure (backoff should double): %+v", row)
	}

	e.clock.Advance(20 * time.Second)
	eventually(t, "third attempt", func() bool { return calls.Load() == 3 })
	e.settle("flaky", 1)
	row, _ = e.store.Row("flaky")
	if row.LastStatus != jobs.StatusSucceeded || row.Failures != 0 || !row.NextRunAt.Equal(t0.Add(30*time.Second).Add(time.Hour)) {
		t.Fatalf("after success: %+v", row)
	}
	if got := strings.Count(e.logs.String(), `"level":"ERROR"`); got != 2 {
		t.Errorf("%d error records, want one per failed run (2)", got)
	}
}

func TestScheduler_ExhaustedRetriesFallBackToTheInterval(t *testing.T) {
	e := newEnv(t)
	var calls atomic.Int32
	task := jobs.Task{Name: "broken", Every: time.Hour, Run: func(context.Context) error { calls.Add(1); return errors.New("nope") }}
	e.start([]jobs.Task{task}, jobs.WithRetry(10*time.Second, 1))

	eventually(t, "attempt 1", func() bool { return calls.Load() == 1 })
	e.settle("broken", 1)
	e.clock.Advance(10 * time.Second)
	eventually(t, "attempt 2", func() bool { return calls.Load() == 2 })
	e.settle("broken", 1)

	row, _ := e.store.Row("broken")
	if want := t0.Add(10 * time.Second).Add(time.Hour); !row.NextRunAt.Equal(want) || row.Failures != 0 {
		t.Fatalf("after retries ran out: next=%v failures=%d, want next=%v failures=0", row.NextRunAt, row.Failures, want)
	}
}

func TestScheduler_PanicBecomesAFailedRun(t *testing.T) {
	e := newEnv(t)
	var calls atomic.Int32
	task := jobs.Task{Name: "boom", Every: time.Hour, Run: func(context.Context) error {
		if calls.Add(1) == 1 {
			panic("kaboom")
		}
		return nil
	}}
	e.start([]jobs.Task{task})

	eventually(t, "panicking run", func() bool { return calls.Load() == 1 })
	e.settle("boom", 1)
	row, _ := e.store.Row("boom")
	if row.LastStatus != jobs.StatusFailed || !strings.Contains(row.LastError, "panicked") || !strings.Contains(row.LastError, "kaboom") {
		t.Fatalf("row = %+v", row)
	}
	// the scheduler survived: the retry runs
	e.clock.Advance(10 * time.Second)
	eventually(t, "retry after panic", func() bool { return calls.Load() == 2 })
}

func TestScheduler_TimeoutCancelsTheRun(t *testing.T) {
	e := newEnv(t)
	started := make(chan struct{})
	task := jobs.Task{Name: "slow", Every: time.Hour, Timeout: time.Minute, Run: func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}}
	e.start([]jobs.Task{task})

	<-started
	e.clock.Advance(time.Minute)
	e.settle("slow", 1)
	row, _ := e.store.Row("slow")
	if row.LastStatus != jobs.StatusFailed || !strings.Contains(row.LastError, "timed out") {
		t.Fatalf("row = %+v", row)
	}
}

func TestScheduler_ReclaimsARunWhoseHolderCrashed(t *testing.T) {
	e := newEnv(t)
	// a replica that claimed the tick and died: nothing will ever Finish it
	if c, err := e.store.Claim(context.Background(), "count", t0, time.Minute+5*time.Second); err != nil || !c.Granted {
		t.Fatal(c, err)
	}
	var n atomic.Int32
	task := counting(&n)
	task.Timeout = time.Minute
	e.start([]jobs.Task{task})

	if !e.clock.WaitPending(1) {
		t.Fatal("loop did not sleep")
	}
	e.clock.Advance(30 * time.Second)
	e.clock.WaitPending(1)
	if n.Load() != 0 {
		t.Fatal("took over a live lease")
	}
	e.clock.Advance(40 * time.Second) // past timeout + grace
	eventually(t, "takeover", func() bool { return n.Load() == 1 })
}

func TestScheduler_StopWaitsForTheRunningTask(t *testing.T) {
	e := newEnv(t)
	started, release := make(chan struct{}), make(chan struct{})
	var finished atomic.Bool
	task := jobs.Task{Name: "long", Every: time.Hour, Run: func(context.Context) error {
		close(started)
		<-release
		finished.Store(true)
		return nil
	}}
	s := e.start([]jobs.Task{task})
	<-started

	stopped := make(chan error, 1)
	go func() { stopped <- s.Stop(context.Background()) }()
	select {
	case <-stopped:
		t.Fatal("Stop returned while a task was running")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	if !finished.Load() {
		t.Fatal("Stop returned before the task finished")
	}
	if row, _ := e.store.Row("long"); row.LastStatus != jobs.StatusSucceeded {
		t.Errorf("a gracefully stopped run must be recorded: %+v", row)
	}
	if err := s.Stop(context.Background()); err != nil {
		t.Errorf("second Stop = %v", err)
	}
}

func TestScheduler_StopDeadlineCancelsTheTaskAndLeavesItDue(t *testing.T) {
	e := newEnv(t)
	started := make(chan struct{})
	task := jobs.Task{Name: "long", Every: time.Hour, Run: func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}}
	s := e.start([]jobs.Task{task})
	<-started

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Stop(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop = %v, want context.Canceled", err)
	}
	row, _ := e.store.Row("long")
	if row.LastStatus != jobs.StatusInterrupted || !row.NextRunAt.Equal(e.clock.Now()) {
		t.Fatalf("an interrupted run must be due again at once: %+v", row)
	}
	if strings.Contains(e.logs.String(), `"level":"ERROR"`) {
		t.Errorf("a shutdown is not an error: %s", e.logs.String())
	}
}

func TestScheduler_SurvivesStoreErrors(t *testing.T) {
	e := newEnv(t)
	var n atomic.Int32
	flaky := &flakyStore{Store: e.store}
	flaky.failures.Store(1)
	e.store = nil
	s, err := jobs.New(flaky, []jobs.Task{counting(&n)}, jobs.WithClock(e.clock), jobs.WithJitter(0), jobs.WithLogger(slog.New(slog.NewJSONHandler(e.logs, nil))))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Stop(context.Background()) }()

	if !e.clock.WaitPending(1) {
		t.Fatal("loop did not back off")
	}
	e.clock.Advance(30 * time.Second)
	eventually(t, "run after the store recovers", func() bool { return n.Load() == 1 })
	if !strings.Contains(e.logs.String(), "job claim failed") {
		t.Error("claim failure not logged")
	}
}

type flakyStore struct {
	*jobstest.Store
	failures atomic.Int32
}

func (f *flakyStore) Claim(ctx context.Context, name string, now time.Time, lease time.Duration) (jobs.Claim, error) {
	if f.failures.Add(-1) >= 0 {
		return jobs.Claim{}, errors.New("db down")
	}
	return f.Store.Claim(ctx, name, now, lease)
}

func TestScheduler_StopLeaksNoGoroutines(t *testing.T) {
	before := runtime.NumGoroutine()
	e := newEnv(t)
	var n atomic.Int32
	s := e.start([]jobs.Task{counting(&n), {Name: "other", Every: time.Minute, Run: func(context.Context) error { return nil }}})
	eventually(t, "runs", func() bool { return n.Load() == 1 })
	if err := s.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	eventually(t, "goroutines to exit", func() bool { return runtime.NumGoroutine() <= before })
	if e.clock.Pending() != 0 {
		t.Errorf("%d timers left pending", e.clock.Pending())
	}
}

func TestNew_RejectsInvalidTasks(t *testing.T) {
	run := func(context.Context) error { return nil }
	cases := map[string][]jobs.Task{
		"empty name":    {{Every: time.Hour, Run: run}},
		"zero interval": {{Name: "a", Run: run}},
		"nil run":       {{Name: "a", Every: time.Hour}},
		"duplicate":     {{Name: "a", Every: time.Hour, Run: run}, {Name: "a", Every: time.Hour, Run: run}},
	}
	for name, tasks := range cases {
		if _, err := jobs.New(jobstest.NewStore(), tasks); !errors.Is(err, jobs.ErrInvalidTask) {
			t.Errorf("%s: err = %v, want ErrInvalidTask", name, err)
		}
	}
	if _, err := jobs.New(nil, nil); err == nil {
		t.Error("nil store accepted")
	}
}

func TestScheduler_StartTwiceFails(t *testing.T) {
	e := newEnv(t)
	s := e.start(nil)
	if err := s.Start(context.Background()); !errors.Is(err, jobs.ErrStarted) {
		t.Fatalf("second Start = %v", err)
	}
}
