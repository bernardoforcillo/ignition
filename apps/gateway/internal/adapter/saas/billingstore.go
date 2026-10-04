package saas

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

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

// stateStore keeps the provider's last word on a workspace's subscription (status, period end).
type stateStore struct{ db *pg.DB }

// stateSetter is the write side the subscription sink needs.
type stateSetter interface {
	set(ctx context.Context, workspaceID string, status billing.Status, periodEnd time.Time) error
}

func (s *stateStore) set(ctx context.Context, workspaceID string, status billing.Status, periodEnd time.Time) error {
	var end any // NULL when billing did not report a period
	if !periodEnd.IsZero() {
		end = periodEnd.UTC()
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO billing_subscriptions (workspace_id, status, current_period_end) VALUES ($1, $2, $3)
		ON CONFLICT (workspace_id) DO UPDATE
		SET status = EXCLUDED.status, current_period_end = EXCLUDED.current_period_end, updated_at = now()`,
		workspaceID, string(status), end); err != nil {
		return fmt.Errorf("recording billing state: %w", err)
	}
	return nil
}

// state returns the recorded status and period end; ok is false when billing never reported one.
func (s *stateStore) state(ctx context.Context, workspaceID string) (status string, periodEnd time.Time, ok bool, err error) {
	rows, err := s.db.Query(ctx, `SELECT status, current_period_end FROM billing_subscriptions WHERE workspace_id = $1`, workspaceID)
	if err != nil {
		return "", time.Time{}, false, fmt.Errorf("reading billing state: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return "", time.Time{}, false, rows.Err()
	}
	var end sql.NullTime
	if err := rows.Scan(&status, &end); err != nil {
		return "", time.Time{}, false, fmt.Errorf("scanning billing state: %w", err)
	}
	return status, end.Time, true, nil
}

// hasCustomer reports whether the workspace has a provider customer.
func (s *customerStore) has(ctx context.Context, workspaceID string) (bool, error) {
	_, err := s.customer(ctx, workspaceID)
	if errors.Is(err, core.ErrNoBillingCustomer) {
		return false, nil
	}
	return err == nil, err
}
