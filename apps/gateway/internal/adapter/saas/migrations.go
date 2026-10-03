package saas

import (
	"context"

	"github.com/bernardoforcillo/authlayer/org"
	dropsstore "github.com/bernardoforcillo/authlayer/store/drops"
	"github.com/bernardoforcillo/drops/pg"

	"github.com/bernardoforcillo/ignition/go-packages/database"
	"github.com/bernardoforcillo/ignition/go-packages/features/pgstore"
	jobsstore "github.com/bernardoforcillo/ignition/go-packages/jobs/pgstore"
)

// Migration IDs sort lexicographically into apply order and must stay
// unique across every module sharing schema_migrations, hence the
// per-module prefixes. Never edit an applied migration: add a new one.
const (
	identitySchemaID = "identity_0001_schema"
	billingEventsID  = "billing_0001_events"
	billingCustomers = "billing_0002_customers"
	billingStates    = "billing_0003_subscription_states"
	jobNotifications = "gateway_0001_job_notifications"
)

const createBillingEvents = `
CREATE TABLE IF NOT EXISTS billing_events (
    event_id     TEXT        PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`

// billing_customers remembers which payment-provider customer a workspace
// is, learned from verified webhook events, so the portal can be opened.
const createBillingCustomers = `
CREATE TABLE IF NOT EXISTS billing_customers (
    workspace_id TEXT        PRIMARY KEY,
    customer_id  TEXT        NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
)`

// billing_subscriptions keeps what the provider last said about a workspace's subscription
// (lifecycle status and period end), which the entitlement store does not model. It is fed by
// the same verified events as the entitlement and read by BillingService.GetSubscription.
const createBillingStates = `
CREATE TABLE IF NOT EXISTS billing_subscriptions (
    workspace_id       TEXT        PRIMARY KEY,
    status             TEXT        NOT NULL,
    current_period_end TIMESTAMPTZ,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
)`

// job_notifications records that a one-off message (kind) went out for a subject (key), so a retried
// job or a second replica never sends it twice: the primary key is what makes the claim atomic.
const createJobNotifications = `
CREATE TABLE IF NOT EXISTS job_notifications (
    kind    TEXT        NOT NULL,
    key     TEXT        NOT NULL,
    sent_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (kind, key)
)`

// Migrations returns every schema step the SaaS surface needs, across the
// identity, features, billing and jobs modules. Applying them is idempotent.
func Migrations() []database.Migration {
	migs := []database.Migration{
		{ID: identitySchemaID, Up: createIdentitySchema},
		database.SQL(billingEventsID, createBillingEvents),
		database.SQL(billingCustomers, createBillingCustomers),
		database.SQL(billingStates, createBillingStates),
		database.SQL(jobNotifications, createJobNotifications),
	}
	for _, m := range pgstore.Migrations() {
		migs = append(migs, database.SQL(m.ID, m.SQL))
	}
	for _, m := range jobsstore.Migrations() {
		migs = append(migs, database.SQL(m.ID, m.SQL))
	}
	return migs
}

// createIdentitySchema creates authlayer's tables (users, sessions,
// verifications, organizations, members, invitations) inside the
// migration's transaction. CreateSchema is idempotent.
func createIdentitySchema(ctx context.Context, tx *pg.DB) error {
	if err := dropsstore.NewAuthStore(tx).CreateSchema(ctx); err != nil {
		return err
	}
	if err := dropsstore.New[org.Organization, org.Member](tx).CreateSchema(ctx); err != nil {
		return err
	}
	return dropsstore.NewInviteStore(tx).CreateSchema(ctx)
}
