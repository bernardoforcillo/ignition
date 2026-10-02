package saas

import (
	"context"
	"errors"
	"fmt"

	"github.com/bernardoforcillo/featurelayer/entitlement"

	"github.com/bernardoforcillo/ignition/go-packages/billing"
	"github.com/bernardoforcillo/ignition/go-packages/identity/workspace"
)

// subscriptionStore is the slice of the features/pgstore subscription store
// this package uses.
type subscriptionStore interface {
	Subscription(ctx context.Context, tenantID string) (*entitlement.Subscription, error)
	Set(ctx context.Context, sub entitlement.Subscription) error
}

// subscriptionSink is billing.SubscriptionSink over the features
// subscription store: it translates billing's view of a workspace's
// subscription into featurelayer's and writes it. Per-workspace grants
// (manual overrides) are preserved; the billing anchor is kept by the
// store on update. It also remembers the provider's customer for the workspace (billing puts it on
// every subscription it pushes, lapsed ones included), which is what lets OpenPortal work, and the
// provider's status and period end, which BillingService.GetSubscription reports.
type subscriptionSink struct {
	store     subscriptionStore
	customers customerSetter
	states    stateSetter
}

var _ billing.SubscriptionSink = (*subscriptionSink)(nil)

func (s *subscriptionSink) SetSubscription(ctx context.Context, workspaceID string, sub billing.Subscription) error {
	// The customer first: both writes are idempotent, and a failure of either makes the webhook
	// fail so the provider retries it (billing records the event only after the sink succeeds).
	if sub.CustomerID != "" {
		if err := s.customers.set(ctx, workspaceID, sub.CustomerID); err != nil {
			return err
		}
	}
	if err := s.states.set(ctx, workspaceID, sub.Status, sub.PeriodEnd); err != nil {
		return err
	}
	out := toEntitlement(workspaceID, sub)
	existing, err := s.store.Subscription(ctx, workspaceID)
	switch {
	case err == nil:
		out.Grants = existing.Grants
	case !errors.Is(err, entitlement.ErrNoSubscription):
		return fmt.Errorf("reading current subscription: %w", err)
	}
	return s.store.Set(ctx, out)
}

// toEntitlement maps a billing subscription onto featurelayer's. billing
// has already applied the status policy (which plan a past-due or canceled
// workspace holds), so only the shape changes here; a provider trial is
// already a paid plan in billing's output, hence no featurelayer Trial.
func toEntitlement(workspaceID string, sub billing.Subscription) entitlement.Subscription {
	out := entitlement.Subscription{
		TenantID:      workspaceID,
		Plan:          entitlement.PlanID(sub.PlanID),
		BillingAnchor: sub.PeriodStart,
	}
	for _, id := range sub.AddOnIDs {
		out.AddOns = append(out.AddOns, entitlement.AddOnID(id))
	}
	return out
}

// Workspaces is the workspace service plus the domain rule that a new
// workspace starts on the free plan: without a subscription row it would
// be entitled to nothing.
type Workspaces struct {
	*workspace.Service
	subs     subscriptionStore
	freePlan entitlement.PlanID
}

// Create makes the workspace and gives it the free plan. If the
// subscription cannot be written the error is returned (the workspace
// exists but is entitled to nothing until billing or an operator sets one).
func (w *Workspaces) Create(ctx context.Context, userID, name, slug string) (workspace.Workspace, error) {
	ws, err := w.Service.Create(ctx, userID, name, slug)
	if err != nil {
		return workspace.Workspace{}, err
	}
	if err := w.subs.Set(ctx, entitlement.Subscription{TenantID: ws.ID, Plan: w.freePlan}); err != nil {
		return workspace.Workspace{}, fmt.Errorf("providing free plan for workspace %q: %w", ws.ID, err)
	}
	return ws, nil
}
