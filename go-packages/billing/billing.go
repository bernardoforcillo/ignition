// Package billing is a provider-agnostic subscription-billing layer.
//
// A payment provider (Stripe, ...) tells us what a workspace pays for through
// webhooks. Service normalizes those events, maps provider price IDs to plan
// and add-on IDs via a Catalog, and pushes the result to a SubscriptionSink.
// The features/entitlements module adapts that sink; this package imports
// neither it nor any payment SDK.
package billing

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// Sentinel errors. Adapters wrap them so callers (notably the HTTP handler)
// can classify failures with errors.Is.
var (
	// ErrInvalidConfig reports a rejected catalog or adapter configuration.
	ErrInvalidConfig = errors.New("billing: invalid config")
	// ErrInvalidSignature reports a webhook whose signature did not verify.
	ErrInvalidSignature = errors.New("billing: invalid webhook signature")
	// ErrMalformedEvent reports a webhook payload that could not be decoded.
	ErrMalformedEvent = errors.New("billing: malformed event")
	// ErrIgnoredEvent reports a valid webhook of a type we do not act on.
	ErrIgnoredEvent = errors.New("billing: event type ignored")
	// ErrUnknownPrice reports a provider price missing from the catalog.
	ErrUnknownPrice = errors.New("billing: price not in catalog")
	// ErrAmbiguousPlan reports a subscription carrying several distinct plans.
	ErrAmbiguousPlan = errors.New("billing: subscription has several plans")
	// ErrMissingWorkspace reports an event that cannot be tied to a workspace.
	ErrMissingWorkspace = errors.New("billing: event has no workspace")
)

// Status is a subscription's lifecycle state, normalized across providers.
type Status string

const (
	StatusTrialing   Status = "trialing"
	StatusActive     Status = "active"
	StatusPastDue    Status = "past_due"
	StatusCanceled   Status = "canceled"
	StatusIncomplete Status = "incomplete"
)

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	switch s {
	case StatusTrialing, StatusActive, StatusPastDue, StatusCanceled, StatusIncomplete:
		return true
	}
	return false
}

// EventType is the normalized kind of a provider webhook.
type EventType string

const (
	EventSubscriptionCreated EventType = "subscription_created"
	EventSubscriptionUpdated EventType = "subscription_updated"
	EventSubscriptionDeleted EventType = "subscription_deleted"
	EventInvoicePaid         EventType = "invoice_paid"
	EventInvoiceFailed       EventType = "invoice_failed"
)

// Event is a provider webhook normalized by Provider.ParseWebhook. For invoice
// events the provider sets Status itself (paid -> active, failed -> past_due).
type Event struct {
	ID             string // provider event ID; the idempotency key
	Type           EventType
	WorkspaceID    string
	CustomerID     string
	SubscriptionID string
	Status         Status
	PriceIDs       []string // provider price IDs on the subscription
	TrialEnd       time.Time
	PeriodStart    time.Time
	PeriodEnd      time.Time
	OccurredAt     time.Time
}

// Subscription is what a workspace is entitled to, in billing's own terms.
type Subscription struct {
	PlanID      string
	AddOnIDs    []string
	Status      Status
	TrialEnd    time.Time // zero when not trialing
	PeriodStart time.Time
	PeriodEnd   time.Time
}

// CheckoutRequest asks the provider for a hosted checkout page.
type CheckoutRequest struct {
	WorkspaceID string
	PriceID     string // provider price ID, usually Catalog-known
	SuccessURL  string
	CancelURL   string
}

// Provider is the port a payment provider implements. ParseWebhook owns
// signature verification and must return ErrInvalidSignature on failure,
// ErrMalformedEvent on bad payloads and ErrIgnoredEvent for unhandled types.
type Provider interface {
	CreateCheckout(ctx context.Context, req CheckoutRequest) (url string, err error)
	CreatePortalSession(ctx context.Context, workspaceID, returnURL string) (url string, err error)
	ParseWebhook(ctx context.Context, payload []byte, headers http.Header) (Event, error)
}

// SubscriptionSink receives the desired subscription of a workspace. It must
// be idempotent (set, not increment): the same value may be pushed again.
// The features module adapts its entitlement subscription store to this port.
type SubscriptionSink interface {
	SetSubscription(ctx context.Context, workspaceID string, sub Subscription) error
}

// EventStore remembers processed event IDs so replays are no-ops. The app
// supplies a durable implementation (see README); billingtest has a memory one.
type EventStore interface {
	Seen(ctx context.Context, eventID string) (bool, error)
	Record(ctx context.Context, eventID string) error
}
