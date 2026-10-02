package stripe

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/bernardoforcillo/ignition/go-packages/billing"
)

const testSecret = "whsec_test_fake"

var fixedNow = time.Date(2030, 6, 1, 12, 0, 0, 0, time.UTC)

func sign(secret string, ts time.Time, payload string) string {
	t := strconv.FormatInt(ts.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(t + "." + payload))
	return fmt.Sprintf("t=%s,v1=%s", t, hex.EncodeToString(mac.Sum(nil)))
}

func newProvider(t *testing.T, mod func(*Config)) *Provider {
	t.Helper()
	cfg := Config{WebhookSecret: testSecret, Now: func() time.Time { return fixedNow }}
	if mod != nil {
		mod(&cfg)
	}
	p, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestVerifySignature(t *testing.T) {
	const body = `{"id":"evt_1"}`
	valid := sign(testSecret, fixedNow, body)
	rotated := "t=" + strconv.FormatInt(fixedNow.Unix(), 10) + ",v1=deadbeef," + valid[len("t="+strconv.FormatInt(fixedNow.Unix(), 10)+","):]
	tests := []struct {
		name    string
		payload string
		header  string
		wantErr bool
	}{
		{"valid", body, valid, false},
		{"valid among rotated v1 values", body, rotated, false},
		{"within tolerance", body, sign(testSecret, fixedNow.Add(-4*time.Minute), body), false},
		{"tampered payload", body + " ", valid, true},
		{"wrong secret", body, sign("whsec_other", fixedNow, body), true},
		{"stale timestamp", body, sign(testSecret, fixedNow.Add(-10*time.Minute), body), true},
		{"future timestamp", body, sign(testSecret, fixedNow.Add(10*time.Minute), body), true},
		{"missing header", body, "", true},
		{"no v1 part", body, "t=" + strconv.FormatInt(fixedNow.Unix(), 10), true},
		{"non-hex signature", body, "t=" + strconv.FormatInt(fixedNow.Unix(), 10) + ",v1=zz", true},
		{"bad timestamp", body, "t=abc,v1=00", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifySignature([]byte(tt.payload), tt.header, testSecret, fixedNow, DefaultTolerance)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, billing.ErrInvalidSignature) {
				t.Errorf("err = %v, want ErrInvalidSignature", err)
			}
		})
	}
}

func parse(t *testing.T, payload string) (billing.Event, error) {
	t.Helper()
	h := http.Header{}
	h.Set("Stripe-Signature", sign(testSecret, fixedNow, payload))
	return newProvider(t, nil).ParseWebhook(context.Background(), []byte(payload), h)
}

func TestParseWebhook_SubscriptionUpdated(t *testing.T) {
	ev, err := parse(t, `{"id":"evt_1","type":"customer.subscription.updated","created":1900000000,
	 "data":{"object":{"id":"sub_1","customer":"cus_1","status":"trialing","trial_end":1900100000,
	 "current_period_start":1900000000,"current_period_end":1902000000,
	 "metadata":{"workspace_id":"ws_1"},
	 "items":{"data":[{"price":{"id":"price_pro"}},{"price":{"id":"price_seats"}}]}}}}`)
	if err != nil {
		t.Fatal(err)
	}
	if ev.ID != "evt_1" || ev.Type != billing.EventSubscriptionUpdated || ev.Status != billing.StatusTrialing ||
		ev.WorkspaceID != "ws_1" || ev.CustomerID != "cus_1" || ev.SubscriptionID != "sub_1" ||
		len(ev.PriceIDs) != 2 || ev.PriceIDs[0] != "price_pro" ||
		ev.TrialEnd.Unix() != 1900100000 || ev.PeriodEnd.Unix() != 1902000000 {
		t.Fatalf("unexpected event: %+v", ev)
	}
}

func TestParseWebhook_StatusMapping(t *testing.T) {
	tests := map[string]billing.Status{
		"active": billing.StatusActive, "past_due": billing.StatusPastDue, "unpaid": billing.StatusPastDue,
		"canceled": billing.StatusCanceled, "incomplete_expired": billing.StatusCanceled,
		"incomplete": billing.StatusIncomplete, "trialing": billing.StatusTrialing,
	}
	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			ev, err := parse(t, `{"id":"e","type":"customer.subscription.created","data":{"object":{"status":"`+in+`"}}}`)
			if err != nil || ev.Status != want {
				t.Fatalf("status = %q err = %v, want %q", ev.Status, err, want)
			}
		})
	}
	if _, err := parse(t, `{"id":"e","type":"customer.subscription.created","data":{"object":{"status":"???"}}}`); !errors.Is(err, billing.ErrMalformedEvent) {
		t.Errorf("unknown status err = %v", err)
	}
}

