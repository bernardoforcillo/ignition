// Package billingtest holds hand-written fakes for tests of billing users.
package billingtest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/bernardoforcillo/ignition/go-packages/billing"
)

// SignatureHeader carries FakeProvider.Secret on fake webhooks.
const SignatureHeader = "X-Fake-Signature"

// FakeProvider implements billing.Provider. Webhook payloads are the JSON
// encoding of billing.Event (see Payload), "signed" by a shared secret header.
type FakeProvider struct {
	Secret      string
	CheckoutURL string
	PortalURL   string
	Err         error // returned by CreateCheckout/CreatePortalSession when set

	mu       sync.Mutex
	Checkout []billing.CheckoutRequest
}

var _ billing.Provider = (*FakeProvider)(nil)

// Payload encodes ev as a FakeProvider webhook body.
func Payload(ev billing.Event) []byte {
	b, err := json.Marshal(ev)
	if err != nil {
		panic(err) // Event is always marshalable
	}
	return b
}

func (p *FakeProvider) CreateCheckout(_ context.Context, req billing.CheckoutRequest) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Checkout = append(p.Checkout, req)
	if p.Err != nil {
		return "", p.Err
	}
	return p.CheckoutURL, nil
}

func (p *FakeProvider) CreatePortalSession(_ context.Context, _, _ string) (string, error) {
	if p.Err != nil {
		return "", p.Err
	}
	return p.PortalURL, nil
}

func (p *FakeProvider) ParseWebhook(_ context.Context, payload []byte, headers http.Header) (billing.Event, error) {
	if headers.Get(SignatureHeader) != p.Secret {
		return billing.Event{}, billing.ErrInvalidSignature
	}
	var ev billing.Event
	if err := json.Unmarshal(payload, &ev); err != nil {
		return billing.Event{}, fmt.Errorf("%w: %v", billing.ErrMalformedEvent, err)
	}
	return ev, nil
}

// MemoryEventStore is an in-memory billing.EventStore. Fail* inject errors.
type MemoryEventStore struct {
	FailSeen, FailRecord error

	mu   sync.Mutex
	seen map[string]bool
}

var _ billing.EventStore = (*MemoryEventStore)(nil)

func (s *MemoryEventStore) Seen(_ context.Context, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.FailSeen != nil {
		return false, s.FailSeen
	}
	return s.seen[id], nil
}

func (s *MemoryEventStore) Record(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.FailRecord != nil {
		return s.FailRecord
	}
	if s.seen == nil {
		s.seen = map[string]bool{}
	}
	s.seen[id] = true
	return nil
}

// RecordingSink is a billing.SubscriptionSink keeping the latest subscription
// per workspace and the number of pushes. Err is returned when set.
type RecordingSink struct {
	Err error

	mu    sync.Mutex
	Subs  map[string]billing.Subscription
	Calls int
}

var _ billing.SubscriptionSink = (*RecordingSink)(nil)

func (s *RecordingSink) SetSubscription(_ context.Context, workspaceID string, sub billing.Subscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Err != nil {
		return s.Err
	}
	if s.Subs == nil {
		s.Subs = map[string]billing.Subscription{}
	}
	s.Subs[workspaceID] = sub
	s.Calls++
	return nil
}
