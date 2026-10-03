package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/bernardoforcillo/authlayer/org"
	dropsstore "github.com/bernardoforcillo/authlayer/store/drops"
	"github.com/bernardoforcillo/drops/pg"

	"github.com/bernardoforcillo/ignition/go-packages/database"
	"github.com/bernardoforcillo/ignition/go-packages/database/dbtest"
)

// openStore returns a store over empty tables. The authlayer tables are created with authlayer's
// own schema; the two gateway tables are the ones saas.Migrations() creates (the end-to-end test
// in package main runs the real migrations and would catch a drift).
func openStore(t *testing.T) (*store, *pg.DB) {
	t.Helper()
	if testing.Short() {
		t.Skip("needs external service")
	}
	db := scratchSchema(t)
	ctx := context.Background()
	for _, create := range []func(context.Context) error{
		dropsstore.NewAuthStore(db).CreateSchema,
		dropsstore.New[org.Organization, org.Member](db).CreateSchema,
		dropsstore.NewInviteStore(db).CreateSchema,
	} {
		if err := create(ctx); err != nil {
			t.Fatal(err)
		}
	}
	for _, ddl := range []string{
		`CREATE TABLE billing_subscriptions (workspace_id TEXT PRIMARY KEY, status TEXT NOT NULL,
			current_period_end TIMESTAMPTZ, updated_at TIMESTAMPTZ NOT NULL DEFAULT now())`,
		`CREATE TABLE job_notifications (kind TEXT NOT NULL, key TEXT NOT NULL,
			sent_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY (kind, key))`,
	} {
		if _, err := db.Exec(ctx, ddl); err != nil {
			t.Fatal(err)
		}
	}
	return &store{db: db}, db
}

