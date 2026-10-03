package stripe

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/bernardoforcillo/ignition/go-packages/billing"
)

// WorkspaceMetadataKey is the Stripe metadata key holding the workspace ID.
// Checkout sets it on the subscription so every later event carries it.
const WorkspaceMetadataKey = "workspace_id"

type envelope struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Created int64  `json:"created"`
	Data    struct {
		Object json.RawMessage `json:"object"`
	} `json:"data"`
}

type priceRef struct {
	ID string `json:"id"`
}

// lineItem covers the subscription item and invoice line shapes: the price
// sits at price.id, or pricing.price_details.price on newer API versions.
type lineItem struct {
	Price   *priceRef `json:"price"`
	Pricing *struct {
		PriceDetails struct {
			Price string `json:"price"`
		} `json:"price_details"`
	} `json:"pricing"`
	Period struct {
		Start int64 `json:"start"`
		End   int64 `json:"end"`
	} `json:"period"`
	CurrentPeriodStart int64 `json:"current_period_start"` // subscription item, newer API
	CurrentPeriodEnd   int64 `json:"current_period_end"`
}

func (l lineItem) priceID() string {
	if l.Price != nil && l.Price.ID != "" {
		return l.Price.ID
	}
	if l.Pricing != nil {
		return l.Pricing.PriceDetails.Price
	}
	return ""
}

type itemList struct {
	Data []lineItem `json:"data"`
}

type subscriptionObject struct {
	ID                 string            `json:"id"`
	Customer           string            `json:"customer"`
	Status             string            `json:"status"`
	Metadata           map[string]string `json:"metadata"`
	TrialEnd           int64             `json:"trial_end"`
	CurrentPeriodStart int64             `json:"current_period_start"`
	CurrentPeriodEnd   int64             `json:"current_period_end"`
	Items              itemList          `json:"items"`
}

type invoiceObject struct {
	Customer            string      `json:"customer"`
	Subscription        string      `json:"subscription"` // older API
	SubscriptionDetails *subDetails `json:"subscription_details"`
	Parent              *struct {
		SubscriptionDetails *subDetails `json:"subscription_details"`
	} `json:"parent"` // newer API
	Metadata map[string]string `json:"metadata"`
	Lines    itemList          `json:"lines"`
}

type subDetails struct {
	Subscription string            `json:"subscription"`
	Metadata     map[string]string `json:"metadata"`
}

func parseEvent(payload []byte) (billing.Event, error) {
	var env envelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return billing.Event{}, fmt.Errorf("%w: %v", billing.ErrMalformedEvent, err)
	}
	ev := billing.Event{ID: env.ID, OccurredAt: unix(env.Created)}
	var err error
	switch env.Type {
	case "customer.subscription.created":
		ev.Type = billing.EventSubscriptionCreated
		err = fillSubscription(&ev, env.Data.Object)
	case "customer.subscription.updated":
		ev.Type = billing.EventSubscriptionUpdated
		err = fillSubscription(&ev, env.Data.Object)
	case "customer.subscription.deleted":
		ev.Type = billing.EventSubscriptionDeleted
		err = fillSubscription(&ev, env.Data.Object)
	case "invoice.paid":
		ev.Type, ev.Status = billing.EventInvoicePaid, billing.StatusActive
		err = fillInvoice(&ev, env.Data.Object)
	case "invoice.payment_failed":
		ev.Type, ev.Status = billing.EventInvoiceFailed, billing.StatusPastDue
		err = fillInvoice(&ev, env.Data.Object)
	default:
		return billing.Event{}, fmt.Errorf("%w: %q", billing.ErrIgnoredEvent, env.Type)
	}
	if err != nil {
		return billing.Event{}, fmt.Errorf("%w: %s: %v", billing.ErrMalformedEvent, env.Type, err)
	}
	if ev.ID == "" {
		return billing.Event{}, fmt.Errorf("%w: missing event id", billing.ErrMalformedEvent)
	}
	return ev, nil
}

func fillSubscription(ev *billing.Event, raw json.RawMessage) error {
	var s subscriptionObject
	if err := json.Unmarshal(raw, &s); err != nil {
		return err
	}
	status, err := mapStatus(s.Status)
	if err != nil {
		return err
	}
	ev.Status, ev.CustomerID, ev.SubscriptionID = status, s.Customer, s.ID
	ev.WorkspaceID = s.Metadata[WorkspaceMetadataKey]
	ev.TrialEnd = unix(s.TrialEnd)
	ev.PeriodStart, ev.PeriodEnd = unix(s.CurrentPeriodStart), unix(s.CurrentPeriodEnd)
	for _, it := range s.Items.Data {
		if id := it.priceID(); id != "" {
			ev.PriceIDs = append(ev.PriceIDs, id)
		}
		if ev.PeriodEnd.IsZero() { // newer API keeps periods on items
			ev.PeriodStart, ev.PeriodEnd = unix(it.CurrentPeriodStart), unix(it.CurrentPeriodEnd)
		}
	}
	return nil
}

func fillInvoice(ev *billing.Event, raw json.RawMessage) error {
	var inv invoiceObject
	if err := json.Unmarshal(raw, &inv); err != nil {
		return err
	}
	ev.CustomerID, ev.SubscriptionID = inv.Customer, inv.Subscription
	meta := inv.Metadata
	for _, d := range []*subDetails{inv.SubscriptionDetails, parentDetails(inv)} {
		if d == nil {
			continue
		}
		if ev.SubscriptionID == "" {
			ev.SubscriptionID = d.Subscription
		}
		if d.Metadata[WorkspaceMetadataKey] != "" {
			meta = d.Metadata
		}
	}
	ev.WorkspaceID = meta[WorkspaceMetadataKey]
	for _, l := range inv.Lines.Data {
		if id := l.priceID(); id != "" {
			ev.PriceIDs = append(ev.PriceIDs, id)
		}
		if ev.PeriodEnd.IsZero() {
			ev.PeriodStart, ev.PeriodEnd = unix(l.Period.Start), unix(l.Period.End)
		}
	}
	return nil
}

func parentDetails(inv invoiceObject) *subDetails {
	if inv.Parent == nil {
		return nil
	}
	return inv.Parent.SubscriptionDetails
}

func mapStatus(s string) (billing.Status, error) {
	switch s {
	case "trialing":
		return billing.StatusTrialing, nil
	case "active":
		return billing.StatusActive, nil
	case "past_due", "unpaid":
		return billing.StatusPastDue, nil
	case "canceled", "incomplete_expired", "paused":
		return billing.StatusCanceled, nil
	case "incomplete":
		return billing.StatusIncomplete, nil
	}
	return "", fmt.Errorf("unknown stripe status %q", s)
}

func unix(sec int64) time.Time {
	if sec == 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0).UTC()
}
