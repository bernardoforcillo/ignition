package saas

import (
	"context"
	"fmt"

	"github.com/bernardoforcillo/ignition/go-packages/billing"
)

// Checkout is the billing service restricted to the configured catalog:
// a client may only start a checkout for a price listed in
// BILLING_PRICES, not for any price in the Stripe account.
type Checkout struct {
	*billing.Service
	catalog *billing.Catalog
}

// StartCheckout rejects an unknown price with billing.ErrUnknownPrice
// before the provider is called.
func (c *Checkout) StartCheckout(ctx context.Context, req billing.CheckoutRequest) (string, error) {
	if _, ok := c.catalog.Lookup(req.PriceID); !ok {
		return "", fmt.Errorf("%w: %q", billing.ErrUnknownPrice, req.PriceID)
	}
	return c.Service.StartCheckout(ctx, req)
}
