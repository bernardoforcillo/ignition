package saas

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bernardoforcillo/ignition/go-packages/billing"
	"github.com/bernardoforcillo/ignition/go-packages/database"
	"github.com/bernardoforcillo/ignition/go-packages/database/dbtest"
	"github.com/bernardoforcillo/ignition/go-packages/features"
	"github.com/bernardoforcillo/ignition/go-packages/identity/account"
	"github.com/bernardoforcillo/ignition/go-packages/identity/auth"

	"github.com/bernardoforcillo/ignition/apps/gateway/internal/config"
	"github.com/bernardoforcillo/ignition/apps/gateway/internal/core"
)

func testConfig() config.SaaS {
	return config.SaaS{
		DatabaseURL: os.Getenv("TEST_DATABASE_URL"),
		AuthSecret:  []byte(strings.Repeat("s", 32)),
		AccessTTL:   15 * time.Minute,
		RefreshTTL:  time.Hour,
		AppURL:      "https://app.example.com",
		CompanyName: "Acme",
		MailFrom:    "Acme <hi@example.com>",
		Billing: &config.Billing{
			StripeWebhookSecret: "whsec_test",
			Prices: []config.PriceSpec{
				{ProviderPriceID: "price_pro", Kind: "plan", ID: "pro"},
				{ProviderPriceID: "price_extra", Kind: "addon", ID: "extra-api-calls"},
			},
		},
	}
}

// testPassword satisfies authlayer's default password rules.
const testPassword = "Correct-Horse-9!x"

type sentMail struct {
	To   []string `json:"to"`
	Text string   `json:"text"`
}

// mailbox stands in for Resend (through RESEND_BASE_URL) and keeps every email it is sent, so a
// test can follow the links the product mails out.
type mailbox struct {
	mu   sync.Mutex
	msgs []sentMail
}

func newMailbox(t *testing.T) (*mailbox, string) {
	t.Helper()
	mb := &mailbox{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m sentMail
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mb.mu.Lock()
		mb.msgs = append(mb.msgs, m)
		mb.mu.Unlock()
		_, _ = w.Write([]byte(`{"id":"em_1"}`))
	}))
	t.Cleanup(srv.Close)
	return mb, srv.URL
}

