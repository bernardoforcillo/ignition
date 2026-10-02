---
paths:
  - "go-packages/telemetry/**"
  - "apps/web/src/lib/analytics/**"
  - "apps/web/src/features/consent/**"
  - "apps/web/src/features/error-boundary/**"
  - "apps/web/src/stores/consent/**"
  - "apps/web/src/app/analytics-bridge.ts"
  - "apps/gateway/main.go"
trigger: glob
globs: go-packages/telemetry/**,apps/web/src/lib/analytics/**,apps/web/src/features/consent/**,apps/web/src/features/error-boundary/**,apps/web/src/stores/consent/**,apps/web/src/app/analytics-bridge.ts,apps/gateway/main.go
description: Observability is split on purpose — every log goes to stdout for the platform (GCP), only critical errors and a few business events go to PostHog, and PostHog is the browser's product-analytics tool; consent, privacy and URL scrubbing are mandatory
alwaysApply: false
---

# Rule: logs to the platform, critical errors and product analytics to PostHog

**The failure modes:**
1. **PostHog as a log store.** Shipping Info/Warn logs to PostHog costs money, buries
   the real errors and puts operational data where it does not belong.
2. **Tracking before consent.** An EU product that sends analytics before the visitor
   accepts is non-compliant, however good the dashboard looks.
3. **Secrets in URLs.** Reset, verification and invitation links carry single-use
   tokens in the query string; PostHog records `$current_url` on every event.
4. **Personal data in properties.** An email or a free-text value in an event is a
   GDPR liability that cannot be fixed by deleting a dashboard.

## The split

| Where | What | How |
|---|---|---|
| stdout (Cloud Logging on GCP, or Loki) | **every** log, as JSON | `go-packages/telemetry`, `LOG_FORMAT=gcp` for Cloud Logging's severity fields |
| PostHog, from Go | Error-level logs as `$exception` (stack trace) and the few events only the server can vouch for (`subscription_changed`) | `POSTHOG_API_KEY`; `CaptureLevel` can be raised, never lowered |
| PostHog, from the browser | pageviews, autocapture, product events, error tracking, session replay (inputs masked), feature flags | `~/lib/analytics`, after consent |
| Infra metrics (latency, rps) | not PostHog | a Prometheus/Cloud Monitoring concern |

## Do

- **Browser:** import `analytics` from `~/lib/analytics`, never `posthog-js`. Add
  events to `AnalyticsEvents` in `events.ts` first (the type is the vocabulary), then
  `analytics.track("object_verb", { location: "…", … })`. Past tense, snake_case, always a
  `location`. Properties are ids, categories, lengths or booleans: **never** an email,
  name, token or free text.
- **Consent is automatic:** PostHog starts opted out (`opt_out_capturing_by_default`);
  the banner calls `applyConsent`. Call `track` unconditionally; never wrap it in a
  consent check, and never initialise a second PostHog client.
- **Scrub URLs.** `before_send` runs `scrubProperties` on every event. Any new sensitive
  query parameter goes in `SENSITIVE_PARAMS` (`scrub.ts`) with a test.
- **Identify with the account id only** (done by the bridge on sign-in; `reset()` on
  sign-out). Never `identify` with an email.
- **Server:** log with `slog.*Context(ctx, …)`; an Error record becomes an exception
  attributed to the signed-in account (the auth interceptor sets the distinct id). Use
  `telemetry.Capture` only for a business fact the browser cannot know, with ids and
  statuses, and `$process_person_profile: false` when no person is involved.
- **Erasure:** when an account is deleted, delete its person in PostHog too.

## Do NOT

- Don't log secrets, tokens, passwords, full emails or message bodies (they carry
  single-use links), at any level: Error logs leave the process.
- Don't send Info/Warn to PostHog, and don't lower `CaptureLevel`.
- Don't add a second analytics/error-tracking SDK (Sentry, Segment, GA) next to PostHog.
- Don't capture raw search text, URLs or form values; send a length or a category.
- Don't enable PostHog autocapture of copied text or unmasked session replay.

## Verify

```bash
pnpm --filter @ignition/web test          # scrubbing, consent mapping, typed events
(cd go-packages/telemetry && go test -short -race ./...)
# no analytics SDK imported outside the analytics module (should print nothing)
grep -rn "from \"posthog-js" apps/web/src | grep -v "lib/analytics/"
```
