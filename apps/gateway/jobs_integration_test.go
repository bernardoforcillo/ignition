package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bernardoforcillo/drops/pg"

	"github.com/bernardoforcillo/ignition/go-packages/database"
	"github.com/bernardoforcillo/ignition/go-packages/database/dbtest"
	jobslib "github.com/bernardoforcillo/ignition/go-packages/jobs"

	jobsadapter "github.com/bernardoforcillo/ignition/apps/gateway/internal/adapter/jobs"
	"github.com/bernardoforcillo/ignition/apps/gateway/internal/adapter/saas"
	"github.com/bernardoforcillo/ignition/apps/gateway/internal/config"
)

// syncBuffer is a log sink two schedulers can write to.
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

// scratchDB opens the test database on a private schema (dropped afterwards), so this test does
// not collide with the other packages' tests that share TEST_DATABASE_URL.
func scratchDB(t *testing.T) *database.DB {
	t.Helper()
	admin := dbtest.Open(t) // skips without TEST_DATABASE_URL
	var b [6]byte
	_, _ = rand.Read(b[:])
	schema := "gateway_jobs_" + hex.EncodeToString(b[:])
	if _, err := admin.Exec(t.Context(), "CREATE SCHEMA "+schema); err != nil {
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
	db, err := database.Open(t.Context(), database.Config{DSN: u.String()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

type resendMail struct {
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
}

func exec(t *testing.T, db *pg.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(t.Context(), q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func scalar(t *testing.T, db *pg.DB, q string, args ...any) int {
	t.Helper()
	rows, err := db.Query(t.Context(), q, args...)
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

// TestBackgroundJobs_AgainstPostgres runs the real tasks, over the real migrations and the real
// mailer pointed at a fake Resend, in two scheduler instances (two replicas) sharing one database:
// the trial reminder reaches the owner exactly once and the cleanups delete only what expired.
func TestBackgroundJobs_AgainstPostgres(t *testing.T) {
	if testing.Short() {
		t.Skip("needs external service")
	}
	db := scratchDB(t)
	ctx := t.Context()

	var mu sync.Mutex
	var mails []resendMail
	resend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m resendMail
		_ = json.NewDecoder(r.Body).Decode(&m)
		mu.Lock()
		mails = append(mails, m)
		mu.Unlock()
		_, _ = w.Write([]byte(`{"id":"em_1"}`))
	}))
	defer resend.Close()

	logs := &syncBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	cfg := config.SaaS{
		AuthSecret: []byte(strings.Repeat("s", 32)), AccessTTL: time.Minute, RefreshTTL: time.Hour,
		AppURL: "https://app.example.com", CompanyName: "Acme", MailFrom: "Acme <hi@example.com>",
		ResendAPIKey: "re_test", ResendBaseURL: resend.URL,
	}
	svc, err := saas.Build(ctx, cfg, db, logger)
	if err != nil {
		t.Fatal(err)
	}

	// Accounts: ada owns the workspace whose trial ends in 2 days; bob is only a member of it;
	// carl owns one trialing for 10 days; dee owns one already paying.
	users := map[string]string{}
	for _, name := range []string{"ada", "bob", "carl", "dee"} {
		var id string
		rows, err := db.Query(ctx, `INSERT INTO users (id, email, password_hash, created_at, updated_at)
			VALUES (gen_random_uuid(), $1, 'x', now(), now()) RETURNING id::text`, name+"@example.com")
		if err != nil {
			t.Fatal(err)
		}
		if rows.Next() {
			_ = rows.Scan(&id)
		}
		_ = rows.Close()
		users[name] = id
	}
	workspace := func(owner, name string) string {
		ws, err := svc.Workspaces.Create(ctx, users[owner], name, "")
		if err != nil {
			t.Fatal(err)
		}
		return ws.ID
	}
	wsAda, wsCarl, wsDee := workspace("ada", "Ada Co"), workspace("carl", "Carl Co"), workspace("dee", "Dee Co")
	exec(t, db.DB, `INSERT INTO organization_members (container_id, user_id, role_key, joined_at) VALUES ($1::uuid, $2::uuid, 'member', now())`, wsAda, users["bob"])
	now := time.Now()
	for _, s := range []struct {
		ws, status string
		end        time.Time
	}{
		{wsAda, "trialing", now.Add(48 * time.Hour)},
		{wsCarl, "trialing", now.Add(10 * 24 * time.Hour)},
		{wsDee, "active", now.Add(24 * time.Hour)},
	} {
		exec(t, db.DB, `INSERT INTO billing_subscriptions (workspace_id, status, current_period_end) VALUES ($1, $2, $3)`, s.ws, s.status, s.end)
	}

	// Rows for the cleanups: one expired and one live invitation / session / token each.
	for i, exp := range []time.Duration{-time.Hour, time.Hour} {
		n := string(rune('a' + i))
		exec(t, db.DB, `INSERT INTO organization_invites (id, container_id, email, role_key, token_hash, invited_by, expires_at, created_at)
			VALUES (gen_random_uuid(), $1::uuid, $2, 'member', $3, $4::uuid, $5, now())`, wsAda, n+"@example.com", n, users["ada"], now.Add(exp))
		exec(t, db.DB, `INSERT INTO verifications (id, user_id, token_hash, purpose, email, expires_at, created_at)
			VALUES (gen_random_uuid(), $1::uuid, $2, 'email', 'x@example.com', $3, now())`, users["ada"], n, now.Add(exp*48))
	}
	for i, exp := range []time.Duration{-30 * 24 * time.Hour, -time.Hour, time.Hour} {
		exec(t, db.DB, `INSERT INTO sessions (id, user_id, token_hash, family_id, expires_at, created_at, user_agent, ip)
			VALUES (gen_random_uuid(), $1::uuid, $2, 'f', $3, now(), '', '')`, users["ada"], string(rune('a'+i)), now.Add(exp))
	}

	// Two replicas, every task due every second.
	var runners []*jobsadapter.Runner
	for range 2 {
		r, err := jobsadapter.New(
			jobsadapter.Deps{DB: svc.DB(), Owners: svc, Mail: svc.Mail, AppURL: cfg.AppURL, Logger: logger},
			jobsadapter.WithEvery(time.Second), jobsadapter.WithSchedulerOptions(jobslib.WithJitter(0)),
		)
		if err != nil {
			t.Fatal(err)
		}
		if err := r.Start(ctx); err != nil {
			t.Fatal(err)
		}
		runners = append(runners, r)
	}
	deadline := time.Now().Add(10 * time.Second)
	for scalar(t, db.DB, `SELECT coalesce(max(run_count), 0) FROM scheduled_job_runs WHERE name = 'trials.remind'`) < 3 {
		if time.Now().After(deadline) {
			t.Fatal("trials.remind did not run three times")
		}
		time.Sleep(50 * time.Millisecond)
	}
	for _, r := range runners {
		if err := r.Stop(ctx); err != nil {
			t.Fatal(err)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(mails) != 1 {
		t.Fatalf("%d emails reached Resend over 3 runs of 2 replicas, want exactly 1: %+v", len(mails), mails)
	}
	m := mails[0]
	if len(m.To) != 1 || m.To[0] != "ada@example.com" || m.Subject != "Your Ada Co trial is ending soon" || !strings.Contains(m.Text, "https://app.example.com/app/billing") {
		t.Errorf("email = %+v", m)
	}
	if got := scalar(t, db.DB, `SELECT count(*) FROM job_notifications`); got != 1 {
		t.Errorf("%d notification rows, want 1", got)
	}
	if got := scalar(t, db.DB, `SELECT count(*) FROM organization_invites`); got != 1 {
		t.Errorf("%d invitations left, want the live one", got)
	}
	if got := scalar(t, db.DB, `SELECT count(*) FROM sessions`); got != 2 {
		t.Errorf("%d sessions left, want the live one and the one inside the grace period", got)
	}
	if got := scalar(t, db.DB, `SELECT count(*) FROM verifications`); got != 1 {
		t.Errorf("%d verifications left, want the live one", got)
	}
	for _, name := range []string{"invitations.cleanup", "sessions.cleanup", "trials.remind"} {
		if scalar(t, db.DB, `SELECT count(*) FROM scheduled_job_runs WHERE name = $1 AND last_status = 'succeeded'`, name) != 1 {
			t.Errorf("task %s has no succeeded run recorded", name)
		}
	}
	if strings.Contains(logs.String(), "@example.com") {
		t.Errorf("an address reached the logs:\n%s", logs.String())
	}
	if strings.Contains(logs.String(), `"level":"ERROR"`) {
		t.Errorf("error logged:\n%s", logs.String())
	}
}
