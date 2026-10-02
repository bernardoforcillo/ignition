package saas

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bernardoforcillo/ignition/go-packages/billing"
	"github.com/bernardoforcillo/ignition/go-packages/database"
	"github.com/bernardoforcillo/ignition/go-packages/database/dbtest"
	"github.com/bernardoforcillo/ignition/go-packages/features"

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

// resetSchema drops everything Build creates, so each run starts empty.
func resetSchema(t *testing.T, db *database.DB) {
	t.Helper()
	_, err := db.Exec(t.Context(), `DROP TABLE IF EXISTS
		billing_events, billing_customers, feature_subscriptions, feature_usage, schema_migrations CASCADE`)
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
