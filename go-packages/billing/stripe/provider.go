package stripe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bernardoforcillo/ignition/go-packages/billing"
)

const defaultBaseURL = "https://api.stripe.com"

// CustomerResolver maps a workspace to its Stripe customer ID (stored by the
// app, e.g. from the checkout.session / subscription events' CustomerID).
type CustomerResolver func(ctx context.Context, workspaceID string) (customerID string, err error)

// Config holds the adapter's dependencies; nothing is read from the environment.
type Config struct {
	APIKey        string // restricted key; only used for Checkout/portal calls
	WebhookSecret string // "whsec_..." signing secret of the endpoint
	Customers     CustomerResolver
	HTTPClient    *http.Client     // nil: a client with a 10s timeout
	BaseURL       string           // empty: Stripe's API; tests point at httptest
	Tolerance     time.Duration    // zero: DefaultTolerance
	Now           func() time.Time // nil: time.Now
}

// Provider implements billing.Provider for Stripe.
type Provider struct {
	cfg Config
}

var _ billing.Provider = (*Provider)(nil)

// New validates cfg and returns the Stripe provider.
func New(cfg Config) (*Provider, error) {
	if cfg.WebhookSecret == "" {
		return nil, fmt.Errorf("%w: stripe webhook secret is required", billing.ErrInvalidConfig)
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	if cfg.Tolerance == 0 {
		cfg.Tolerance = DefaultTolerance
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Provider{cfg: cfg}, nil
}

// ParseWebhook verifies the Stripe-Signature header, then normalizes
// customer.subscription.{created,updated,deleted}, invoice.paid and
// invoice.payment_failed. Other types yield billing.ErrIgnoredEvent.
func (p *Provider) ParseWebhook(_ context.Context, payload []byte, headers http.Header) (billing.Event, error) {
	err := verifySignature(payload, headers.Get("Stripe-Signature"), p.cfg.WebhookSecret, p.cfg.Now(), p.cfg.Tolerance)
	if err != nil {
		return billing.Event{}, err
	}
	return parseEvent(payload)
}

// CreateCheckout starts a subscription-mode Checkout Session and tags the
// resulting subscription with the workspace ID so webhooks can route back.
func (p *Provider) CreateCheckout(ctx context.Context, req billing.CheckoutRequest) (string, error) {
	form := url.Values{
		"mode":                    {"subscription"},
		"line_items[0][price]":    {req.PriceID},
		"line_items[0][quantity]": {"1"},
		"success_url":             {req.SuccessURL},
		"cancel_url":              {req.CancelURL},
		"client_reference_id":     {req.WorkspaceID},
		"subscription_data[metadata][" + WorkspaceMetadataKey + "]": {req.WorkspaceID},
	}
	return p.postForURL(ctx, "/v1/checkout/sessions", form)
}

// CreatePortalSession opens the Stripe billing portal for the workspace's customer.
func (p *Provider) CreatePortalSession(ctx context.Context, workspaceID, returnURL string) (string, error) {
	if p.cfg.Customers == nil {
		return "", fmt.Errorf("%w: no customer resolver configured", billing.ErrInvalidConfig)
	}
	customer, err := p.cfg.Customers(ctx, workspaceID)
	if err != nil {
		return "", fmt.Errorf("resolve stripe customer for workspace %q: %w", workspaceID, err)
	}
	return p.postForURL(ctx, "/v1/billing_portal/sessions",
		url.Values{"customer": {customer}, "return_url": {returnURL}})
}

func (p *Provider) postForURL(ctx context.Context, path string, form url.Values) (string, error) {
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.BaseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("build stripe request: %w", err)
	}
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	resp, err := p.cfg.HTTPClient.Do(r)
	if err != nil {
		return "", fmt.Errorf("call stripe %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read stripe %s response: %w", path, err)
	}
	var out struct {
		URL   string `json:"url"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &out); err != nil && resp.StatusCode < 300 {
		return "", fmt.Errorf("decode stripe %s response: %w", path, err)
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("stripe %s: status %d: %w", path, resp.StatusCode, errors.New(out.Error.Message))
	}
	if out.URL == "" {
		return "", fmt.Errorf("stripe %s: response has no url", path)
	}
	return out.URL, nil
}
