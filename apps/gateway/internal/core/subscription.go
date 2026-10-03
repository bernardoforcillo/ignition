package core

import "time"

// SubscriptionInfo is what a workspace is entitled to and what billing last reported about it.
type SubscriptionInfo struct {
	PlanID   string
	AddOnIDs []string
	// Status is billing's lifecycle state ("active", "past_due", ...), empty when the provider
	// never reported one (a workspace on the free plan that never paid).
	Status string
	// CurrentPeriodEnd is zero when unknown.
	CurrentPeriodEnd time.Time
	// HasCustomer reports that the workspace has a payment-provider customer (the portal exists).
	HasCustomer bool
}

// PriceInfo is one purchasable catalog entry.
type PriceInfo struct {
	PriceID string
	// Kind is "plan" or "addon"; ID is the plan or add-on it buys.
	Kind string
	ID   string
}
