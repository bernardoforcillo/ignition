// Package features is the template's feature-management layer: what the product
// can do, which workspace is entitled to it, and how much of it they may use.
//
// The engine is github.com/bernardoforcillo/featurelayer. This package owns the
// starter definitions (catalog.go), the glue that turns "workspace" into
// featurelayer's "tenant", and the typed errors a transport layer maps to
// response codes. Per-tenant state lives behind featurelayer's two store
// interfaces; the Postgres implementations are in package pgstore.
package features

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	featurelayer "github.com/bernardoforcillo/featurelayer"
	"github.com/bernardoforcillo/featurelayer/catalog"
	"github.com/bernardoforcillo/featurelayer/entitlement"
)

// Sentinel errors returned (wrapped) by Require and RequireConsume. A transport
// layer maps them with errors.Is: ErrNotEntitled to permission denied / 403,
// ErrLimitReached to resource exhausted / 429, ErrDisabled to unavailable /
// 503. Any other error is an infrastructure failure, not a business "no".
var (
	// ErrNotEntitled: the workspace's plan does not carry the feature, or the
	// feature is unknown, or the workspace has no subscription.
	ErrNotEntitled = errors.New("features: not entitled")
	// ErrDisabled: the feature is switched off (flag, lifecycle or a
	// prerequisite), whatever the plan says.
	ErrDisabled = errors.New("features: disabled")
	// ErrLimitReached: a metered limit is used up for the current period.
	ErrLimitReached = errors.New("features: limit reached")
)

// Engine answers "may this workspace use this feature right now, and how much
// is left". It is safe for concurrent use and caches nothing beyond the
// immutable snapshot, so build one at startup and share it.
type Engine struct {
	e *featurelayer.Engine
}

type options struct {
	cfg    featurelayer.Config
	clock  func() time.Time
	logger *slog.Logger
}

// Option configures New.
type Option func(*options)

// WithConfig replaces the default catalog (Config) with the product's own
// definitions, or with a variant in a test.
func WithConfig(cfg featurelayer.Config) Option { return func(o *options) { o.cfg = cfg } }

// WithClock overrides the time source; tests use it to cross a period boundary.
func WithClock(now func() time.Time) Option { return func(o *options) { o.clock = now } }

// WithLogger sets where denials are logged (default slog.Default()).
func WithLogger(l *slog.Logger) Option { return func(o *options) { o.logger = l } }

// New builds the engine over the two per-tenant stores. Both are required:
// without a SubscriptionStore featurelayer runs in flags-only mode and every
// gated feature skips its commercial check. It fails if the definitions are
// invalid, so a bad catalog is a startup error.
func New(subs entitlement.SubscriptionStore, usage entitlement.UsageStore, opts ...Option) (*Engine, error) {
	if subs == nil || usage == nil {
		return nil, errors.New("features: subscription and usage stores are required")
	}
	o := options{cfg: Config(), logger: slog.Default()}
	for _, opt := range opts {
		opt(&o)
	}
	snap, err := featurelayer.NewSnapshot(o.cfg)
	if err != nil {
		return nil, fmt.Errorf("features: invalid definitions: %w", err)
	}
	flOpts := []featurelayer.Option{
		featurelayer.WithSubscriptions(subs),
		featurelayer.WithUsage(usage),
		featurelayer.WithDecisionHook(denialLogger(o.logger)),
	}
	if o.clock != nil {
		flOpts = append(flOpts, featurelayer.WithClock(o.clock))
	}
	return &Engine{e: featurelayer.New(snap, flOpts...)}, nil
}

// denialLogger records the decisions worth looking at: a refusal, or a store
// failure behind one. Allowed decisions are the overwhelming majority and would
// only bury the others.
func denialLogger(l *slog.Logger) func(featurelayer.DecisionEvent) {
	return func(ev featurelayer.DecisionEvent) {
		if ev.Decision.Enabled && ev.Err == nil {
			return
		}
		l.Debug("feature denied",
			"op", ev.Op,
			"feature", string(ev.Decision.Feature),
			"workspace_id", ev.Context.TenantID,
			"reason", string(ev.Decision.Reason),
			"detail", ev.Decision.Detail,
			"error", ev.Err,
		)
	}
}

