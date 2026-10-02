# billing

Provider-agnostic subscription billing for a SaaS. Standard library only; no
payment SDK. The tenant is a **workspace**. This module decides *what a
workspace pays for*; enforcing it is the features/entitlements module's job and
this module imports neither it nor the sibling packages.

## Flow

```
 user clicks "Upgrade"
        |
        v
 Service.StartCheckout --> Provider.CreateCheckout --> hosted checkout URL
        |                                   (workspace ID tagged on the subscription)
   user pays at provider
        |
        v   webhook (signed)
 httpwebhook.Handler
   - cap body, POST only
   - Provider.ParseWebhook   verifies signature, normalizes to billing.Event
   - Service.HandleEvent     dedupe by event ID (EventStore)
                             price IDs -> plan / add-ons (Catalog)
                             apply status policy
        |
        v
 SubscriptionSink.SetSubscription(workspaceID, billing.Subscription)
        |
        v   (adapter owned by the features module)
 entitlement SubscriptionStore  -->  feature checks
```

`billing.Subscription` carries plan ID, add-on IDs, status, trial end and
current period. The features module implements (or adapts) `SubscriptionSink`,
mapping those fields onto its own subscription type.

## Wiring

```go
catalog, err := billing.NewCatalog(billing.CatalogConfig{
    FreePlanID: "free",
    Prices: []billing.Price{
        {ProviderPriceID: "price_...", Kind: billing.KindPlan, ID: "pro"},
        {ProviderPriceID: "price_...", Kind: billing.KindAddOn, ID: "extra-seats"},
    },
})
prov, err := stripe.New(stripe.Config{WebhookSecret: cfg.StripeWebhookSecret, APIKey: cfg.StripeKey, Customers: lookup})
svc := billing.NewService(catalog, prov, sink, store, logger)
mux.Handle("POST /webhooks/billing", httpwebhook.New(prov, svc, 0, logger))
```

Secrets and price IDs come from the app's config; the module reads no
environment.

## Status policy

| Provider state | Plan pushed to the sink | Status |
|---|---|---|
| trialing, active | paid plan + add-ons | as is |
| past_due (failed invoice) | paid plan kept (grace while the provider retries) | `past_due` so entitlements can restrict |
| canceled / subscription deleted | free plan, no add-ons | `canceled` |
| incomplete | free plan until the first payment succeeds | `incomplete` |

A subscription with an unknown price or two distinct plans is rejected
(`ErrUnknownPrice`, `ErrAmbiguousPlan`) rather than guessed.

## HTTP status mapping

200 applied / replayed / ignored event type; 400 bad signature or payload;
405 non-POST; 413 body over cap (default 1 MiB); 422 valid but unusable event
(unknown price, no workspace; retrying cannot help, fix the catalog or
metadata); 500 anything else, e.g. the sink is down, so the provider retries.

## Adding a provider

1. New package `billing/<name>` implementing `billing.Provider`.
2. `ParseWebhook` verifies the signature **first** and returns
   `ErrInvalidSignature`, then maps the payload to `billing.Event`
   (`ErrMalformedEvent` on bad input, `ErrIgnoredEvent` for unhandled types).
   Invoice events set `Status` themselves (paid -> active, failed -> past_due).
3. Put the workspace ID in provider metadata at checkout so every event carries it.
4. Test with a signed fixture and `httptest`; see `stripe/stripe_test.go`.

`billingtest.FakeProvider`, `MemoryEventStore` and `RecordingSink` are
hand-written fakes for tests of your own code.

## Security notes

- **Verify signatures** over the raw body before parsing. The Stripe adapter
  does HMAC-SHA256 over `t.payload`, compares in constant time, accepts any
  `v1` (secret rotation) and rejects timestamps outside a 5-minute window.
- **Cap the body** (`http.MaxBytesReader`) before reading.
- **Idempotency**: providers deliver at least once. `HandleEvent` records the
  event ID only after the sink accepted the subscription, so sink failures are
  retried and replays are no-ops. `SetSubscription` must be a set, not an
  increment. Events can arrive out of order; the next event corrects state.
- Never log payloads or secrets; the handler logs errors only.
- Never hard-code keys; pass them through `stripe.Config`.

## Not included

- **Postgres `EventStore` and its migration are left to the app.** Suggested
  shape: `CREATE TABLE billing_events (event_id text PRIMARY KEY, processed_at timestamptz NOT NULL DEFAULT now());`
  with `Seen` = `SELECT 1`, `Record` = `INSERT ... ON CONFLICT DO NOTHING`.
- The Stripe adapter is a skeleton: webhook verification and parsing are
  complete; Checkout and portal calls are minimal form-encoded REST requests
  (no retries, idempotency keys, or API-version pinning).
- Mapping workspace -> Stripe customer ID (`stripe.Config.Customers`).