// scratchSchema returns a connection whose search_path is a private schema, dropped on cleanup, so
// these tests neither drop nor see the tables other packages' tests use in the same database.
func scratchSchema(t *testing.T) *pg.DB {
	t.Helper()
	admin := dbtest.Open(t) // skips without TEST_DATABASE_URL
	var b [6]byte
	_, _ = rand.Read(b[:])
	schema := "jobs_test_" + hex.EncodeToString(b[:])
	ctx := context.Background()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE") })

	u, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := database.Open(ctx, database.Config{DSN: u.String()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db.DB
}

func mustExec(t *testing.T, db *pg.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(context.Background(), q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func count(t *testing.T, db *pg.DB, table string) int {
	t.Helper()
	rows, err := db.Query(context.Background(), `SELECT count(*) FROM `+table)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var n int
	if rows.Next() {
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
	}
	return n
}

const (
	ws1  = "11111111-1111-4111-8111-111111111111"
	ws2  = "22222222-2222-4222-8222-222222222222"
	ws3  = "33333333-3333-4333-8333-333333333333"
	usr1 = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
)

func TestStore_DeleteExpiredInvitations(t *testing.T) {
	s, db := openStore(t)
	mustExec(t, db, `INSERT INTO organizations (id, owner_id, created_at, updated_at, name, slug) VALUES ($1::uuid, $2::uuid, now(), now(), 'A', 'a')`, ws1, usr1)
	for i, exp := range []time.Duration{-time.Hour, -time.Minute, time.Hour} {
		mustExec(t, db, `INSERT INTO organization_invites (id, container_id, email, role_key, token_hash, invited_by, expires_at, created_at)
			VALUES (gen_random_uuid(), $1::uuid, $2, 'member', $3, $4::uuid, $5, now())`,
			ws1, string(rune('a'+i))+"@example.com", string(rune('a'+i)), usr1, now0.Add(exp))
	}
	n, err := s.DeleteExpiredInvitations(context.Background(), now0)
	if err != nil || n != 2 {
		t.Fatalf("deleted %d, err %v; want the 2 expired", n, err)
	}
	if left := count(t, db, "organization_invites"); left != 1 {
		t.Errorf("%d invitations left, want the live one", left)
	}
}

func TestStore_DeleteExpiredSessionsNeverTouchesLiveOnes(t *testing.T) {
	s, db := openStore(t)
	cutoff := now0.Add(-sessionGrace)
	rows := []struct {
		name    string
		expires time.Time
		rotated bool
	}{
		{"long expired", cutoff.Add(-time.Hour), false},
		{"long expired and rotated", cutoff.Add(-time.Hour), true},
		{"expired within grace", cutoff.Add(time.Hour), false},
		{"live", now0.Add(time.Hour), false},
		{"live but rotated (reuse detection needs it)", now0.Add(time.Hour), true},
	}
	for i, r := range rows {
		var rotated any
		if r.rotated {
			rotated = now0.Add(-time.Hour)
		}
		mustExec(t, db, `INSERT INTO sessions (id, user_id, token_hash, family_id, expires_at, created_at, rotated_at, user_agent, ip)
			VALUES (gen_random_uuid(), $1::uuid, $2, 'fam', $3, now(), $4, '', '')`, usr1, r.name+string(rune('0'+i)), r.expires, rotated)
	}
	n, err := s.DeleteExpiredSessions(context.Background(), cutoff)
	if err != nil || n != 2 {
		t.Fatalf("deleted %d, err %v; want 2", n, err)
	}
	if left := count(t, db, "sessions"); left != 3 {
		t.Errorf("%d sessions left, want 3 (grace + both live)", left)
	}
	var live int
	q, _ := db.Query(context.Background(), `SELECT count(*) FROM sessions WHERE expires_at > $1`, now0)
	defer func() { _ = q.Close() }()
	if q.Next() {
		_ = q.Scan(&live)
	}
	if live != 2 {
		t.Errorf("%d live sessions left, want 2", live)
	}
}

func TestStore_DeleteExpiredVerifications(t *testing.T) {
	s, db := openStore(t)
	for i, exp := range []time.Duration{-48 * time.Hour, -time.Hour, time.Hour} {
		mustExec(t, db, `INSERT INTO verifications (id, user_id, token_hash, purpose, email, expires_at, created_at)
			VALUES (gen_random_uuid(), $1::uuid, $2, 'email', 'a@example.com', $3, now())`, usr1, string(rune('a'+i)), now0.Add(exp))
	}
	n, err := s.DeleteExpiredVerifications(context.Background(), now0.Add(-verificationGrace))
	if err != nil || n != 1 {
		t.Fatalf("deleted %d, err %v; want 1", n, err)
	}
}

func TestStore_EndingTrials(t *testing.T) {
	s, db := openStore(t)
	for _, w := range []struct{ id, name string }{{ws1, "Soon"}, {ws2, "Later"}, {ws3, "Paid"}} {
		mustExec(t, db, `INSERT INTO organizations (id, owner_id, created_at, updated_at, name, slug) VALUES ($1::uuid, $2::uuid, now(), now(), $3, $3)`, w.id, usr1, w.name)
	}
	sub := func(ws, status string, end any) {
		mustExec(t, db, `INSERT INTO billing_subscriptions (workspace_id, status, current_period_end) VALUES ($1, $2, $3)`, ws, status, end)
	}
	sub(ws1, "trialing", now0.Add(48*time.Hour))
	sub(ws2, "trialing", now0.Add(10*24*time.Hour))
	sub(ws3, "active", now0.Add(24*time.Hour))
	sub("not-a-uuid", "trialing", now0.Add(24*time.Hour))                         // must not break the query
	sub("44444444-4444-4444-8444-444444444444", "trialing", now0.Add(-time.Hour)) // already over / no workspace

	got, err := s.EndingTrials(context.Background(), now0, now0.Add(trialWarning))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].WorkspaceID != ws1 || got[0].WorkspaceName != "Soon" || !got[0].EndsAt.Equal(now0.Add(48*time.Hour)) {
		t.Fatalf("trials = %+v, want only the workspace trialing and ending in 2 days", got)
	}
}

func TestStore_NotificationClaimIsAtomicAndReleasable(t *testing.T) {
	s, _ := openStore(t)
	ctx := context.Background()
	if first, err := s.Claim(ctx, "k", "a"); err != nil || !first {
		t.Fatalf("first claim = %v, %v", first, err)
	}
	if again, err := s.Claim(ctx, "k", "a"); err != nil || again {
		t.Fatalf("second claim = %v, %v; want refused", again, err)
	}
	if other, _ := s.Claim(ctx, "k2", "a"); !other {
		t.Error("claims of a different kind collide")
	}
	if err := s.Release(ctx, "k", "a"); err != nil {
		t.Fatal(err)
	}
	if again, _ := s.Claim(ctx, "k", "a"); !again {
		t.Error("a released claim cannot be taken again")
	}

	// concurrent claimers: exactly one wins
	wins := make(chan bool, 10)
	for range 10 {
		go func() { ok, _ := s.Claim(ctx, "race", "x"); wins <- ok }()
	}
	won := 0
	for range 10 {
		if <-wins {
			won++
		}
	}
	if won != 1 {
		t.Errorf("%d concurrent claims won, want 1", won)
	}
}
