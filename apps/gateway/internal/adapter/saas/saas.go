// Package saas is the capabilities/composition adapter for the SaaS
// surface: it turns configuration and a database handle into the
// identity, features, billing and mail services, runs their migrations,
// and implements the ports those libraries leave to the application (the
// billing subscription sink and event store, the rate limiter).
//
// It imports the libraries and internal/core, never another adapter; the
// transport (internal/adapter/httpapi) declares the interfaces it needs
// and main.go hands it the values from Services.
package saas

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/bernardoforcillo/authlayer/org"
	dropsstore "github.com/bernardoforcillo/authlayer/store/drops"
	"github.com/bernardoforcillo/drops/pg"
	"github.com/bernardoforcillo/featurelayer/entitlement"

	"github.com/bernardoforcillo/ignition/go-packages/billing"
	"github.com/bernardoforcillo/ignition/go-packages/billing/httpwebhook"
	"github.com/bernardoforcillo/ignition/go-packages/billing/stripe"
	"github.com/bernardoforcillo/ignition/go-packages/database"
	"github.com/bernardoforcillo/ignition/go-packages/features"
	"github.com/bernardoforcillo/ignition/go-packages/features/pgstore"
	"github.com/bernardoforcillo/ignition/go-packages/identity/account"
	"github.com/bernardoforcillo/ignition/go-packages/identity/auth"
	"github.com/bernardoforcillo/ignition/go-packages/identity/permissions"
	"github.com/bernardoforcillo/ignition/go-packages/identity/workspace"
	"github.com/bernardoforcillo/ignition/go-packages/mailer"
	"github.com/bernardoforcillo/ignition/go-packages/mailer/resend"

	"github.com/bernardoforcillo/ignition/apps/gateway/internal/config"
	"github.com/bernardoforcillo/ignition/apps/gateway/internal/core"
)

// Services is everything the transport layer needs, as concrete library
// values; main.go assigns them to httpapi's consumer-side interfaces.
type Services struct {
	// directory holds the SQL the identity engines lack; Emails exposes the part transport needs.
	directory *directory

	Auth       *auth.Service
	Workspaces *Workspaces
	Account    *account.Service
	Features   *features.Engine

	// Billing and BillingWebhook are nil unless billing is configured.
	Billing        *Checkout
	BillingWebhook http.Handler

	// Mail is the product mailer; background jobs send their emails through it.
	Mail *mailer.Mailer

	db *database.DB
}

// DB is the shared Postgres handle, for adapters (background jobs) that keep their own tables.
func (s *Services) DB() *pg.DB { return s.db.DB }

// Ready reports whether the database answers; /readyz uses it.
func (s *Services) Ready(ctx context.Context) error { return s.db.Ping(ctx) }

// Close releases the database. Build takes ownership of the handle it
// was given only on success; on a Build error the caller still owns it.
func (s *Services) Close() error { return s.db.Close() }

// EventSink receives the few server-authoritative business events worth sending to product
// analytics (the browser reports everything it can see itself). Implemented by the telemetry
// module; nil disables them.
type EventSink interface {
	Capture(ctx context.Context, id, event string, props map[string]any)
}

// Option customizes Build.
type Option func(*options)

type options struct{ events EventSink }

// WithEvents sends server-authoritative business events to sink.
func WithEvents(sink EventSink) Option { return func(o *options) { o.events = sink } }

