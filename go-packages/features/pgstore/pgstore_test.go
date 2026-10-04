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
	"github.com/bernardoforcillo/featurelayer/entitlement"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/bernardoforcillo/ignition/go-packages/features/pgstore"
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

// openDB connects to TEST_DATABASE_URL, applies the migrations and returns a
// DB with empty tables. It skips when no database is configured.
func openDB(t *testing.T) *pg.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db := pg.New(stdlib.New(sqlDB))
	ctx := context.Background()
	for _, m := range pgstore.Migrations() {
		if _, err := db.Exec(ctx, m.SQL); err != nil {
			t.Fatalf("migration %s: %v", m.ID, err)
		}
	}
	for _, table := range []string{"feature_subscriptions", "feature_usage"} {
		if _, err := db.Exec(ctx, "TRUNCATE "+table); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
	return db
}

func TestSubscriptionStore_RoundTripAndFailClosed(t *testing.T) {
	store := pgstore.NewSubscriptionStore(openDB(t))
	ctx := context.Background()

	if _, err := store.Subscription(ctx, "missing"); err != entitlement.ErrNoSubscription {
		t.Fatalf("unknown tenant: err = %v, want ErrNoSubscription", err)
	}

	anchor := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)
	want := entitlement.Subscription{
		TenantID:      "ws-1",
		Plan:          "pro",
		AddOns:        []entitlement.AddOnID{"extra"},
		Trial:         &entitlement.PlanTrial{Plan: "pro", Until: anchor.Add(time.Hour)},
		Grants:        []entitlement.Grant{entitlement.Override("f", &entitlement.Limit{Max: 5, Period: entitlement.Month}, "x")},
		BillingAnchor: anchor,
	}
	if err := store.Set(ctx, want); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := store.Subscription(ctx, "ws-1")
	if err != nil {
		t.Fatalf("Subscription: %v", err)
	}
	if got.Plan != "pro" || len(got.AddOns) != 1 || got.Trial == nil || len(got.Grants) != 1 || !got.BillingAnchor.Equal(anchor) {
		t.Fatalf("round trip lost data: %+v", got)
	}

	// An update must not move the billing anchor.
	want.Plan, want.Trial, want.BillingAnchor = "free", nil, anchor.AddDate(0, 1, 0)
	if err := store.Set(ctx, want); err != nil {
		t.Fatalf("Set (update): %v", err)
	}
	got, _ = store.Subscription(ctx, "ws-1")
	if got.Plan != "free" || got.Trial != nil || !got.BillingAnchor.Equal(anchor) {
		t.Fatalf("update: plan=%q trial=%v anchor=%v", got.Plan, got.Trial, got.BillingAnchor)
	}

	if err := store.Delete(ctx, "ws-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Subscription(ctx, "ws-1"); err != entitlement.ErrNoSubscription {
		t.Fatalf("after delete: err = %v", err)
	}
}

func TestUsageStore_IncrementRespectsTheCeiling(t *testing.T) {
	store := pgstore.NewUsageStore(openDB(t))
	ctx := context.Background()
	key := entitlement.UsageKey{Tenant: "ws-1", Feature: "api.calls", Period: "2026-03-01T00:00:00Z"}

	tests := []struct {
		name        string
		delta, max  int64
		wantTotal   int64
		wantAllowed bool
	}{
		{"first increment creates the row", 6, 10, 6, true},
		{"fits exactly", 4, 10, 10, true},
		{"over the ceiling changes nothing", 1, 10, 10, false},
		{"larger than the whole allowance", 11, 10, 10, false},
		{"unlimited ignores the ceiling", 100, -1, 110, true},
	}
	for _, tc := range tests {
		total, allowed, err := store.Increment(ctx, key, tc.delta, tc.max)
		if err != nil || total != tc.wantTotal || allowed != tc.wantAllowed {
			t.Fatalf("%s: total=%d allowed=%v err=%v", tc.name, total, allowed, err)
		}
	}
	if used, err := store.Get(ctx, key); err != nil || used != 110 {
		t.Fatalf("Get = %d, %v", used, err)
	}
}

func TestUsageStore_ConcurrentIncrementsNeverOverspend(t *testing.T) {
	store := pgstore.NewUsageStore(openDB(t))
	ctx := context.Background()
	key := entitlement.UsageKey{Tenant: "ws-1", Feature: "api.calls", Period: "p"}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		applied int
	)
	for range 30 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, ok, err := store.Increment(ctx, key, 1, 10)
			if err != nil {
				t.Errorf("Increment: %v", err)
				return
			}
			if ok {
				mu.Lock()
				applied++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if applied != 10 {
		t.Fatalf("applied %d increments, want exactly the ceiling (10)", applied)
	}
}
