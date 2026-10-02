package pgstore

import (
	"context"
	"fmt"

	"github.com/bernardoforcillo/drops/pg"
	"github.com/bernardoforcillo/featurelayer/entitlement"
)

// UsageStore is featurelayer's entitlement.UsageStore over feature_usage: how
// much of a metered feature a tenant has spent in a period.
type UsageStore struct{ db *pg.DB }

// NewUsageStore builds the store over an open database.
func NewUsageStore(db *pg.DB) *UsageStore { return &UsageStore{db: db} }

var _ entitlement.UsageStore = (*UsageStore)(nil)

// Get returns the counter, or 0 when nothing has been spent yet.
func (u *UsageStore) Get(ctx context.Context, key entitlement.UsageKey) (int64, error) {
	rows, err := u.db.Query(ctx,
		`SELECT used FROM feature_usage WHERE tenant_id = $1 AND feature = $2 AND period = $3`,
		key.Tenant, string(key.Feature), key.Period)
	if err != nil {
		return 0, fmt.Errorf("pgstore: reading usage: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var used int64
	if rows.Next() {
		if err := rows.Scan(&used); err != nil {
			return 0, fmt.Errorf("pgstore: scanning usage: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("pgstore: reading usage: %w", err)
	}
	return used, nil
}

// Increment adds delta only if the result stays within max (max < 0 means
// unlimited) and reports the counter's value and whether it was applied; a
// refusal changes nothing.
//
// The ceiling is enforced by the statement, not by a read then a write: two
// concurrent requests must not both see room for the last unit and both spend it.
func (u *UsageStore) Increment(ctx context.Context, key entitlement.UsageKey, delta, max int64) (int64, bool, error) {
	// A request larger than the whole allowance can never fit, and the INSERT
	// below would not catch it: with no row yet there is no conflict, so the
	// DO UPDATE guard never runs.
	if max >= 0 && delta > max {
		used, err := u.Get(ctx, key)
		return used, false, err
	}

	rows, err := u.db.Query(ctx, `
		INSERT INTO feature_usage (tenant_id, feature, period, used)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (tenant_id, feature, period) DO UPDATE
			SET used = feature_usage.used + $4, updated_at = now()
			WHERE $5 < 0 OR feature_usage.used + $4 <= $5
		RETURNING used`,
		key.Tenant, string(key.Feature), key.Period, delta, max)
	if err != nil {
		return 0, false, fmt.Errorf("pgstore: incrementing usage: %w", err)
	}
	var (
		used    int64
		applied bool
	)
	if rows.Next() {
		if err := rows.Scan(&used); err != nil {
			_ = rows.Close()
			return 0, false, fmt.Errorf("pgstore: scanning usage: %w", err)
		}
		applied = true
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return 0, false, fmt.Errorf("pgstore: incrementing usage: %w", err)
	}
	if applied {
		return used, true, nil
	}
	// The guard refused the update: report where the counter actually stands.
	used, err = u.Get(ctx, key)
	return used, false, err
}
