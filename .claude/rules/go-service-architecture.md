---
paths:
  - "apps/gateway/**/*.go"
  - "go-packages/**/*.go"
trigger: glob
globs: apps/gateway/**/*.go,go-packages/**/*.go
description: Go services keep a hexagonal layout (core, adapter, config) wired only in main.go, use native Connect for RPC, map errors once at the inbound adapter, and read config and log through one place each
alwaysApply: false
---

# Rule: a Go service is core ← adapter, wired in one place

Go code under `apps/` and `go-packages/` follows the layout `apps/gateway` already has. It is what
lets logic be tested with fakes and a transport be added without touching it.

**The failure modes:**
1. **Infrastructure inside the handler.** A handler holding a DB or HTTP client
   can only be tested against the real thing, and the handler, the business
   rule and the query all change together.
2. **Raw errors to the client.** Returning `err.Error()` sends SQL, hostnames
   and internals to the caller. A Connect handler that returns a plain `error`
   reaches the client as `Unknown`: connect-go only maps `*connect.Error`.
3. **Wiring everywhere.** Constructors that read env vars or build their own
   dependencies cannot be reused or tested.

Scaffold a new service with `pnpm gen service`: it writes this layout, registers the
module in `go.work` and (optionally) the Kubernetes manifests in every environment.

## Layout

```text
apps/<svc>/
  main.go                 # the only composition root: config → adapters → core → servers
  internal/config/        # env → Config, validated at startup
  internal/core/          # business logic; imports stdlib only, declares the ports it needs
  internal/adapter/<x>/   # inbound (httpapi) and outbound (proxy, db, …) adapters
```

- **Only the folders you need.** No `port/` package until a second adapter
  exists: a consumer-declared interface in `core` is enough.
- **`main.go`** builds config, outbound adapters, core, inbound adapters and
  servers, and shuts down gracefully using `cfg.ShutdownTimeout`.
- **Adapters assert their port**: `var _ core.Forwarder = (*Proxy)(nil)`.
- **Constructors take required dependencies as parameters.** `Set*`/`With*`
  only for a dependency that is genuinely optional.

## Library modules (`go-packages/*`)

Reusable domain code lives in its own Go module under `go-packages/<name>`
(listed in `go.work`); see `saas-packages.md` for the ones that ship. A library
module:

- reads **no environment** (a `FromEnv` constructor at most), owns no transport
  and no `main`;
- declares the ports it needs (`Mailer`, `SubscriptionSink`, `EventStore`) next to
  the code that uses them, and exports sentinel errors for the transport layer;
- is wired **only** in the composition root: `apps/gateway/main.go` through
  `internal/adapter/saas`, which builds the stores, senders and services and hands
  the transport layer small consumer-side interfaces;
- never imports another `go-packages` module unless the dependency is the point
  (billing and features stay decoupled through a port the app implements).

## Do

- **Connect for every client-facing API.** Anything the web or another typed client calls
  is a Connect RPC defined in `/proto`: never a hand-rolled JSON/HTTP endpoint. Plain HTTP
  is only for what a third party dictates: provider callbacks (the OAuth redirect), provider
  webhooks (Stripe), probes (`/healthz`, `/readyz`) and the signed URLs object storage
  serves. Even then, keep the handler a thin adapter that hands off to the same services
  the RPCs use.
- **Transport by caller.** Browser or typed client → a **native Connect**
  handler whose service is generated from the shared `/proto` into
  `go-packages/proto` (`protobuf-codegen.md`). Plain HTTP only for probes (`/healthz`,
  `/readyz`) and proxying.
- **Map errors once, at the inbound adapter.** `core` returns sentinel errors
  (`var ErrNotFound = errors.New("…")`); the inbound adapter has one mapper:
  `connect.NewError(connect.CodeNotFound, errors.New("route not found"))`.
  For internal failures send a constant message to the client and log the
  detail with `slog.ErrorContext(ctx, "…", "error", err)`.
- **Config** through `internal/config` only; every new variable is documented
  in the `Load` comment and, when deployed, in the
  `configmap.yaml` of each environment (`kubernetes-manifests.md`). Secrets
  arrive as env vars from a `secretKeyRef`.
- **Logging**: `log/slog` with key-value pairs, JSON in production.
- **Context**: `ctx context.Context` is the first parameter of every core and
  port method that does I/O.

## Do NOT

- Don't import an HTTP framework, DB driver or gRPC/Connect type in
  `internal/core`; don't import one adapter from another.
- Don't send `err.Error()` to a client.
- Don't read `os.Getenv` outside `internal/config`.

## Verify

`git diff` shows new files only once git knows them: run `git add -N <files>`
first. Each block prints nothing when the branch is clean.

```bash
BASE=$(git merge-base origin/main HEAD)
# framework or transport types added to core
git diff -U0 "$BASE" -- 'apps/*/internal/core/*.go' \
  | grep -E '^\+.*("connectrpc.com|"net/http|buildwithgo/amaro)'
# raw error text sent to a client
git diff -U0 "$BASE" -- '*.go' \
  | grep -E '^\+.*connect\.NewError\(connect\.Code[A-Za-z]+, *(err|fmt\.Errorf)'
# env access outside config
git diff -U0 "$BASE" -- '*.go' ':!apps/*/internal/config/*' | grep -E '^\+.*os\.Getenv'
```

Vet and short tests: `go-testing.md`.