func TestParseWebhook_Invoices(t *testing.T) {
	paid, err := parse(t, `{"id":"evt_2","type":"invoice.paid","data":{"object":{"customer":"cus_1","subscription":"sub_1",
	 "subscription_details":{"metadata":{"workspace_id":"ws_1"}},
	 "lines":{"data":[{"price":{"id":"price_pro"},"period":{"start":1900000000,"end":1902000000}}]}}}}`)
	if err != nil || paid.Type != billing.EventInvoicePaid || paid.Status != billing.StatusActive ||
		paid.WorkspaceID != "ws_1" || paid.SubscriptionID != "sub_1" || paid.PriceIDs[0] != "price_pro" || paid.PeriodEnd.Unix() != 1902000000 {
		t.Fatalf("paid: %+v err=%v", paid, err)
	}

	failed, err := parse(t, `{"id":"evt_3","type":"invoice.payment_failed","data":{"object":{"customer":"cus_1",
	 "parent":{"subscription_details":{"subscription":"sub_9","metadata":{"workspace_id":"ws_2"}}},
	 "lines":{"data":[{"pricing":{"price_details":{"price":"price_team"}}}]}}}}`)
	if err != nil || failed.Type != billing.EventInvoiceFailed || failed.Status != billing.StatusPastDue ||
		failed.WorkspaceID != "ws_2" || failed.SubscriptionID != "sub_9" || failed.PriceIDs[0] != "price_team" {
		t.Fatalf("failed: %+v err=%v", failed, err)
	}
}

func TestParseWebhook_ErrorClassification(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		sig     bool
		want    error
	}{
		{"ignored type", `{"id":"e","type":"charge.succeeded","data":{"object":{}}}`, true, billing.ErrIgnoredEvent},
		{"malformed json", `{nope`, true, billing.ErrMalformedEvent},
		{"missing event id", `{"type":"invoice.paid","data":{"object":{}}}`, true, billing.ErrMalformedEvent},
		{"unsigned", `{"id":"e","type":"invoice.paid","data":{"object":{}}}`, false, billing.ErrInvalidSignature},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.Header{}
			if tt.sig {
				h.Set("Stripe-Signature", sign(testSecret, fixedNow, tt.payload))
			}
			_, err := newProvider(t, nil).ParseWebhook(context.Background(), []byte(tt.payload), h)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestNew_RequiresWebhookSecret(t *testing.T) {
	if _, err := New(Config{}); !errors.Is(err, billing.ErrInvalidConfig) {
		t.Fatalf("err = %v", err)
	}
}

func TestCreateCheckout_PostsFormAndReturnsURL(t *testing.T) {
	var gotForm map[string][]string
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm, gotAuth, gotPath = r.PostForm, r.Header.Get("Authorization"), r.URL.Path
		_, _ = w.Write([]byte(`{"url":"https://checkout.test/s"}`))
	}))
	defer srv.Close()
	p := newProvider(t, func(c *Config) { c.BaseURL, c.HTTPClient, c.APIKey = srv.URL, srv.Client(), "sk_test_fake" })

	url, err := p.CreateCheckout(context.Background(), billing.CheckoutRequest{
		WorkspaceID: "ws_1", PriceID: "price_pro", SuccessURL: "https://app/ok", CancelURL: "https://app/no"})
	if err != nil || url != "https://checkout.test/s" {
		t.Fatalf("url=%q err=%v", url, err)
	}
	if gotPath != "/v1/checkout/sessions" || gotAuth != "Bearer sk_test_fake" ||
		gotForm["line_items[0][price]"][0] != "price_pro" ||
		gotForm["subscription_data[metadata][workspace_id]"][0] != "ws_1" {
		t.Errorf("bad request: path=%s auth=%s form=%v", gotPath, gotAuth, gotForm)
	}
}

func TestCreatePortalSession_ResolvesCustomerAndSurfacesAPIErrors(t *testing.T) {
	var gotCustomer string
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotCustomer = r.PostForm.Get("customer")
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = w.Write([]byte(`{"url":"https://portal.test/x"}`))
			return
		}
		_, _ = w.Write([]byte(`{"error":{"message":"no such customer"}}`))
	}))
	defer srv.Close()
	p := newProvider(t, func(c *Config) {
		c.BaseURL, c.HTTPClient = srv.URL, srv.Client()
		c.Customers = func(context.Context, string) (string, error) { return "cus_1", nil }
	})

	if url, err := p.CreatePortalSession(context.Background(), "ws_1", "https://app"); err != nil || url != "https://portal.test/x" || gotCustomer != "cus_1" {
		t.Fatalf("url=%q customer=%q err=%v", url, gotCustomer, err)
	}
	status = http.StatusBadRequest
	if _, err := p.CreatePortalSession(context.Background(), "ws_1", "https://app"); err == nil {
		t.Fatal("want error on 400")
	}
	if _, err := newProvider(t, nil).CreatePortalSession(context.Background(), "w", "u"); !errors.Is(err, billing.ErrInvalidConfig) {
		t.Errorf("no resolver err = %v", err)
	}
}