// Build applies every migration and wires the services over db.
func Build(ctx context.Context, cfg config.SaaS, db *database.DB, logger *slog.Logger, opts ...Option) (*Services, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	if err := database.Migrate(ctx, db, Migrations()...); err != nil {
		return nil, err
	}

	mail, err := newMailer(cfg, logger)
	if err != nil {
		return nil, err
	}

	authSvc, err := auth.NewService(
		dropsstore.NewAuthStore(db.DB), mail, newMemoryLimiter(time.Now),
		auth.Config{Secret: cfg.AuthSecret, AccessTTL: cfg.AccessTTL, RefreshTTL: cfg.RefreshTTL, BaseURL: cfg.AppURL},
	)
	if err != nil {
		return nil, fmt.Errorf("saas: auth: %w", err)
	}

	subs := pgstore.NewSubscriptionStore(db.DB)
	engine, err := features.New(subs, pgstore.NewUsageStore(db.DB), features.WithLogger(logger))
	if err != nil {
		return nil, fmt.Errorf("saas: features: %w", err)
	}

	freePlan := features.PlanFree
	if cfg.Billing != nil && cfg.Billing.FreePlan != "" {
		freePlan = entitlement.PlanID(cfg.Billing.FreePlan)
	}
	wsSvc := workspace.NewService(
		permissions.NewAccess(),
		dropsstore.New[org.Organization, org.Member](db.DB),
		dropsstore.NewInviteStore(db.DB),
		mail, cfg.AppURL,
	)

	dir := &directory{db: db.DB}
	s := &Services{
		Auth:       authSvc,
		Workspaces: &Workspaces{Service: wsSvc, subs: subs, freePlan: freePlan},
		Account:    account.NewService(authSvc, wsSvc, dir),
		Features:   engine,
		Mail:       mail,
		db:         db,
		directory:  dir,
	}
	if cfg.Billing != nil {
		if err := s.wireBilling(db, *cfg.Billing, subs, freePlan, logger, o.events); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func newMailer(cfg config.SaaS, logger *slog.Logger) (*mailer.Mailer, error) {
	var sender mailer.Sender = mailer.NewLogSender(logger)
	if cfg.ResendAPIKey != "" {
		client, err := resend.New(resend.Config{APIKey: cfg.ResendAPIKey, BaseURL: cfg.ResendBaseURL})
		if err != nil {
			return nil, fmt.Errorf("saas: resend: %w", err)
		}
		sender = client
	} else {
		logger.Warn("RESEND_API_KEY not set: emails are logged, not sent")
	}
	m, err := mailer.New(sender, mailer.Config{
		From:         cfg.MailFrom,
		ReplyTo:      cfg.MailReplyTo,
		CompanyName:  cfg.CompanyName,
		AppURL:       cfg.AppURL,
		AssetBaseURL: cfg.AssetBaseURL,
	}, logger)
	if err != nil {
		return nil, fmt.Errorf("saas: mailer: %w", err)
	}
	return m, nil
}

func (s *Services) wireBilling(db *database.DB, cfg config.Billing, subs subscriptionStore, freePlan entitlement.PlanID, logger *slog.Logger, events EventSink) error {
	catalog, err := newCatalog(cfg, freePlan)
	if err != nil {
		return err
	}
	customers := &customerStore{db: db.DB}
	states := &stateStore{db: db.DB}
	provider, err := stripe.New(stripe.Config{
		APIKey:        cfg.StripeAPIKey,
		WebhookSecret: cfg.StripeWebhookSecret,
		Customers:     customers.customer,
		BaseURL:       cfg.StripeAPIBaseURL,
	})
	if err != nil {
		return fmt.Errorf("saas: stripe: %w", err)
	}
	svc := billing.NewService(catalog, provider, &subscriptionSink{store: subs, customers: customers, states: states, events: events}, &eventStore{db: db.DB}, logger)
	prices := make([]core.PriceInfo, len(cfg.Prices))
	for i, p := range cfg.Prices {
		prices[i] = core.PriceInfo{PriceID: p.ProviderPriceID, Kind: p.Kind, ID: p.ID}
	}
	s.Billing = &Checkout{
		Service: svc, catalog: catalog, prices: prices, subs: subs, freePlan: freePlan,
		state: billingReader{states, customers},
	}
	s.BillingWebhook = httpwebhook.New(provider, svc, 0, logger)
	return nil
}

// newCatalog validates BILLING_PRICES against the feature catalog, so a
// typo'd plan id is a startup error rather than a workspace that pays and
// is entitled to nothing.
func newCatalog(cfg config.Billing, freePlan entitlement.PlanID) (*billing.Catalog, error) {
	known := features.Config()
	plans, addOns := map[string]bool{}, map[string]bool{}
	for _, p := range known.Plans {
		plans[string(p.ID)] = true
	}
	for _, a := range known.AddOns {
		addOns[string(a.ID)] = true
	}
	if !plans[string(freePlan)] {
		return nil, fmt.Errorf("saas: free plan %q is not a plan in the feature catalog", freePlan)
	}

	prices := make([]billing.Price, 0, len(cfg.Prices))
	for _, p := range cfg.Prices {
		kind := billing.PriceKind(p.Kind)
		if (kind == billing.KindPlan && !plans[p.ID]) || (kind == billing.KindAddOn && !addOns[p.ID]) {
			return nil, fmt.Errorf("saas: BILLING_PRICES %q targets %s %q, which is not in the feature catalog", p.ProviderPriceID, p.Kind, p.ID)
		}
		prices = append(prices, billing.Price{ProviderPriceID: p.ProviderPriceID, Kind: kind, ID: p.ID})
	}
	catalog, err := billing.NewCatalog(billing.CatalogConfig{FreePlanID: string(freePlan), Prices: prices})
	if err != nil {
		return nil, fmt.Errorf("saas: billing catalog: %w", err)
	}
	return catalog, nil
}

// billingReader is the read side of the billing tables: the provider's status and period end,
// and whether the workspace has a customer to open the portal for.
type billingReader struct {
	*stateStore
	*customerStore
}

// OwnerIDs lists the account ids that own a workspace.
func (s *Services) OwnerIDs(ctx context.Context, workspaceID string) ([]string, error) {
	return s.directory.OwnerIDs(ctx, workspaceID)
}

// MemberEmails resolves account ids to email addresses for member lists.
func (s *Services) MemberEmails(ctx context.Context, ids []string) (map[string]string, error) {
	return s.directory.Emails(ctx, ids)
}
