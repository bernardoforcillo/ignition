---
trigger: always_on
description: Code is organized by who may know about whom — UI, transport, domain, capabilities and foundations, with dependencies flowing one way and no layer skipping ahead
alwaysApply: true
---

# Rule: every layer has one job, and dependencies flow one way

Applies repo-wide, concretely to `apps/web`, `apps/gateway` and `packages/*`.
The organizing question is not "where does this file go" but "who is allowed to
know about this".

## The layers, mapped to this repo

1. **UI** — `apps/web/src/features/*`, `apps/web/src/routes/*`,
   `packages/components`. Expresses user intent. No business logic, no direct
   vendor or database calls.
2. **Transport** — `apps/web/src/lib/api` (the frontend's API client) and
   `apps/gateway/internal/adapter/httpapi` (the gateway's handlers). Validates
   input, authenticates, authorizes, and delegates. Owns no business decisions.
3. **Domain** — `apps/gateway/internal/core`. Owns product and business
   decisions. The only layer allowed to call capabilities.
4. **Capabilities / vendors** — `apps/gateway/internal/adapter/<x>` (proxy, and
   later a database, cache, email…). Each wraps exactly one external system
   behind an interface **defined by the domain layer**.
5. **Supporting foundations** — `packages/*`, cross-cutting config
   (`internal/config`), and mechanism shared by several domains. The one layer
   importable from more than one place in the chain above, and the one that must
   hold no product decisions.

## The dependency rule

Never skip a layer, and never let a lower layer import a higher one:

- `apps/web` never imports a vendor SDK or a database driver. A vendor
  integration goes through a backend service; the frontend calls the transport
  layer and nothing else.
- `internal/core` never imports `internal/adapter/*`. Core defines the
  interface; the adapter implements it and imports core.
- Transport never owns business logic: validate, authenticate, authorize,
  delegate. A vague job description ("bridge between client and backend") is how
  a handler quietly accumulates caching, retries and eventually business rules.
- The protobuf schema (`apps/*/proto`, generated, committed) *is* the typed
  request/response contract. Don't add a runtime validator (Zod, Yup) at that
  boundary.

## Build order is the dependency rule, reversed

For a feature spanning layers: data model and contracts first, then backend
wiring, then UI and polish. Building UI before the backend it depends on makes
the author (human or AI) fill the gap with an assumption that gets thrown away.
Keep each phase's in-scope / out-of-scope explicit.

## Enforce it mechanically, not just by convention

Convention erodes faster with AI agents in the loop: they reuse whatever pattern
they find. The cheapest backstop is dependency-level — don't add a vendor SDK or
DB client to `apps/web/package.json` at all; code that can't be installed can't
be imported. A Biome or `go vet`-based import-boundary check is a reasonable
next step, not yet in place.

## Naming and supply-chain hygiene

- Kebab-case for files and folders: one predictable format everywhere.
- pnpm gates dependency build scripts (`allowBuilds` in `pnpm-workspace.yaml`)
  and refuse versions younger than 7 days (`minimumReleaseAge: 10080`): most
  compromised packages are caught and yanked within days. Don't lower either, or
  add a `minimumReleaseAgeExclude` entry, without understanding why the version
  is new.
- **Go and pnpm have different threat models.** Go modules run no install-time
  scripts; their defense is `go.sum` plus the checksum database. What Go lacks
  is vulnerability scanning: run `govulncheck` in the service's lint step.

## The meta-rule

Fixing a bad dependency direction gets more expensive with every file built on
top of it, and AI agents amplify whatever structure already exists. Structure it
correctly while that is cheap.
