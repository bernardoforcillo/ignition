// Package pgstore implements featurelayer's SubscriptionStore and UsageStore
// over Postgres, using github.com/bernardoforcillo/drops.
package pgstore

// Migration is one numbered, ordered schema step. The ID is unique across the
// whole application (hence the "features_" prefix) and sorts in apply order;
// hand the list to whatever migration runner the service uses.
type Migration struct {
	ID  string
	SQL string
}

// Migrations returns the schema this package needs, in apply order. Every
// statement is idempotent.
//
// tenant_id is TEXT with no foreign key so this package stays independent of
// the identity schema; add a REFERENCES to your workspaces table in an app
// migration if you want cascade deletes.
func Migrations() []Migration {
	return []Migration{
		{ID: "features_0001_subscriptions", SQL: createSubscriptions},
		{ID: "features_0002_usage", SQL: createUsage},
	}
}

const createSubscriptions = `
CREATE TABLE IF NOT EXISTS feature_subscriptions (
    tenant_id      TEXT        PRIMARY KEY,
    plan           TEXT        NOT NULL,
    add_ons        JSONB       NOT NULL DEFAULT '[]'::jsonb,
    trial          JSONB,
    grants         JSONB       NOT NULL DEFAULT '[]'::jsonb,
    billing_anchor TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
)`

// The composite key is the counter's identity and what makes the upsert in
// UsageStore.Increment atomic. A new period writes a new row, so there is
// nothing to reset.
const createUsage = `
CREATE TABLE IF NOT EXISTS feature_usage (
    tenant_id  TEXT        NOT NULL,
    feature    TEXT        NOT NULL,
    period     TEXT        NOT NULL,
    used       BIGINT      NOT NULL DEFAULT 0 CHECK (used >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, feature, period)
)`
