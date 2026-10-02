# features

The template's feature-management layer: **can workspace W (user U) use feature
F right now, and how much of it is left?** It wraps
[featurelayer](https://github.com/bernardoforcillo/featurelayer) and adds the
default catalog, typed errors for the transport layer, and Postgres stores.
Tenant = workspace.

## The model

| Concept | What it is | Where it lives |
|---|---|---|
| **Feature** | A capability with a stable key and a lifecycle (`draft` to `retired`). | `catalog.go` (code) |
| **Flag** | A switch on a feature: kill switch, rollout, segment targeting. Off means off for everyone, whatever the plan. | `catalog.go` |
| **Plan** | A set of entitlements (a feature, optionally with a metered limit and period). Plans can `Extends` another. | `catalog.go` |
| **Add-on** | Extra entitlements/limits stacked on a plan; can `Requires` specific plans. | `catalog.go` |
| **Subscription** | Which plan, add-ons, trial and per-tenant grants a workspace holds. | `pgstore` (Postgres) |
| **Usage** | One counter per (workspace, feature, period); a new period is a new row, so nothing resets. | `pgstore` |

Definitions are code (reviewed, deployed); per-workspace state is data.
Everything fails closed: unknown feature, no subscription, store error all mean "no".

`catalog.go` is a **starter to replace**: `data.export` (boolean, Pro),
`api.calls` (metered, monthly: 1k Free / 100k Pro) with a kill-switch flag,
and an `extra-api-calls` add-on that requires Pro.

## Wiring

```go
db := pg.New(stdlib.New(sqlDB))
engine, err := features.New(
	pgstore.NewSubscriptionStore(db),
	pgstore.NewUsageStore(db),
)
```

Apply `pgstore.Migrations()` (ordered `{ID, SQL}`, idempotent) with your
migration runner. `features.New` validates the catalog, so a bad definition
is a startup error. Options: `WithConfig`, `WithClock`, `WithLogger`.

## Add a feature

1. Add a typed key in `catalog.go` (`const Foo catalog.Key = "foo"`). Keys are
   stable; add rather than rename, or stored counters and grants are orphaned.
2. Add a `catalog.Feature` to `Config().Features`.
3. Grant it from a plan or add-on (`entitlement.Entitlement{Feature: Foo}` or
   `entitlement.Limited(Foo, 100, entitlement.Month)`). A feature no plan
   grants can never be used.
4. Add the check (below) in the same commit.

## Gate an RPC handler

```go
if err := h.features.Require(ctx, features.DataExport, workspaceID, userID); err != nil {
	return nil, toConnectError(err)
}

func toConnectError(err error) error {
	switch {
	case errors.Is(err, features.ErrNotEntitled):
		return connect.NewError(connect.CodePermissionDenied, err)
	case errors.Is(err, features.ErrLimitReached):
		return connect.NewError(connect.CodeResourceExhausted, err)
	case errors.Is(err, features.ErrDisabled):
		return connect.NewError(connect.CodeUnavailable, err)
	default: // store failure: the check failed closed
		return connect.NewError(connect.CodeInternal, err)
	}
}
```

- `Require` reads the meter for limited features without spending.
- `RequireConsume(ctx, key, ws, user, n)` checks and spends atomically
  (nothing is spent on a refusal).
- `Evaluate` returns the full `Decision` (reason, limit, usage) and `Allowed`
  the bool, for UI flags. `Usage` reads a counter; `Consume` spends and returns
  the remaining budget.

## How billing sets subscriptions

The billing module owns the Stripe-side truth and mirrors it with one call on
checkout completion, plan change, trial start or cancellation:

```go
err := subs.Set(ctx, entitlement.Subscription{
	TenantID:      workspaceID,
	Plan:          features.PlanPro,
	AddOns:        []entitlement.AddOnID{features.AddOnExtraAPICalls},
	BillingAnchor: periodStart, // first insert only
})
```

`Set` is idempotent (safe for webhook retries) and keeps the stored billing
anchor on update, so a plan change never grants a fresh budget mid-period. On
cancellation set `Plan` to `features.PlanFree` (or `Delete` to entitle nothing).
Create a free subscription when a workspace is created, otherwise it has no
entitlements.

## Tests

`go test -short ./...` uses featurelayer's in-memory stores. The Postgres tests
in `pgstore` skip unless `TEST_DATABASE_URL` is set; they truncate
`feature_subscriptions` and `feature_usage`, so point them at a scratch database.
