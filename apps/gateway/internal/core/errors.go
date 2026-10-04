package core

import "errors"

// ErrNoBillingCustomer reports that a workspace has never completed a
// checkout, so there is no payment-provider customer to open a portal for.
// Capability adapters return it; the inbound adapter maps it once.
var ErrNoBillingCustomer = errors.New("workspace has no billing customer")
