package pgstore_test

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/bernardoforcillo/drops/pg"
	"github.com/bernardoforcillo/drops/stdlib"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/bernardoforcillo/ignition/go-packages/jobs"
	"github.com/bernardoforcillo/ignition/go-packages/jobs/jobstest"
	"github.com/bernardoforcillo/ignition/go-packages/jobs/pgstore"
)

func TestMigrations_AreOrderedAndNamed(t *testing.T) {
	ms := pgstore.Migrations()
	if len(ms) == 0 {
		t.Fatal("no migrations")
	}
	for i, m := range ms {
		if m.ID == "" || m.SQL == "" {
			t.Fatalf("migration %d is empty: %+v", i, m)
		}
		if i > 0 && ms[i-1].ID >= m.ID {
			t.Fatalf("migration ids must ascend: %q then %q", ms[i-1].ID, m.ID)
		}
	}
}

// openDB connects to TEST_DATABASE_URL, applies the migrations (twice: they are idempotent) and
// returns a DB with an empty table. It skips when no database is configured.
func openDB(t *testing.T) *pg.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("needs a database")
	}
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	sqlDB.SetMaxOpenConns(30)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db := pg.New(stdlib.New(sqlDB))
	ctx := context.Background()
	for range 2 {
		for _, m := range pgstore.Migrations() {
			if _, err := db.Exec(ctx, m.SQL); err != nil {
				t.Fatalf("migration %s: %v", m.ID, err)
			}
		}
	}
	if _, err := db.Exec(ctx, "TRUNCATE scheduled_job_runs"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return db
}

func TestStore_Contract(t *testing.T) {
	jobstest.TestStore(t, func(t *testing.T) jobs.Store {
		db := openDB(t)
		return pgstore.New(db)
	})
}

func TestStore_RecordsTheRun(t *testing.T) {
	db := openDB(t)
	s := pgstore.New(db)
	ctx := context.Background()
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	c, err := s.Claim(ctx, "a", t0, time.Minute)
	if err != nil || !c.Granted {
		t.Fatalf("claim = %+v, %v", c, err)
	}
	if ok, err := s.Finish(ctx, "a", jobs.Result{Token: c.Token, Status: jobs.StatusFailed, Err: "boom", Failures: 1, FinishedAt: t0.Add(time.Second), NextRunAt: t0.Add(time.Hour)}); err != nil || !ok {
		t.Fatalf("finish = %v, %v", ok, err)
	}

	var status, lastErr string
	var runs int64
	var failures int
	var next, started time.Time
	rows, err := db.Query(ctx, `SELECT last_status, last_error, run_count, failures, next_run_at, last_started_at FROM scheduled_job_runs WHERE name = 'a'`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		t.Fatal("no row")
	}
	if err := rows.Scan(&status, &lastErr, &runs, &failures, &next, &started); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || lastErr != "boom" || runs != 1 || failures != 1 || !next.Equal(t0.Add(time.Hour)) || !started.Equal(t0) {
		t.Errorf("row = %s %q %d %d %v %v", status, lastErr, runs, failures, next, started)
	}
}

// Two schedulers (two replicas) over one real table must never run the same interval twice.
func TestScheduler_TwoReplicasOnPostgresNeverDoubleRun(t *testing.T) {
	db := openDB(t)
	var mu sync.Mutex
	var starts []time.Time
	task := jobs.Task{Name: "pg-tick", Every: time.Second, Run: func(context.Context) error {
		mu.Lock()
		defer mu.Unlock()
		starts = append(starts, time.Now())
		return nil
	}}
	var stoppers []func()
	for range 2 {
		s, err := jobs.New(pgstore.New(db), []jobs.Task{task}, jobs.WithJitter(0))
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		stoppers = append(stoppers, func() { _ = s.Stop(context.Background()) })
	}
	time.Sleep(2600 * time.Millisecond)
	for _, stop := range stoppers {
		stop()
	}

	mu.Lock()
	defer mu.Unlock()
	if len(starts) < 2 || len(starts) > 3 {
		t.Fatalf("%d runs in 2.6s of a 1s task, want 2 or 3", len(starts))
	}
	for i := 1; i < len(starts); i++ {
		if gap := starts[i].Sub(starts[i-1]); gap < 900*time.Millisecond {
			t.Errorf("runs %d and %d only %v apart: the interval was run twice", i-1, i, gap)
		}
	}
}
