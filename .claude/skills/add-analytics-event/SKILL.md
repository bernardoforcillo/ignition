---
name: add-analytics-event
description: Use when adding a PostHog product-analytics event, instrumenting a screen or flow, gating something behind a PostHog feature flag, or reporting a server-side business event in this repository — covers the typed event vocabulary, consent, privacy, URL scrubbing and the browser-vs-server split.
---

# Add an analytics event

PostHog is already wired: browser through `~/lib/analytics` (consent, scrubbing, identify and
pageviews are automatic), server through `go-packages/telemetry`. You add an event; you do NOT
initialise PostHog or check consent. Rules: `.claude/rules/observability.md`.

## Browser event (the normal case)

1. **Declare it** in `apps/web/src/lib/analytics/events.ts`:
   ```ts
   export interface AnalyticsEvents {
     // …
     report_exported: { location: "reports"; format: string };
   }
   ```
   Names are `object_verb`, snake_case, past tense. Every event has a `location` naming the
   screen. Properties are primitives: an id, a category, a length, a boolean.
2. **Capture it** where the action *succeeds* (a mutation's `onSuccess`, not the click):
   ```ts
   import { analytics } from "~/lib/analytics";
   analytics.track("report_exported", { location: "reports", format: "csv" });
   ```
3. **Test the logic** if it computes a property (`lib/analytics/*.test.ts` style). The wiring
   itself needs no test: `track` is typed.

## Privacy checklist (EU product)

- No email, name, token, URL, or free text in properties: send `query_length`, a category or a
  hash instead of the raw value.
- A new screen whose URL carries a secret (`?token=`, `?code=`) needs the parameter in
  `SENSITIVE_PARAMS` (`scrub.ts`) plus a test case in `analytics.test.ts`.
- Session replay masks every input; add the class `ph-no-capture` to any element showing
  sensitive text.

## Feature flag

```ts
const on = analytics.isFeatureEnabled("new-billing-page");      // read
const off = analytics.onFeatureFlags(() => rerender());          // subscribe (returns unsubscribe)
```
Create the flag in PostHog, roll it out to internal users first, delete the flag and the guard
when it ships to everyone.

## Server event (rare)

Only for a business fact the browser cannot know (a webhook-driven plan change). Capture it where
the fact becomes true, after the write succeeded:
```go
tel.Capture(ctx, "workspace:"+id, "subscription_changed", map[string]any{
	"plan_id": planID, "status": status, "$process_person_profile": false,
})
```
Errors need nothing: `slog.ErrorContext(ctx, "…", "error", err)` already reaches PostHog Error
tracking (attributed to the signed-in account). Everything below Error stays in stdout logs.

## Common mistakes

| Mistake | Do instead |
|---|---|
| `import posthog from "posthog-js"` in a component | `import { analytics } from "~/lib/analytics"` |
| `if (hasConsent) analytics.track(…)` | call `track` unconditionally; opt-out is automatic |
| `track("click", { text: button.label })` | a declared event with a stable property |
| An email or raw query in a property | a length, category or id |
| `identify(user.email)` | the account id (the bridge does it) |
| Info/Warn logs "for PostHog" | stdout is the log channel; PostHog is for Error and business events |
