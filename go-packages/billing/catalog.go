package billing

import "fmt"

// PriceKind says whether a provider price buys a plan or an add-on.
type PriceKind string

const (
	KindPlan  PriceKind = "plan"
	KindAddOn PriceKind = "addon"
)

// Price maps one provider price ID to a plan or add-on ID.
type Price struct {
	ProviderPriceID string
	Kind            PriceKind
	ID              string // plan or add-on ID in the entitlement system
}

// CatalogConfig is the config-driven source of a Catalog.
type CatalogConfig struct {
	FreePlanID string // plan a workspace falls back to when it stops paying
	Prices     []Price
}

// Catalog resolves provider price IDs. It is immutable after NewCatalog.
type Catalog struct {
	free   string
	prices map[string]Price
}

// NewCatalog validates cfg: a free plan, and prices with a non-empty,
// unique provider ID, a known kind and a target ID.
func NewCatalog(cfg CatalogConfig) (*Catalog, error) {
	if cfg.FreePlanID == "" {
		return nil, fmt.Errorf("%w: free plan ID is required", ErrInvalidConfig)
	}
	prices := make(map[string]Price, len(cfg.Prices))
	for i, p := range cfg.Prices {
		switch {
		case p.ProviderPriceID == "":
			return nil, fmt.Errorf("%w: price %d has no provider price ID", ErrInvalidConfig, i)
		case p.ID == "":
			return nil, fmt.Errorf("%w: price %q has no plan/add-on ID", ErrInvalidConfig, p.ProviderPriceID)
		case p.Kind != KindPlan && p.Kind != KindAddOn:
			return nil, fmt.Errorf("%w: price %q has unknown kind %q", ErrInvalidConfig, p.ProviderPriceID, p.Kind)
		}
		if _, dup := prices[p.ProviderPriceID]; dup {
			return nil, fmt.Errorf("%w: duplicate price %q", ErrInvalidConfig, p.ProviderPriceID)
		}
		prices[p.ProviderPriceID] = p
	}
	return &Catalog{free: cfg.FreePlanID, prices: prices}, nil
}

// FreePlanID returns the fallback plan.
func (c *Catalog) FreePlanID() string { return c.free }

// Lookup returns the catalog entry for a provider price ID.
func (c *Catalog) Lookup(providerPriceID string) (Price, bool) {
	p, ok := c.prices[providerPriceID]
	return p, ok
}
