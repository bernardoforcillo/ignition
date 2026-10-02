---
paths:
  - "go-packages/**"
  - "packages/mailer/**"
  - "apps/gateway/internal/adapter/saas/**"
trigger: glob
globs: go-packages/**,packages/mailer/**,apps/gateway/internal/adapter/saas/**
description: The SaaS building blocks (database, identity, features, billing, mailer) — what each owns, how they are wired in the gateway, and how to extend them without breaking their boundaries
alwaysApply: false
---

# Rule: reuse the SaaS packages, extend them where they are meant to be extended

Auth, workspaces, feature gating, billing, database access and email are solved
once, in `go-packages/*` (Go modules in `go.work`) and `packages/mailer`
(TypeScript). A new product capability is built **on** them, never next to them.

**The failure modes:**
1. **Re-implementing a solved problem.** A second login flow, a hand-rolled plan
   check or a one-off email template drifts from the tested one and bypasses its
   guarantees (enumeration-safe sign-up, fail-closed entitlements, signed
   webhooks, escaped template data).
2. **Leaking a boundary.** A module that reads env, imports a sibling, or sends
   `err.Error()` to a client cannot be reused or tested.
3. **Missing generated email.** The Go mailer embeds HTML exported from React
   (`pnpm gen:email`); the output is generated, never committed. A fresh clone or
   an edited template needs the export before `go build`/`go test`.

## What each package owns

| Module | Owns | You add |
|---|---|---|
| `go-packages/database` | `*DB` (drops/pgx), ordered idempotent migrations, `dbtest` | a `database.Migration` per schema change |
| `go-packages/identity` | sign-up/login/refresh/logout, workspaces, RBAC, invitations (authlayer) | product resources and actions in `permissions/permissions.go` |
| `go-packages/features` | feature catalog, plans, flags, metered limits (featurelayer), pg stores | features, plans and add-ons in `features/catalog.go` |
| `go-packages/billing` | provider port, Stripe webhooks, idempotent event handling | a price-to-plan entry in the catalog config; a provider by implementing `billing.Provider` |
| `go-packages/mailer` | embedded templates, Resend sender, `*Mailer` | a typed `Send...` method per email a service sends |
| `packages/mailer` | the React/react.email templates and their export (design-time only, sends nothing) | a template in `src/emails/` + an entry in `src/templates.ts` |

## Do

- **Wire in one place.** `apps/gateway/main.go` builds everything through
  `internal/adapter/saas`; transport handlers depend on small consumer-side
  interfaces. SaaS is optional: it is enabled by `DATABASE_URL`.
- **Gate with the engine, not with `if plan == "pro"`.** Declare the feature in
  `features/catalog.go`, then `engine.Require(ctx, key, workspaceID, userID)` (or
  `RequireConsume` for a metered limit) and map the sentinels in the transport
  error mapper. A feature is added in the same commit that checks it.
- **Authorize with workspace permissions.** Add the resource and actions to
  `permissions/permissions.go`, then `workspace.Authorize(...)`. Roles are stored
  by name, so adding a statement never corrupts saved roles.
- **Schema changes are migrations**, appended to the ordered list and run through
  `database.Migrate`; never edit an applied migration.
- **Emails**: write the component, register it, run `pnpm gen:email`. The files
  in `go-packages/mailer/templates/` are generated and gitignored: never commit
  them (the gateway Dockerfile exports them in a Node stage).
  Pass URLs as `*Url`/`*Link` variables (validated as absolute http(s)); the
  mailer fills `CompanyName` and `AssetBaseUrl` itself. Images live in
  `packages/mailer/src/emails/static` and must be served at
  `<AssetBaseURL>/static/...`.
- **Secrets never enter the repo or the logs**: `AUTH_SECRET`, `RESEND_API_KEY`,
  `STRIPE_*` and `DATABASE_URL` are Kubernetes secrets; log recipients and
  subjects, never message bodies (they carry single-use links).
- **Billing webhooks are verified, capped and idempotent**: keep signature
  verification in the provider, the body cap in the handler, and record the event
  id only after the sink accepted the subscription.

## Do NOT

- Don't send email from TypeScript or add a provider SDK (Resend, SendGrid, ...)
  to any `package.json`: **email is sent only from Go**, through
  `go-packages/mailer`. A TS app that needs one calls a Go RPC.
- Don't read env in a library module, or import one `go-packages` module from
  another to share a type (use a port).
- Don't hand-edit or commit `go-packages/mailer/templates/`; it is generated.
- Don't add a second auth, session or email path "just for this endpoint".
- Don't relax fail-closed behavior (unknown feature, no subscription, store error
  → denied) to make a test pass.

## Verify

```bash
# the export runs and no generated email file is tracked (second command prints nothing)
pnpm gen:email && git ls-files go-packages/mailer/templates | grep -v -e .gitkeep -e .gitignore
# no email SDK or sender in the JS workspaces (should print nothing)
grep -rEn '"(resend|nodemailer|@sendgrid/mail|postmark)"' --include=package.json apps packages | grep -v node_modules
# every library module vets and passes its short tests
for m in go-packages/*/; do (cd "$m" && go vet ./... && go test -short -race ./...); done
# library modules read no env outside a FromEnv constructor (review each hit)
grep -rn "os.Getenv" go-packages --include='*.go' | grep -v _test.go
```