// evalContext maps this product's identifiers onto featurelayer's: a workspace
// is the tenant, so entitlements, limits and usage are per workspace.
func evalContext(workspaceID, userID string) featurelayer.EvalContext {
	return featurelayer.EvalContext{TenantID: workspaceID, UserID: userID}
}

// Evaluate reports whether a workspace may use a feature, with the reasoning
// attached so a caller can tell "not on your plan" from "switched off". It
// consumes nothing and does not look at the meter.
func (e *Engine) Evaluate(ctx context.Context, key catalog.Key, workspaceID, userID string) featurelayer.Decision {
	return e.e.Evaluate(ctx, key, evalContext(workspaceID, userID))
}

// Allowed is Evaluate reduced to its yes/no, for UI flags and branching.
func (e *Engine) Allowed(ctx context.Context, key catalog.Key, workspaceID, userID string) bool {
	return e.Evaluate(ctx, key, workspaceID, userID).Enabled
}

// Usage reads a metered feature's counter without spending any of it. It takes
// the user id because a metered feature resolves its dependency chain on the way
// to the meter, and flags are evaluated against the caller: a rollout or segment
// rule must answer the same here as in Consume.
func (e *Engine) Usage(ctx context.Context, key catalog.Key, workspaceID, userID string) (featurelayer.Decision, error) {
	return e.e.Usage(ctx, key, evalContext(workspaceID, userID))
}

// Consume spends n units of a metered feature. A refusal (limit reached, not
// entitled, flag off) is a decision with Enabled false and a nil error; only a
// store or configuration failure is an error. Nothing is spent on a refusal.
func (e *Engine) Consume(ctx context.Context, key catalog.Key, workspaceID, userID string, n int64) (featurelayer.Decision, error) {
	return e.e.Consume(ctx, key, evalContext(workspaceID, userID), n)
}

// Require is the gate for an RPC handler: nil when the workspace may use the
// feature, otherwise an error wrapping ErrNotEntitled, ErrDisabled or
// ErrLimitReached. For a metered feature it also reads the counter (without
// spending) so an exhausted budget is refused up front. A store failure is
// returned wrapped but matches no sentinel; the check fails closed.
func (e *Engine) Require(ctx context.Context, key catalog.Key, workspaceID, userID string) error {
	d := e.Evaluate(ctx, key, workspaceID, userID)
	if d.Enabled && d.Entitlement != nil && d.Entitlement.Limit != nil {
		var err error
		if d, err = e.Usage(ctx, key, workspaceID, userID); err != nil {
			return fmt.Errorf("features: reading usage of %s: %w", key, err)
		}
		if d.Enabled && d.Usage != nil && d.Usage.Max >= 0 && d.Usage.Remaining == 0 {
			return denied(ErrLimitReached, d)
		}
	}
	return decisionError(d)
}

// RequireConsume is Require that also spends n units atomically, for handlers
// that charge per call. Nothing is spent on a refusal.
func (e *Engine) RequireConsume(ctx context.Context, key catalog.Key, workspaceID, userID string, n int64) error {
	d, err := e.Consume(ctx, key, workspaceID, userID, n)
	if err != nil {
		return fmt.Errorf("features: consuming %s: %w", key, err)
	}
	return decisionError(d)
}

// decisionError turns a decision into nil or a typed error.
func decisionError(d featurelayer.Decision) error {
	if d.Enabled {
		return nil
	}
	switch d.Reason {
	case featurelayer.ReasonLimitReached:
		return denied(ErrLimitReached, d)
	case featurelayer.ReasonNotEntitled, featurelayer.ReasonDenied:
		return denied(ErrNotEntitled, d)
	case featurelayer.ReasonStoreError:
		return fmt.Errorf("features: evaluating %s: %w", d.Feature, d.Err)
	default: // flag_*, lifecycle, prerequisite, unknown_feature
		return denied(ErrDisabled, d)
	}
}

func denied(sentinel error, d featurelayer.Decision) error {
	return fmt.Errorf("%w: %s (%s)", sentinel, d.Feature, d.Reason)
}
