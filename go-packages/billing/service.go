package billing

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
)

// Service turns provider events into subscription state for the sink.
//
// Status policy (what the sink receives):
//   - trialing, active: the paid plan and its add-ons.
//   - past_due: the paid plan is kept (grace while the provider retries the
//     payment); Status tells entitlements it is at risk.
//   - canceled (or a deleted subscription): the free plan, no add-ons.
//   - incomplete: the free plan until the first payment succeeds.
//
// Events are processed at least once: the event ID is recorded only after the
// sink accepted the subscription, so a failed push is retried by the provider.
// Delivery order is not guaranteed by providers; an older event arriving late
// can overwrite newer state until the next event corrects it.
type Service struct {
	catalog  *Catalog
	provider Provider
	sink     SubscriptionSink
	store    EventStore
	log      *slog.Logger
}

// NewService wires a Service from its ports.
func NewService(catalog *Catalog, provider Provider, sink SubscriptionSink, store EventStore, log *slog.Logger) *Service {
	return &Service{catalog: catalog, provider: provider, sink: sink, store: store, log: log}
}

// StartCheckout returns the provider's hosted checkout URL.
func (s *Service) StartCheckout(ctx context.Context, req CheckoutRequest) (string, error) {
	url, err := s.provider.CreateCheckout(ctx, req)
	if err != nil {
		return "", fmt.Errorf("start checkout for workspace %q: %w", req.WorkspaceID, err)
	}
	return url, nil
}

// OpenPortal returns the provider's customer-portal URL.
func (s *Service) OpenPortal(ctx context.Context, workspaceID, returnURL string) (string, error) {
	url, err := s.provider.CreatePortalSession(ctx, workspaceID, returnURL)
	if err != nil {
		return "", fmt.Errorf("open portal for workspace %q: %w", workspaceID, err)
	}
	return url, nil
}

// HandleEvent applies ev once. A replay of an already-processed event ID is a
// no-op returning nil.
func (s *Service) HandleEvent(ctx context.Context, ev Event) error {
	seen, err := s.store.Seen(ctx, ev.ID)
	if err != nil {
		return fmt.Errorf("check event %q: %w", ev.ID, err)
	}
	if seen {
		s.log.InfoContext(ctx, "billing event replay skipped", "event_id", ev.ID)
		return nil
	}
	if ev.WorkspaceID == "" {
		return fmt.Errorf("event %q: %w", ev.ID, ErrMissingWorkspace)
	}
	sub, err := s.subscriptionFor(ev)
	if err != nil {
		return fmt.Errorf("event %q: %w", ev.ID, err)
	}
	if err := s.sink.SetSubscription(ctx, ev.WorkspaceID, sub); err != nil {
		return fmt.Errorf("set subscription for workspace %q: %w", ev.WorkspaceID, err)
	}
	if err := s.store.Record(ctx, ev.ID); err != nil {
		// The sink is idempotent, so the retry this triggers is harmless.
		return fmt.Errorf("record event %q: %w", ev.ID, err)
	}
	s.log.InfoContext(ctx, "billing event applied",
		"event_id", ev.ID, "type", string(ev.Type), "workspace_id", ev.WorkspaceID,
		"plan", sub.PlanID, "status", string(sub.Status))
	return nil
}

func (s *Service) subscriptionFor(ev Event) (Subscription, error) {
	status := ev.Status
	if ev.Type == EventSubscriptionDeleted {
		status = StatusCanceled
	}
	if !status.Valid() {
		return Subscription{}, fmt.Errorf("%w: unknown status %q", ErrMalformedEvent, status)
	}
	free := Subscription{
		CustomerID: ev.CustomerID,
		PlanID:     s.catalog.FreePlanID(), Status: status,
		PeriodStart: ev.PeriodStart, PeriodEnd: ev.PeriodEnd,
	}
	if status == StatusCanceled || status == StatusIncomplete {
		return free, nil
	}

	sub := Subscription{
		CustomerID: ev.CustomerID,
		Status:     status, TrialEnd: ev.TrialEnd,
		PeriodStart: ev.PeriodStart, PeriodEnd: ev.PeriodEnd,
	}
	for _, priceID := range ev.PriceIDs {
		p, ok := s.catalog.Lookup(priceID)
		if !ok {
			return Subscription{}, fmt.Errorf("%w: %q", ErrUnknownPrice, priceID)
		}
		switch {
		case p.Kind == KindAddOn:
			if !slices.Contains(sub.AddOnIDs, p.ID) {
				sub.AddOnIDs = append(sub.AddOnIDs, p.ID)
			}
		case sub.PlanID == "" || sub.PlanID == p.ID:
			sub.PlanID = p.ID
		default:
			return Subscription{}, fmt.Errorf("%w: %q and %q", ErrAmbiguousPlan, sub.PlanID, p.ID)
		}
	}
	if sub.PlanID == "" {
		return Subscription{}, fmt.Errorf("%w: no plan price on subscription", ErrUnknownPrice)
	}
	return sub, nil
}
