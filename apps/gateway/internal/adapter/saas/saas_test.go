package saas

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bernardoforcillo/authlayer/org"
	"github.com/bernardoforcillo/authlayer/store/memory"
	"github.com/bernardoforcillo/featurelayer/entitlement"

	"github.com/bernardoforcillo/ignition/go-packages/billing"
	"github.com/bernardoforcillo/ignition/go-packages/billing/billingtest"
	"github.com/bernardoforcillo/ignition/go-packages/features"
	"github.com/bernardoforcillo/ignition/go-packages/identity/permissions"
	"github.com/bernardoforcillo/ignition/go-packages/identity/workspace"

	"github.com/bernardoforcillo/ignition/apps/gateway/internal/config"
)

// fakeSubs is an in-memory subscriptionStore.
type fakeSubs struct {
	byTenant map[string]entitlement.Subscription
	setErr   error
}

func newFakeSubs() *fakeSubs { return &fakeSubs{byTenant: map[string]entitlement.Subscription{}} }

func (f *fakeSubs) Subscription(_ context.Context, id string) (*entitlement.Subscription, error) {
	s, ok := f.byTenant[id]
	if !ok {
		return nil, entitlement.ErrNoSubscription
	}
	return &s, nil
}

func (f *fakeSubs) Set(_ context.Context, sub entitlement.Subscription) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.byTenant[sub.TenantID] = sub
	return nil
}

func TestToEntitlement_MapsBillingSubscription(t *testing.T) {
	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		in   billing.Subscription
		want entitlement.Subscription
	}{
		{
			name: "paid plan with add-ons",
			in: billing.Subscription{PlanID: "pro", AddOnIDs: []string{"extra-api-calls"}, Status: billing.StatusActive,
				PeriodStart: start, PeriodEnd: start.AddDate(0, 1, 0)},
			want: entitlement.Subscription{TenantID: "ws-1", Plan: "pro", AddOns: []entitlement.AddOnID{"extra-api-calls"}, BillingAnchor: start},
		},
		{
			name: "canceled falls back to the plan billing chose",
			in:   billing.Subscription{PlanID: "free", Status: billing.StatusCanceled, PeriodStart: start},
			want: entitlement.Subscription{TenantID: "ws-1", Plan: "free", BillingAnchor: start},
		},
		{
			name: "trialing paid plan needs no featurelayer trial",
			in:   billing.Subscription{PlanID: "pro", Status: billing.StatusTrialing, TrialEnd: start.AddDate(0, 0, 14), PeriodStart: start},
			want: entitlement.Subscription{TenantID: "ws-1", Plan: "pro", BillingAnchor: start},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := toEntitlement("ws-1", tc.in)
			if got.TenantID != tc.want.TenantID || got.Plan != tc.want.Plan || !got.BillingAnchor.Equal(tc.want.BillingAnchor) ||
				len(got.AddOns) != len(tc.want.AddOns) || got.Trial != nil {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
			for i := range got.AddOns {
				if got.AddOns[i] != tc.want.AddOns[i] {
					t.Fatalf("add-ons %v, want %v", got.AddOns, tc.want.AddOns)
				}
			}
		})
	}
}

func TestSubscriptionSink_KeepsExistingGrants(t *testing.T) {
	subs := newFakeSubs()
	grant := entitlement.Override("data.export", nil, "support override")
	subs.byTenant["ws-1"] = entitlement.Subscription{TenantID: "ws-1", Plan: "free", Grants: []entitlement.Grant{grant}}
	sink := &subscriptionSink{store: subs}

	if err := sink.SetSubscription(t.Context(), "ws-1", billing.Subscription{PlanID: "pro", Status: billing.StatusActive}); err != nil {
		t.Fatal(err)
	}
	got := subs.byTenant["ws-1"]
	if got.Plan != "pro" || len(got.Grants) != 1 || got.Grants[0].Reason != "support override" {
		t.Fatalf("stored %+v: plan must change, grants must survive", got)
	}
}

