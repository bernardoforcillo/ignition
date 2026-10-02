package billing_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/bernardoforcillo/ignition/go-packages/billing"
	"github.com/bernardoforcillo/ignition/go-packages/billing/billingtest"
)

func testCatalog(t *testing.T) *billing.Catalog {
	t.Helper()
	c, err := billing.NewCatalog(billing.CatalogConfig{
		FreePlanID: "free",
		Prices: []billing.Price{
			{ProviderPriceID: "price_pro", Kind: billing.KindPlan, ID: "pro"},
			{ProviderPriceID: "price_team", Kind: billing.KindPlan, ID: "team"},
			{ProviderPriceID: "price_seats", Kind: billing.KindAddOn, ID: "extra-seats"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

type fixture struct {
	svc   *billing.Service
	sink  *billingtest.RecordingSink
	store *billingtest.MemoryEventStore
	prov  *billingtest.FakeProvider
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{
		sink:  &billingtest.RecordingSink{},
		store: &billingtest.MemoryEventStore{},
		prov:  &billingtest.FakeProvider{CheckoutURL: "https://pay.test/c", PortalURL: "https://pay.test/p"},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	f.svc = billing.NewService(testCatalog(t), f.prov, f.sink, f.store, log)
	return f
}

func TestHandleEvent_StatusToSubscriptionMapping(t *testing.T) {
	end := time.Date(2030, 1, 2, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		ev       billing.Event
		wantPlan string
		wantAdd  []string
		wantStat billing.Status
	}{
		{"active paid plan with add-on", billing.Event{Type: billing.EventSubscriptionUpdated, Status: billing.StatusActive, PriceIDs: []string{"price_pro", "price_seats"}}, "pro", []string{"extra-seats"}, billing.StatusActive},
		{"trialing keeps paid plan", billing.Event{Type: billing.EventSubscriptionCreated, Status: billing.StatusTrialing, PriceIDs: []string{"price_team"}, TrialEnd: end}, "team", nil, billing.StatusTrialing},
		{"past_due keeps paid plan during grace", billing.Event{Type: billing.EventInvoiceFailed, Status: billing.StatusPastDue, PriceIDs: []string{"price_pro"}}, "pro", nil, billing.StatusPastDue},
		{"canceled falls back to free", billing.Event{Type: billing.EventSubscriptionUpdated, Status: billing.StatusCanceled, PriceIDs: []string{"price_pro"}}, "free", nil, billing.StatusCanceled},
		{"deleted falls back to free", billing.Event{Type: billing.EventSubscriptionDeleted, Status: billing.StatusActive, PriceIDs: []string{"price_pro"}}, "free", nil, billing.StatusCanceled},
		{"incomplete stays on free", billing.Event{Type: billing.EventSubscriptionCreated, Status: billing.StatusIncomplete, PriceIDs: []string{"price_pro"}}, "free", nil, billing.StatusIncomplete},
		{"duplicate add-on prices collapse", billing.Event{Type: billing.EventInvoicePaid, Status: billing.StatusActive, PriceIDs: []string{"price_pro", "price_seats", "price_seats"}}, "pro", []string{"extra-seats"}, billing.StatusActive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			tt.ev.ID, tt.ev.WorkspaceID, tt.ev.PeriodEnd = "evt_1", "ws_1", end
			if err := f.svc.HandleEvent(context.Background(), tt.ev); err != nil {
				t.Fatal(err)
			}
			got := f.sink.Subs["ws_1"]
			if got.PlanID != tt.wantPlan || got.Status != tt.wantStat || len(got.AddOnIDs) != len(tt.wantAdd) {
				t.Fatalf("got %+v, want plan=%s status=%s addons=%v", got, tt.wantPlan, tt.wantStat, tt.wantAdd)
			}
			if !got.PeriodEnd.Equal(end) || !got.TrialEnd.Equal(tt.ev.TrialEnd) {
				t.Errorf("periods not propagated: %+v", got)
			}
		})
	}
}

// The portal needs the provider's customer, and only events ever reveal it, so the sink must get it
// for a paying workspace and for one that lapsed back to the free plan.
func TestHandleEvent_ForwardsTheCustomerToTheSink(t *testing.T) {
	tests := []struct {
		name   string
		status billing.Status
		plan   string
	}{
		{"paying workspace", billing.StatusActive, "pro"},
		{"lapsed workspace keeps its customer", billing.StatusCanceled, "free"},
		{"incomplete checkout", billing.StatusIncomplete, "free"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			ev := billing.Event{ID: "evt_1", WorkspaceID: "ws_1", CustomerID: "cus_42", Status: tt.status, PriceIDs: []string{"price_pro"}}

			if err := f.svc.HandleEvent(context.Background(), ev); err != nil {
				t.Fatal(err)
			}

			got := f.sink.Subs["ws_1"]
			if got.CustomerID != "cus_42" || got.PlanID != tt.plan {
				t.Errorf("got %+v, want customer cus_42 on plan %s", got, tt.plan)
			}
		})
	}
}

func TestHandleEvent_RejectsUnusableEvents(t *testing.T) {
	tests := []struct {
		name string
		ev   billing.Event
		want error
	}{
		{"unknown price", billing.Event{ID: "e", WorkspaceID: "w", Status: billing.StatusActive, PriceIDs: []string{"price_nope"}}, billing.ErrUnknownPrice},
		{"no price at all", billing.Event{ID: "e", WorkspaceID: "w", Status: billing.StatusActive}, billing.ErrUnknownPrice},
		{"two plans", billing.Event{ID: "e", WorkspaceID: "w", Status: billing.StatusActive, PriceIDs: []string{"price_pro", "price_team"}}, billing.ErrAmbiguousPlan},
		{"no workspace", billing.Event{ID: "e", Status: billing.StatusActive, PriceIDs: []string{"price_pro"}}, billing.ErrMissingWorkspace},
		{"bad status", billing.Event{ID: "e", WorkspaceID: "w", Status: "weird", PriceIDs: []string{"price_pro"}}, billing.ErrMalformedEvent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			err := f.svc.HandleEvent(context.Background(), tt.ev)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if f.sink.Calls != 0 {
				t.Error("sink must not be called")
			}
		})
	}
}

func TestHandleEvent_ReplayIsNoOp(t *testing.T) {
	f := newFixture(t)
	ev := billing.Event{ID: "evt_1", WorkspaceID: "ws_1", Status: billing.StatusActive, PriceIDs: []string{"price_pro"}}
	for range 3 {
		if err := f.svc.HandleEvent(context.Background(), ev); err != nil {
			t.Fatal(err)
		}
	}
	if f.sink.Calls != 1 {
		t.Fatalf("sink calls = %d, want 1", f.sink.Calls)
	}
}

func TestHandleEvent_SinkFailureIsRetryable(t *testing.T) {
	f := newFixture(t)
	boom := errors.New("sink down")
	f.sink.Err = boom
	ev := billing.Event{ID: "evt_1", WorkspaceID: "ws_1", Status: billing.StatusActive, PriceIDs: []string{"price_pro"}}

	if err := f.svc.HandleEvent(context.Background(), ev); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want sink error", err)
	}
	f.sink.Err = nil
	if err := f.svc.HandleEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if f.sink.Subs["ws_1"].PlanID != "pro" {
		t.Error("retry did not apply the subscription")
	}
}

func TestHandleEvent_StoreErrorsAreReturned(t *testing.T) {
	boom := errors.New("db down")
	ev := billing.Event{ID: "e", WorkspaceID: "w", Status: billing.StatusActive, PriceIDs: []string{"price_pro"}}

	f := newFixture(t)
	f.store.FailSeen = boom
	if err := f.svc.HandleEvent(context.Background(), ev); !errors.Is(err, boom) || f.sink.Calls != 0 {
		t.Errorf("seen failure: err=%v calls=%d", err, f.sink.Calls)
	}

	f = newFixture(t)
	f.store.FailRecord = boom
	if err := f.svc.HandleEvent(context.Background(), ev); !errors.Is(err, boom) {
		t.Errorf("record failure: err=%v", err)
	}
}

func TestStartCheckoutAndOpenPortal_DelegateToProvider(t *testing.T) {
	f := newFixture(t)
	url, err := f.svc.StartCheckout(context.Background(), billing.CheckoutRequest{WorkspaceID: "ws_1", PriceID: "price_pro"})
	if err != nil || url != "https://pay.test/c" || f.prov.Checkout[0].WorkspaceID != "ws_1" {
		t.Fatalf("checkout: url=%q err=%v", url, err)
	}
	if url, err = f.svc.OpenPortal(context.Background(), "ws_1", "https://app.test"); err != nil || url != "https://pay.test/p" {
		t.Fatalf("portal: url=%q err=%v", url, err)
	}
	boom := errors.New("provider down")
	f.prov.Err = boom
	if _, err = f.svc.StartCheckout(context.Background(), billing.CheckoutRequest{}); !errors.Is(err, boom) {
		t.Errorf("err = %v, want wrapped provider error", err)
	}
}
