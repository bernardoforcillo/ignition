package pgstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bernardoforcillo/drops/pg"
	"github.com/bernardoforcillo/featurelayer/entitlement"
)

// SubscriptionStore is featurelayer's entitlement.SubscriptionStore over
// feature_subscriptions. It stores which plan, add-ons, trial and grants apply
// to a tenant; the definitions those ids resolve against live in code.
type SubscriptionStore struct{ db *pg.DB }

// NewSubscriptionStore builds the store over an open database.
func NewSubscriptionStore(db *pg.DB) *SubscriptionStore { return &SubscriptionStore{db: db} }

var _ entitlement.SubscriptionStore = (*SubscriptionStore)(nil)

// Subscription returns entitlement.ErrNoSubscription for a tenant with no row,
// which featurelayer treats as "entitled to nothing" (fail closed).
func (s *SubscriptionStore) Subscription(ctx context.Context, tenantID string) (*entitlement.Subscription, error) {
	rows, err := s.db.Query(ctx, `
		SELECT plan, add_ons, trial, grants, billing_anchor
		FROM feature_subscriptions WHERE tenant_id = $1`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("pgstore: reading subscription: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("pgstore: reading subscription: %w", err)
		}
		return nil, entitlement.ErrNoSubscription
	}

	var (
		plan                  string
		addOns, trial, grants []byte
		anchor                time.Time
	)
	if err := rows.Scan(&plan, &addOns, &trial, &grants, &anchor); err != nil {
		return nil, fmt.Errorf("pgstore: scanning subscription: %w", err)
	}
	sub := entitlement.Subscription{
		TenantID:      tenantID,
		Plan:          entitlement.PlanID(plan),
		BillingAnchor: anchor.UTC(),
	}
	if err := unmarshalJSON(addOns, &sub.AddOns); err != nil {
		return nil, fmt.Errorf("pgstore: decoding add_ons: %w", err)
	}
	if err := unmarshalJSON(grants, &sub.Grants); err != nil {
		return nil, fmt.Errorf("pgstore: decoding grants: %w", err)
	}
	if len(trial) > 0 && string(trial) != "null" {
		var t entitlement.PlanTrial
		if err := json.Unmarshal(trial, &t); err != nil {
			return nil, fmt.Errorf("pgstore: decoding trial: %w", err)
		}
		sub.Trial = &t
	}
	return &sub, nil
}

func unmarshalJSON(b []byte, dst any) error {
	if len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, dst)
}

// Set writes a tenant's subscription, the call the billing module makes when a
// checkout completes, a plan changes or a trial starts. It is idempotent and
// keeps the stored billing anchor on update: moving the anchor would move the
// current period's boundary and hand the tenant a fresh budget mid-period. A
// zero anchor on first insert means "now".
func (s *SubscriptionStore) Set(ctx context.Context, sub entitlement.Subscription) error {
	addOns, err := json.Marshal(orEmpty(sub.AddOns))
	if err != nil {
		return fmt.Errorf("pgstore: encoding add_ons: %w", err)
	}
	grants, err := json.Marshal(orEmpty(sub.Grants))
	if err != nil {
		return fmt.Errorf("pgstore: encoding grants: %w", err)
	}
	var trial any // nil binds as SQL NULL
	if sub.Trial != nil {
		b, err := json.Marshal(sub.Trial)
		if err != nil {
			return fmt.Errorf("pgstore: encoding trial: %w", err)
		}
		trial = string(b)
	}
	anchor := sub.BillingAnchor
	if anchor.IsZero() {
		anchor = time.Now().UTC()
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO feature_subscriptions (tenant_id, plan, add_ons, trial, grants, billing_anchor)
		VALUES ($1, $2, $3::jsonb, $4::jsonb, $5::jsonb, $6)
		ON CONFLICT (tenant_id) DO UPDATE SET
			plan = EXCLUDED.plan,
			add_ons = EXCLUDED.add_ons,
			trial = EXCLUDED.trial,
			grants = EXCLUDED.grants,
			updated_at = now()`,
		sub.TenantID, string(sub.Plan), string(addOns), trial, string(grants), anchor); err != nil {
		return fmt.Errorf("pgstore: writing subscription: %w", err)
	}
	return nil
}

// Delete removes a tenant's subscription; it is then entitled to nothing.
func (s *SubscriptionStore) Delete(ctx context.Context, tenantID string) error {
	if _, err := s.db.Exec(ctx, `DELETE FROM feature_subscriptions WHERE tenant_id = $1`, tenantID); err != nil {
		return fmt.Errorf("pgstore: deleting subscription: %w", err)
	}
	return nil
}

// orEmpty renders a nil slice as [] so the column never holds JSON null.
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