func TestSubscriptionSink_FirstSubscriptionForAWorkspace(t *testing.T) {
	subs := newFakeSubs()
	sink := &subscriptionSink{store: subs}
	if err := sink.SetSubscription(t.Context(), "ws-new", billing.Subscription{PlanID: "pro"}); err != nil {
		t.Fatal(err)
	}
	if subs.byTenant["ws-new"].Plan != "pro" {
		t.Fatalf("stored %+v", subs.byTenant["ws-new"])
	}
}

func TestSubscriptionSink_StoreFailureIsReturnedSoTheProviderRetries(t *testing.T) {
	subs := newFakeSubs()
	subs.setErr = errors.New("db down")
	sink := &subscriptionSink{store: subs}
	if err := sink.SetSubscription(t.Context(), "ws-1", billing.Subscription{PlanID: "pro"}); err == nil {
		t.Fatal("a failed write must surface")
	}
}

func newTestWorkspaces(subs subscriptionStore) *Workspaces {
	svc := workspace.NewService(
		permissions.NewAccess(),
		memory.New[org.Organization, org.Member](),
		memory.NewInviteStore(),
		nil, "https://app.example.com",
	)
	return &Workspaces{Service: svc, subs: subs, freePlan: features.PlanFree}
}

func TestWorkspaces_CreateStartsOnTheFreePlan(t *testing.T) {
	subs := newFakeSubs()
	w := newTestWorkspaces(subs)

	ws, err := w.Create(t.Context(), "user-1", "Acme", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := subs.byTenant[ws.ID]; got.Plan != features.PlanFree {
		t.Fatalf("subscription = %+v, want the free plan for %s", got, ws.ID)
	}
}

func TestWorkspaces_CreateReportsAFailedProvisioning(t *testing.T) {
	subs := newFakeSubs()
	subs.setErr = errors.New("db down")
	if _, err := newTestWorkspaces(subs).Create(t.Context(), "user-1", "Acme", ""); err == nil {
		t.Fatal("a workspace left without a plan must not look like a success")
	}
}

func TestWorkspaces_InvalidInputDoesNotProvision(t *testing.T) {
	subs := newFakeSubs()
	if _, err := newTestWorkspaces(subs).Create(t.Context(), "user-1", "  ", ""); !errors.Is(err, workspace.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
	if len(subs.byTenant) != 0 {
		t.Fatalf("subscriptions written for a failed create: %v", subs.byTenant)
	}
}

func TestMemoryLimiter_FixedWindowPerKey(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := newMemoryLimiter(func() time.Time { return now })
	allow := func(key string) bool {
		ok, err := l.Allow(t.Context(), key, 2, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}

	if !allow("a") || !allow("a") {
		t.Fatal("first two requests must pass")
	}
	if allow("a") {
		t.Fatal("third request in the window must be refused")
	}
	if !allow("b") {
		t.Fatal("another key has its own budget")
	}
	now = now.Add(time.Minute)
	if !allow("a") {
		t.Fatal("a new window resets the budget")
	}
}

func TestMemoryLimiter_SweepsExpiredWindows(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := newMemoryLimiter(func() time.Time { return now })
	for _, k := range []string{"a", "b", "c"} {
		_, _ = l.Allow(t.Context(), k, 1, time.Minute)
	}
	now = now.Add(2 * time.Minute)
	_, _ = l.Allow(t.Context(), "d", 1, time.Minute)
	if len(l.windows) != 1 {
		t.Fatalf("%d windows kept, want only the live one", len(l.windows))
	}
}

func testCatalog(t *testing.T) *billing.Catalog {
	t.Helper()
	c, err := billing.NewCatalog(billing.CatalogConfig{
		FreePlanID: "free",
		Prices:     []billing.Price{{ProviderPriceID: "price_pro", Kind: billing.KindPlan, ID: "pro"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCheckout_RejectsPricesOutsideTheCatalog(t *testing.T) {
	prov := &billingtest.FakeProvider{}
	svc := billing.NewService(testCatalog(t), prov, &billingtest.RecordingSink{}, &billingtest.MemoryEventStore{}, discard())
	c := &Checkout{Service: svc, catalog: testCatalog(t)}

	if _, err := c.StartCheckout(t.Context(), billing.CheckoutRequest{WorkspaceID: "ws-1", PriceID: "price_other"}); !errors.Is(err, billing.ErrUnknownPrice) {
		t.Fatalf("err = %v, want ErrUnknownPrice", err)
	}
	if _, err := c.StartCheckout(t.Context(), billing.CheckoutRequest{WorkspaceID: "ws-1", PriceID: "price_pro"}); err != nil {
		t.Fatalf("catalog price rejected: %v", err)
	}
}

func TestNewCatalog_ValidatesAgainstTheFeatureCatalog(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.Billing
		free    string
		wantErr string
	}{
		{"known plan and add-on", config.Billing{Prices: []config.PriceSpec{
			{ProviderPriceID: "p1", Kind: "plan", ID: "pro"}, {ProviderPriceID: "p2", Kind: "addon", ID: "extra-api-calls"},
		}}, "free", ""},
		{"no prices is fine", config.Billing{}, "free", ""},
		{"unknown plan", config.Billing{Prices: []config.PriceSpec{{ProviderPriceID: "p1", Kind: "plan", ID: "gold"}}}, "free", "gold"},
		{"unknown add-on", config.Billing{Prices: []config.PriceSpec{{ProviderPriceID: "p1", Kind: "addon", ID: "nope"}}}, "free", "nope"},
		{"unknown free plan", config.Billing{}, "starter", "starter"},
		{"duplicate price", config.Billing{Prices: []config.PriceSpec{
			{ProviderPriceID: "p1", Kind: "plan", ID: "pro"}, {ProviderPriceID: "p1", Kind: "plan", ID: "pro"},
		}}, "free", "duplicate"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newCatalog(tc.cfg, entitlement.PlanID(tc.free))
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("err = %v, want one mentioning %q", err, tc.wantErr)
			}
		})
	}
}

type fakeParser struct {
	ev  billing.Event
	err error
}

func (p fakeParser) ParseWebhook(context.Context, []byte, http.Header) (billing.Event, error) {
	return p.ev, p.err
}

type fakeCustomers struct {
	got    map[string]string
	setErr error
}

func (f *fakeCustomers) set(_ context.Context, ws, cust string) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.got[ws] = cust
	return nil
}

func TestCustomerRecorder(t *testing.T) {
	tests := []struct {
		name     string
		parser   fakeParser
		setErr   error
		wantErr  bool
		wantSeen map[string]string
	}{
		{"verified event records the customer", fakeParser{ev: billing.Event{ID: "e1", WorkspaceID: "ws-1", CustomerID: "cus_1"}}, nil, false, map[string]string{"ws-1": "cus_1"}},
		{"event without a customer records nothing", fakeParser{ev: billing.Event{ID: "e1", WorkspaceID: "ws-1"}}, nil, false, map[string]string{}},
		{"unverified event records nothing", fakeParser{err: billing.ErrInvalidSignature}, nil, true, map[string]string{}},
		{"write failure fails the webhook so it is retried", fakeParser{ev: billing.Event{WorkspaceID: "ws-1", CustomerID: "cus_1"}}, errors.New("db"), true, map[string]string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeCustomers{got: map[string]string{}, setErr: tc.setErr}
			_, err := customerRecorder{inner: tc.parser, customers: store}.ParseWebhook(t.Context(), nil, nil)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if len(store.got) != len(tc.wantSeen) || store.got["ws-1"] != tc.wantSeen["ws-1"] {
				t.Fatalf("recorded %v, want %v", store.got, tc.wantSeen)
			}
		})
	}
}

func discard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
