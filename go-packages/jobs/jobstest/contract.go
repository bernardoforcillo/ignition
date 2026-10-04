package jobstest

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bernardoforcillo/ignition/go-packages/jobs"
)

// TestStore runs the contract every jobs.Store must satisfy. newStore returns a store with no
// rows; it is called once per subtest.
func TestStore(t *testing.T, newStore func(t *testing.T) jobs.Store) {
	t.Helper()
	ctx := context.Background()
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	t.Run("first claim is granted and holds the lease", func(t *testing.T) {
		s := newStore(t)
		c, err := s.Claim(ctx, "a", t0, time.Minute)
		if err != nil || !c.Granted || c.Token == 0 || c.Failures != 0 {
			t.Fatalf("claim = %+v, %v", c, err)
		}
		c2, err := s.Claim(ctx, "a", t0.Add(30*time.Second), time.Minute)
		if err != nil || c2.Granted {
			t.Fatalf("second claim inside the lease = %+v, %v", c2, err)
		}
		if !c2.NextRunAt.Equal(t0.Add(time.Minute)) {
			t.Errorf("NextRunAt = %v, want lease end", c2.NextRunAt)
		}
	})

	t.Run("an expired lease is reclaimed with a new token", func(t *testing.T) {
		s := newStore(t)
		first, _ := s.Claim(ctx, "a", t0, time.Minute)
		second, err := s.Claim(ctx, "a", t0.Add(time.Minute), time.Minute)
		if err != nil || !second.Granted || second.Token == first.Token {
			t.Fatalf("reclaim = %+v, %v (first %+v)", second, err, first)
		}
	})

	t.Run("finish schedules the next run and a stale token is refused", func(t *testing.T) {
		s := newStore(t)
		old, _ := s.Claim(ctx, "a", t0, time.Minute)
		cur, _ := s.Claim(ctx, "a", t0.Add(time.Minute), time.Minute) // old holder's lease expired
		applied, err := s.Finish(ctx, "a", jobs.Result{Token: old.Token, Status: jobs.StatusSucceeded, FinishedAt: t0.Add(2 * time.Minute), NextRunAt: t0.Add(time.Hour)})
		if err != nil || applied {
			t.Fatalf("stale finish applied = %v, %v", applied, err)
		}
		next := t0.Add(time.Hour)
		applied, err = s.Finish(ctx, "a", jobs.Result{Token: cur.Token, Status: jobs.StatusFailed, Err: "boom", Failures: 2, FinishedAt: t0.Add(2 * time.Minute), NextRunAt: next})
		if err != nil || !applied {
			t.Fatalf("finish applied = %v, %v", applied, err)
		}
		if again, _ := s.Finish(ctx, "a", jobs.Result{Token: cur.Token, Status: jobs.StatusSucceeded, NextRunAt: next}); again {
			t.Error("a finished claim was finished twice")
		}
		c, _ := s.Claim(ctx, "a", next.Add(-time.Second), time.Minute)
		if c.Granted || !c.NextRunAt.Equal(next) {
			t.Fatalf("before next run: %+v", c)
		}
		c, _ = s.Claim(ctx, "a", next, time.Minute)
		if !c.Granted || c.Failures != 2 {
			t.Fatalf("at next run: %+v (want granted, 2 failures)", c)
		}
	})

	t.Run("tasks are independent", func(t *testing.T) {
		s := newStore(t)
		a, _ := s.Claim(ctx, "a", t0, time.Minute)
		b, _ := s.Claim(ctx, "b", t0, time.Minute)
		if !a.Granted || !b.Granted {
			t.Fatalf("a=%+v b=%+v", a, b)
		}
	})

	t.Run("concurrent claims grant exactly one", func(t *testing.T) {
		s := newStore(t)
		var granted atomic.Int32
		var wg sync.WaitGroup
		for range 20 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				c, err := s.Claim(ctx, "race", t0, time.Minute)
				if err != nil {
					t.Error(err)
					return
				}
				if c.Granted {
					granted.Add(1)
				}
			}()
		}
		wg.Wait()
		if granted.Load() != 1 {
			t.Fatalf("%d claims granted, want 1", granted.Load())
		}
	})
}
