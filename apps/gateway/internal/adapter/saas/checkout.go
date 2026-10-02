package saas

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bernardoforcillo/featurelayer/entitlement"

	"github.com/bernardoforcillo/ignition/go-packages/billing"

	"github.com/bernardoforcillo/ignition/apps/gateway/internal/core"
)

// billingState is the read side of the tables the subscription sink fills.
type billingState interface {
	state(ctx context.Context, workspaceID string) (status string, periodEnd time.Time, ok bool, err error)
	has(ctx context.Context, workspaceID string) (bool, error)
}

// Checkout is the billing service restricted to the configured catalog:
// a client may only start a checkout for a price listed in
// BILLING_PRICES, not for any price in the Stripe account. It also
// reports what a workspace currently holds and the purchasable catalog.
type Checkout struct {
	*billing.Service
	catalog  *billing.Catalog
	prices   []core.PriceInfo
	subs     subscriptionStore
	state    billingState
	freePlan entitlement.PlanID
}

// StartCheckout rejects an unknown price with billing.ErrUnknownPrice
// before the provider is called.
func (c *Checkout) StartCheckout(ctx context.Context, req billing.CheckoutRequest) (string, error) {
	if _, ok := c.catalog.Lookup(req.PriceID); !ok {
		return "", fmt.Errorf("%w: %q", billing.ErrUnknownPrice, req.PriceID)
	}
	return c.Service.StartCheckout(ctx, req)
}

// Prices returns the catalog in BILLING_PRICES order.
func (c *Checkout) Prices() []core.PriceInfo { return c.prices }

// Subscription reads the workspace's plan and add-ons from the entitlement store (the free plan
// when it has none yet) and the status, period end and customer billing recorded.
func (c *Checkout) Subscription(ctx context.Context, workspaceID string) (core.SubscriptionInfo, error) {
	info := core.SubscriptionInfo{PlanID: string(c.freePlan)}
	sub, err := c.subs.Subscription(ctx, workspaceID)
	switch {
	case err == nil:
		info.PlanID = string(sub.Plan)
		for _, a := range sub.AddOns {
			info.AddOnIDs = append(info.AddOnIDs, string(a))
		}
	case !errors.Is(err, entitlement.ErrNoSubscription):
		return core.SubscriptionInfo{}, fmt.Errorf("reading subscription: %w", err)
	}

	status, end, _, err := c.state.state(ctx, workspaceID)
	if err != nil {
		return core.SubscriptionInfo{}, err
	}
	info.Status, info.CurrentPeriodEnd = status, end
	if info.HasCustomer, err = c.state.has(ctx, workspaceID); err != nil {
		return core.SubscriptionInfo{}, err
	}
	return info, nil
}
