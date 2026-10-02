package saas

import (
	"context"
	"fmt"

	"github.com/bernardoforcillo/drops/pg"

	"github.com/bernardoforcillo/ignition/go-packages/billing"

	"github.com/bernardoforcillo/ignition/apps/gateway/internal/core"
)

// eventStore is billing.EventStore over billing_events.
type eventStore struct{ db *pg.DB }

var _ billing.EventStore = (*eventStore)(nil)

func (s *eventStore) Seen(ctx context.Context, eventID string) (bool, error) {
	rows, err := s.db.Query(ctx, `SELECT 1 FROM billing_events WHERE event_id = $1`, eventID)
	if err != nil {
		return false, fmt.Errorf("reading billing event: %w", err)
	}
	defer func() { _ = rows.Close() }()
	seen := rows.Next()
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("reading billing event: %w", err)
	}
	return seen, nil
}

func (s *eventStore) Record(ctx context.Context, eventID string) error {
	if _, err := s.db.Exec(ctx,
		`INSERT INTO billing_events (event_id) VALUES ($1) ON CONFLICT DO NOTHING`, eventID); err != nil {
		return fmt.Errorf("recording billing event: %w", err)
	}
	return nil
}

// customerStore keeps the workspace -> payment-provider customer mapping. The subscription sink
// fills it from verified provider events, so the customer portal can find the customer later.
type customerStore struct{ db *pg.DB }

// customerSetter is the write side the subscription sink needs.
type customerSetter interface {
	set(ctx context.Context, workspaceID, customerID string) error
}

func (s *customerStore) set(ctx context.Context, workspaceID, customerID string) error {
	if _, err := s.db.Exec(ctx, `
		INSERT INTO billing_customers (workspace_id, customer_id) VALUES ($1, $2)
		ON CONFLICT (workspace_id) DO UPDATE SET customer_id = EXCLUDED.customer_id, updated_at = now()`,
		workspaceID, customerID); err != nil {
		return fmt.Errorf("recording billing customer: %w", err)
	}
	return nil
}

// customer implements stripe.CustomerResolver. A workspace that never
// completed a checkout yields core.ErrNoBillingCustomer.
func (s *customerStore) customer(ctx context.Context, workspaceID string) (string, error) {
	rows, err := s.db.Query(ctx, `SELECT customer_id FROM billing_customers WHERE workspace_id = $1`, workspaceID)
	if err != nil {
		return "", fmt.Errorf("reading billing customer: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return "", fmt.Errorf("reading billing customer: %w", err)
		}
		return "", core.ErrNoBillingCustomer
	}
	var id string
	if err := rows.Scan(&id); err != nil {
		return "", fmt.Errorf("scanning billing customer: %w", err)
	}
	return id, nil
}
