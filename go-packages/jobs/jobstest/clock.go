// Package jobstest holds the test doubles of go-packages/jobs: a fake Clock, an in-memory Store
// and the contract tests every Store must pass.
package jobstest

import (
	"sync"
	"time"

	"github.com/bernardoforcillo/ignition/go-packages/jobs"
)

// Clock is a jobs.Clock that only moves when Advance is called.
type Clock struct {
	mu      sync.Mutex
	now     time.Time
	timers  map[*timer]struct{}
	changed chan struct{} // closed and replaced whenever a timer is added
}

type timer struct {
	c     *Clock
	at    time.Time
	ch    chan time.Time
	fired bool
}

var _ jobs.Clock = (*Clock)(nil)

// NewClock starts the clock at start.
func NewClock(start time.Time) *Clock {
	return &Clock{now: start, timers: map[*timer]struct{}{}, changed: make(chan struct{})}
}

// Now implements jobs.Clock.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// NewTimer implements jobs.Clock.
func (c *Clock) NewTimer(d time.Duration) jobs.Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &timer{c: c, at: c.now.Add(d), ch: make(chan time.Time, 1)}
	if d <= 0 {
		t.fired = true
		t.ch <- c.now
		return t
	}
	c.timers[t] = struct{}{}
	close(c.changed)
	c.changed = make(chan struct{})
	return t
}

func (t *timer) C() <-chan time.Time { return t.ch }

func (t *timer) Stop() bool {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	_, active := t.c.timers[t]
	delete(t.c.timers, t)
	return active
}

// Advance moves time forward by d and fires every timer that came due.
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	for t := range c.timers {
		if !t.at.After(c.now) {
			t.fired = true
			t.ch <- c.now
			delete(c.timers, t)
		}
	}
}

// Pending is the number of timers neither fired nor stopped.
func (c *Clock) Pending() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.timers)
}

// WaitPending blocks until exactly n timers are pending, so a test knows the scheduler's loops
// have gone to sleep before it advances time. It gives up after a real second and reports false.
func (c *Clock) WaitPending(n int) bool {
	deadline := time.After(time.Second)
	for {
		c.mu.Lock()
		ok, changed := len(c.timers) == n, c.changed
		c.mu.Unlock()
		if ok {
			return true
		}
		select {
		case <-changed:
		case <-deadline:
			return false
		}
	}
}