// token waits for the newest email to `to` carrying a link under path and returns its token.
func (mb *mailbox) token(t *testing.T, to, path string) string {
	t.Helper()
	re := regexp.MustCompile(regexp.QuoteMeta(path) + `\?token=([^\s&"]+)`)
	for range 100 {
		mb.mu.Lock()
		for i := len(mb.msgs) - 1; i >= 0; i-- {
			if m := mb.msgs[i]; slices.Contains(m.To, to) {
				if g := re.FindStringSubmatch(m.Text); g != nil {
					mb.mu.Unlock()
					tok, err := url.QueryUnescape(g[1])
					if err != nil {
						t.Fatal(err)
					}
					return tok
				}
			}
		}
		mb.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no email to %s with a %s link", to, path)
	return ""
}

// buildWithMailbox is Build over a database whose emails land in the returned mailbox.
func buildWithMailbox(t *testing.T) (*Services, *mailbox, *database.DB) {
	t.Helper()
	db := dbtest.Open(t)
	resetSchema(t, db)
	mb, baseURL := newMailbox(t)
	cfg := testConfig()
	cfg.ResendAPIKey, cfg.ResendBaseURL = "re_test", baseURL
	svc, err := Build(t.Context(), cfg, db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return svc, mb, db
}

// signUpVerified registers and verifies email through the real flow and returns the user id.
func signUpVerified(t *testing.T, svc *Services, mb *mailbox, email string) string {
	t.Helper()
	ctx := t.Context()
	if err := svc.Auth.SignUp(ctx, email, testPassword, "203.0.113.1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Auth.VerifyEmail(ctx, mb.token(t, email, "/verify-email")); err != nil {
		t.Fatal(err)
	}
	tok, err := svc.Auth.Login(ctx, email, testPassword, "203.0.113.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	return tok.User.ID
}

// resetSchema drops everything Build creates, so each run starts empty.
func resetSchema(t *testing.T, db *database.DB) {
	t.Helper()
	_, err := db.Exec(t.Context(), `DROP TABLE IF EXISTS
		billing_events, billing_customers, billing_subscriptions, feature_subscriptions, feature_usage,
		users, sessions, verifications, organizations, organization_members, organization_roles,
		organization_invites, organization_invite_links, schema_migrations CASCADE`)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
}

// TestBuild_AgainstPostgres is an integration test: it needs
// TEST_DATABASE_URL (a scratch database; it drops the tables it owns).
func TestBuild_AgainstPostgres(t *testing.T) {
	if testing.Short() {
		t.Skip("needs external service")
	}
	db := dbtest.Open(t)
	resetSchema(t, db)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := t.Context()

	// Building twice proves the migrations are idempotent.
	for range 2 {
		svc, err := Build(ctx, testConfig(), db, logger)
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		if svc.Billing == nil || svc.BillingWebhook == nil {
			t.Fatal("billing was configured but not wired")
		}
		if err := svc.Ready(ctx); err != nil {
			t.Fatalf("Ready: %v", err)
		}
	}
	svc, err := Build(ctx, testConfig(), db, logger)
	if err != nil {
		t.Fatal(err)
	}

	// A new workspace starts on the free plan: api.calls on, data.export off.
	ws, err := svc.Workspaces.Create(ctx, testUserID, "Acme", "")
	if err != nil {
		t.Fatalf("Create workspace: %v", err)
	}
	if err := svc.Features.Require(ctx, features.APICalls, ws.ID, testUserID); err != nil {
		t.Errorf("free plan should allow api.calls: %v", err)
	}
	if svc.Features.Allowed(ctx, features.DataExport, ws.ID, testUserID) {
		t.Error("free plan must not allow data.export")
	}
	if got, err := svc.Workspaces.Get(ctx, testUserID, ws.ID); err != nil || got.Slug != "acme" {
		t.Errorf("Get = %+v, %v", got, err)
	}

	// A billing event upgrades the workspace through sink + pgstore, and
	// replaying the same event id is a no-op.
	ev := billing.Event{
		ID: "evt_1", Type: billing.EventSubscriptionUpdated, WorkspaceID: ws.ID, CustomerID: "cus_1",
		Status: billing.StatusActive, PriceIDs: []string{"price_pro"}, PeriodStart: time.Now(),
	}
	for range 2 {
		if err := svc.Billing.HandleEvent(ctx, ev); err != nil {
			t.Fatalf("HandleEvent: %v", err)
		}
	}
	if !svc.Features.Allowed(ctx, features.DataExport, ws.ID, testUserID) {
		t.Error("pro plan should allow data.export after the billing event")
	}
}

func TestStores_AgainstPostgres(t *testing.T) {
	if testing.Short() {
		t.Skip("needs external service")
	}
	db := dbtest.Open(t)
	resetSchema(t, db)
	if err := database.Migrate(t.Context(), db, Migrations()...); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()

	events := &eventStore{db: db.DB}
	if seen, err := events.Seen(ctx, "evt_1"); err != nil || seen {
		t.Fatalf("fresh store: seen=%v err=%v", seen, err)
	}
	for range 2 { // recording twice must not fail
		if err := events.Record(ctx, "evt_1"); err != nil {
			t.Fatal(err)
		}
	}
	if seen, err := events.Seen(ctx, "evt_1"); err != nil || !seen {
		t.Fatalf("after Record: seen=%v err=%v", seen, err)
	}

	customers := &customerStore{db: db.DB}
	if _, err := customers.customer(ctx, "ws-1"); !errors.Is(err, core.ErrNoBillingCustomer) {
		t.Fatalf("unknown workspace: err = %v, want ErrNoBillingCustomer", err)
	}
	if err := customers.set(ctx, "ws-1", "cus_1"); err != nil {
		t.Fatal(err)
	}
	if err := customers.set(ctx, "ws-1", "cus_2"); err != nil {
		t.Fatal(err)
	}
	if got, err := customers.customer(ctx, "ws-1"); err != nil || got != "cus_2" {
		t.Fatalf("customer = %q, %v; want the latest", got, err)
	}
}

// testUserID is a uuid: authlayer's tables key users and owners by uuid.
const testUserID = "00000000-0000-4000-8000-000000000001"

func TestBillingStateStore_AgainstPostgres(t *testing.T) {
	if testing.Short() {
		t.Skip("needs external service")
	}
	db := dbtest.Open(t)
	resetSchema(t, db)
	if err := database.Migrate(t.Context(), db, Migrations()...); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	states := &stateStore{db: db.DB}

	if _, _, ok, err := states.state(ctx, "ws-1"); err != nil || ok {
		t.Fatalf("fresh store: ok=%v err=%v", ok, err)
	}
	end := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	if err := states.set(ctx, "ws-1", billing.StatusActive, end); err != nil {
		t.Fatal(err)
	}
	if err := states.set(ctx, "ws-1", billing.StatusPastDue, end.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	status, got, ok, err := states.state(ctx, "ws-1")
	if err != nil || !ok || status != "past_due" || !got.Equal(end.Add(time.Hour)) {
		t.Fatalf("state = %q %v %v %v; want the latest write", status, got, ok, err)
	}
	if err := states.set(ctx, "ws-2", billing.StatusCanceled, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, got, ok, err := states.state(ctx, "ws-2"); err != nil || !ok || !got.IsZero() {
		t.Fatalf("no period reported: end=%v ok=%v err=%v; want zero", got, ok, err)
	}
}

// TestAccountLifecycle_AgainstPostgres runs the real stores through export and erasure: what the
// SQL in directory.go touches must match the tables authlayer, features and billing create.
func TestAccountLifecycle_AgainstPostgres(t *testing.T) {
	if testing.Short() {
		t.Skip("needs external service")
	}
	svc, mb, db := buildWithMailbox(t)
	ctx := t.Context()
	ada := signUpVerified(t, svc, mb, "ada@example.com")
	bob := signUpVerified(t, svc, mb, "bob@example.com")

	alone, err := svc.Workspaces.Create(ctx, ada, "Alone", "")
	if err != nil {
		t.Fatal(err)
	}
	shared, err := svc.Workspaces.Create(ctx, ada, "Shared", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Workspaces.Invite(ctx, ada, shared.ID, "bob@example.com", "member"); err != nil {
		t.Fatal(err)
	}
	// An invitation to Ada from Bob's own workspace, still pending.
	bobs, _ := svc.Workspaces.Create(ctx, bob, "Bobs", "")
	if _, err := svc.Workspaces.Invite(ctx, bob, bobs.ID, "Ada@Example.com", "admin"); err != nil {
		t.Fatal(err)
	}

	data, _, err := svc.Account.Export(ctx, ada)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"email": "ada@example.com"`, alone.ID, shared.ID, `"workspace_name": "Bobs"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("export lacks %q:\n%s", want, data)
		}
	}
	if strings.Contains(string(data), bobs.ID) {
		t.Errorf("export lists a workspace Ada is only invited to as a membership")
	}

	// Bob accepts: Ada now owns a shared workspace and cannot be deleted.
	if _, err := svc.Workspaces.AcceptInvite(ctx, bob, mb.token(t, "bob@example.com", "/invite/accept")); err != nil {
		t.Fatal(err)
	}
	if err := svc.Account.Delete(ctx, ada, testPassword); !errors.Is(err, account.ErrOwnsSharedWorkspace) {
		t.Fatalf("Delete with a shared workspace: %v, want ErrOwnsSharedWorkspace", err)
	}

	// A billed workspace blocks the deletion too, until the subscription is gone.
	if err := svc.Workspaces.TransferOwnership(ctx, ada, shared.ID, bob); err != nil {
		t.Fatal(err)
	}
	emails, err := (&directory{db: db.DB}).Emails(ctx, []string{ada, bob, "not-a-uuid"})
	if err != nil {
		t.Fatal(err)
	}
	if len(emails) != 2 || emails[ada] != "ada@example.com" || emails[bob] == "" {
		t.Errorf("Emails = %v, want Ada's and Bob's addresses and nothing for a non-uuid", emails)
	}

	states := &stateStore{db: db.DB}
	if err := states.set(ctx, alone.ID, billing.StatusActive, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := svc.Account.Delete(ctx, ada, testPassword); !errors.Is(err, account.ErrActiveSubscription) {
		t.Fatalf("Delete with a billed workspace: %v, want ErrActiveSubscription", err)
	}
	if err := states.set(ctx, alone.ID, billing.StatusCanceled, time.Time{}); err != nil {
		t.Fatal(err)
	}

	if err := svc.Account.Delete(ctx, ada, testPassword); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Account.Me(ctx, ada); !errors.Is(err, auth.ErrAccountNotFound) {
		t.Errorf("Me after delete: %v", err)
	}
	if _, err := svc.Workspaces.Get(ctx, ada, alone.ID); err == nil {
		t.Error("the workspace Ada owned alone should be gone")
	}
	if members, err := svc.Workspaces.ListMembers(ctx, bob, shared.ID); err != nil || len(members) != 1 || members[0].UserID != bob {
		t.Errorf("shared workspace members = %v, %v; want only Bob", members, err)
	}
	for _, table := range []string{"feature_subscriptions", "billing_subscriptions"} {
		var n int
		rows, err := db.Query(ctx, `SELECT count(*) FROM `+table+` WHERE `+map[string]string{"feature_subscriptions": "tenant_id", "billing_subscriptions": "workspace_id"}[table]+` = $1`, alone.ID)
		if err != nil {
			t.Fatal(err)
		}
		rows.Next()
		_ = rows.Scan(&n)
		_ = rows.Close()
		if n != 0 {
			t.Errorf("%s still has %d rows for the erased workspace", table, n)
		}
	}
	dir := &directory{db: db.DB}
	if inv, err := dir.PendingInvitations(ctx, "ada@example.com"); err != nil || len(inv) != 0 {
		t.Errorf("invitations to the deleted address = %v, %v; want none", inv, err)
	}
}

func TestPasswordReset_AgainstPostgres(t *testing.T) {
	if testing.Short() {
		t.Skip("needs external service")
	}
	svc, mb, _ := buildWithMailbox(t)
	ctx := t.Context()
	signUpVerified(t, svc, mb, "ada@example.com")
	session, err := svc.Auth.Login(ctx, "ada@example.com", testPassword, "203.0.113.1", "laptop")
	if err != nil {
		t.Fatal(err)
	}

	for _, addr := range []string{"nobody@example.com", "ada@example.com"} {
		if err := svc.Auth.RequestPasswordReset(ctx, addr, "203.0.113.1"); err != nil {
			t.Fatalf("RequestPasswordReset(%s): %v", addr, err)
		}
	}
	tok := mb.token(t, "ada@example.com", "/reset-password")
	const next = "Brand-New-Horse-7!z"
	if err := svc.Auth.ResetPassword(ctx, tok, next); err != nil {
		t.Fatal(err)
	}

	if err := svc.Auth.ResetPassword(ctx, tok, next); !errors.Is(err, auth.ErrTokenInvalid) {
		t.Errorf("second use of the link: %v, want ErrTokenInvalid", err)
	}
	if _, err := svc.Auth.Refresh(ctx, session.RefreshToken); !errors.Is(err, auth.ErrTokenInvalid) {
		t.Errorf("a session survived the reset: %v", err)
	}
	if _, err := svc.Auth.Login(ctx, "ada@example.com", next, "203.0.113.1", "laptop"); err != nil {
		t.Errorf("login with the new password: %v", err)
	}
	mb.mu.Lock()
	defer mb.mu.Unlock()
	for _, m := range mb.msgs {
		if slices.Contains(m.To, "nobody@example.com") {
			t.Errorf("an unknown address was mailed: %v", m)
		}
	}
}
