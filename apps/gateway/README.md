# gateway

The API gateway for this workspace. It is the single public entry point that:

- reverse-proxies HTTP traffic to internal upstream services based on a
  configured path-prefix routing table (`internal/core` + `internal/adapter/proxy`),
- serves a small Connect RPC control-plane API of its own (`proto/gateway/v1`,
  currently just a `Ping` RPC to prove the transport is wired end-to-end),
- exposes `/healthz` and `/readyz` as plain HTTP endpoints for liveness/readiness
  probes, and
- **optionally** serves the SaaS surface (accounts, workspaces, feature checks,
  Stripe billing) built from the `go-packages/*` modules; see "SaaS surface"
  below. It is off unless `DATABASE_URL` is set, and with it off the gateway
  behaves as a pure proxy exactly as before.

This is the first Go service in this template — there was no sibling app or
established convention to mirror, so the internal layout follows the
technology-agnostic `code-organization.md` and `scaling-and-infra.md`
references directly (see "Package layout" below).

The HTTP framework is [Amaro](https://github.com/buildwithgo/amaro) — a
small, zero-dependency, Hono-inspired Go router/framework — rather than
plain `net/http`. Routing, request logging, rate limiting, and bearer-token
auth all come from Amaro's own facilities (`amaro.App.Mount` for hosting the
generated Connect handler, `middlewares.Logger`, `middlewares.RateLimiter`,
`middlewares.KeyAuthWithConfig`); structured panic recovery is the one piece
still hand-rolled, to keep it on the same slog/JSON logging as everything
else this service emits (see
`internal/adapter/httpapi/middleware/recover.go`).

> **Dependency stability note:** `go.mod` pins `github.com/buildwithgo/amaro`
> to a pseudo-version on its `main` branch (past the `v0.4.0` tag), because
> `App.Mount` — what this service uses to host the Connect handler — was
> added after `v0.4.0` was cut. Amaro is a very young, single-maintainer
> project; treat this as less stable than a typical tagged dependency, and
> re-pin to a real tag (`go get github.com/buildwithgo/amaro@vX.Y.Z`) once one
> exists that includes `Mount`.

## Running it

### With Air (hot reload, recommended for local dev)

```sh
go install github.com/air-verse/air@latest
cd apps/gateway
GATEWAY_UPSTREAM_URL="http://localhost:9000" air
```

### Without Air

```sh
cd apps/gateway
GATEWAY_UPSTREAM_URL="http://localhost:9000" go run .
```

### As a container

The build context is the **repository root**, because the gateway depends on the
sibling modules in `go-packages/` (see "Dependencies and the workspace" below):

```sh
docker build -f apps/gateway/Dockerfile -t gateway .
docker run -p 8080:8080 -e GATEWAY_UPSTREAM_URL="http://host.docker.internal:9000" gateway
```

## Configuration

All configuration is environment variables — no config file, matching Twelve-Factor
Factor III ("Config").

| Variable                   | Default | Meaning                                                                 |
| --------------------------- | ------- | ------------------------------------------------------------------------ |
| `GATEWAY_LISTEN_ADDR`       | `:8080` | Address the HTTP server listens on.                                     |
| `GATEWAY_ROUTES`            | —       | Routing table: `prefix=url,prefix=url,...`. Takes precedence over `GATEWAY_UPSTREAM_URL`. |
| `GATEWAY_UPSTREAM_URL`      | —       | Single catch-all upstream, mounted at `/`. Used only if `GATEWAY_ROUTES` is unset. |
| `GATEWAY_AUTH_TOKEN`        | —       | Shared bearer token checked on proxied requests. Empty disables auth entirely (no-op middleware). |
| `RATE_LIMIT_RPS`            | disabled | Requests/second per client IP, token-bucket. Unset or `<=0` disables the limiter. |
| `RATE_LIMIT_BURST`          | `20`    | Token bucket burst size (only relevant once `RATE_LIMIT_RPS` is set).   |
| `GATEWAY_SHUTDOWN_TIMEOUT`  | `10s`   | Grace period for in-flight requests during shutdown.                    |

#### SaaS variables

Read only when `DATABASE_URL` is set (that is what enables the SaaS surface);
ignored otherwise. Secrets belong in a `secretKeyRef`, never in a ConfigMap.

| Variable                | Default | Meaning |
| ------------------------ | ------- | ------- |
| `DATABASE_URL`           | —       | Postgres DSN. **Setting it enables the SaaS surface**; migrations run at startup. |
| `AUTH_SECRET`            | —       | **Required with SaaS.** HS256 access-token signing key, at least 32 bytes. |
| `APP_URL`                | —       | **Required with SaaS.** Public URL of the web app: base of email links and of Stripe return URLs. |
| `COMPANY_NAME`           | —       | **Required with SaaS.** Name shown in emails. |
| `MAIL_FROM`              | —       | **Required with SaaS.** Sender, e.g. `Ignition <hello@example.com>`. |
| `MAIL_REPLY_TO`          | —       | Reply-to address. |
| `ASSET_BASE_URL`         | `APP_URL` | Origin serving the email images (`<origin>/static/...`). |
| `RESEND_API_KEY`         | —       | Resend key. Unset logs each email (recipient and subject only) instead of sending it: fine for local dev, not for production. |
| `ACCESS_TTL`             | `15m`   | Access-token lifetime. |
| `REFRESH_TTL`            | `720h`  | Refresh-token lifetime. |
| `STRIPE_WEBHOOK_SECRET`  | —       | `whsec_...`. **Setting it enables billing** (`BillingService` and `POST /webhooks/stripe`). |
| `STRIPE_API_KEY`         | —       | Stripe key for checkout/portal calls; without it those RPCs fail (webhooks still work). |
| `BILLING_PRICES`         | —       | Price catalog `price_id=plan:<plan>,price_id=addon:<add-on>,...`, e.g. `price_123=plan:pro,price_456=addon:extra-api-calls`. Plan and add-on ids must exist in the feature catalog (`go-packages/features/catalog.go`) or startup fails. Only these prices can be checked out. |
| `BILLING_FREE_PLAN`      | the features catalog's free plan (`free`) | Plan a new or lapsed workspace holds. |

Optional pool settings of `go-packages/database` (`DATABASE_MAX_OPEN_CONNS`,
etc.) are not wired; the module's defaults apply.

At least one of `GATEWAY_ROUTES` or `GATEWAY_UPSTREAM_URL` must be set — the
process refuses to start with zero routes configured.

Example multi-route config:

```
GATEWAY_ROUTES="/api/users=http://users-service:8081,/api/orders=http://orders-service:8082,/=http://web-frontend:3000"
```

## Endpoints

- `GET /healthz` — liveness. Always 200 once the process is up; no dependency checks.
- `GET /readyz` — readiness. 200 once at least one route is configured (and,
  with SaaS on, the database answers a ping), 503 otherwise.
- `POST /gateway.v1.GatewayService/Ping` (Connect, gRPC, or gRPC-Web) — the
  gateway's own control-plane RPC. Try it with `curl` over Connect's plain
  HTTP/1.1 JSON protocol:

  ```sh
  curl -s http://localhost:8080/gateway.v1.GatewayService/Ping \
    -H "Content-Type: application/json" \
    -d '{"message":"hello"}'
  ```

- With SaaS on: the Connect procedures and webhook listed under "SaaS surface".
  They are registered before the proxy catch-all, so they always win over it.
- Everything else — reverse-proxied to whichever configured route's path
  prefix matches, longest prefix wins. Gated by the rate-limit and auth
  middleware (both no-ops by default; see the config table above).

## SaaS surface

Enabled by `DATABASE_URL`. `main.go` opens the database and calls
`saas.Build` (`internal/adapter/saas`), which runs all migrations (identity
schema, features stores, billing tables) through `go-packages/database`, then
wires `go-packages/{identity,features,billing,mailer}`. Any replica can start
first: migrations are advisory-locked and idempotent.

Connect procedures (`proto/saas/v1`, package `saas.v1`; JSON over HTTP works
with plain `curl` as for `Ping`):

| Service | Procedure | Auth | Notes |
| ------- | --------- | ---- | ----- |
| `AuthService` | `SignUp`, `VerifyEmail`, `Login`, `Refresh` | public | `SignUp` answers identically whether or not the email exists. |
| `AuthService` | `Logout` | bearer | Revokes the session behind a refresh token. |
| `WorkspaceService` | `CreateWorkspace`, `GetWorkspace`, `ListMembers`, `InviteMember`, `AcceptInvite` | bearer | Membership and role permissions are enforced by the workspace service. A new workspace starts on the free plan. |
| `FeatureService` | `CheckFeature` | bearer + member | Returns `enabled`, `reason` and, for a finite meter, `limit` and `remaining`. Consumes nothing. |
| `BillingService` | `StartCheckout`, `OpenPortal` | bearer + `organization:update` (owner/admin) | Served only when billing is enabled. Return URLs are built from `APP_URL`, never taken from the client. |

Plain HTTP: `POST /webhooks/stripe` (billing enabled only; verified by Stripe
signature, not a bearer token; 400/405/413/422/500 per `go-packages/billing`'s
`httpwebhook`).

Authentication is `Authorization: Bearer <access token>`, verified locally from
the signature (no store call). Everything not listed as public is denied
without a token by default (`internal/adapter/httpapi/interceptor.go`).

Things worth knowing:

- **Errors** are mapped once (`httpapi/errors.go`): a domain sentinel becomes a
  Connect code with a constant message; any other failure is logged and answered
  with `internal error`. Non-members get `not_found`, not `permission_denied`.
- **Rate limiting of the public auth RPCs** uses the identity service's own
  per-IP budgets (20 logins / 15 min, 10 sign-ups / hour), backed by an
  in-memory fixed-window limiter (`adapter/saas/ratelimit.go`). Gaps: it is per
  replica, and behind a proxy or ingress the peer address is the proxy's, so
  all clients share one budget until a trusted `X-Forwarded-For` is resolved.
  Amaro's route limiter cannot be used here because it does not apply to
  mounted Connect handlers.
- **Billing** keeps a workspace-to-Stripe-customer mapping learned from verified
  webhooks (`billing_customers`); `OpenPortal` answers `failed_precondition`
  until a workspace has completed a checkout.
- **Access tokens are stateless**: `Logout` revokes the refresh token, but an
  issued access token lives until `ACCESS_TTL` expires.

## Dependencies and the workspace

`go.mod` requires the five local modules with relative `replace` directives
(`../../go-packages/<name>`), so the gateway builds both inside the repo's
`go.work` and standalone (`GOWORK=off go build ./...`), which is what the
Docker build relies on. Hence the repo-root build context above.

## Package layout

This mirrors `code-organization.md`'s five-layer, dependency-direction shape,
applied via that file's own "Applying this to Go" section:

```
apps/gateway/
  main.go                          composition root — the only file that
                                    imports a concrete adapter directly
  proto/gateway/v1/gateway.proto   Connect RPC schema (source of truth)
  proto/saas/v1/*.proto            auth, workspace, feature, billing schemas
  internal/
    gen/                           generated from proto/ via `buf generate`;
                                    checked in so `go build` never requires
                                    the buf/protoc toolchain
    core/                          domain layer — routing decision + ports
      route.go                     Route, Router: given a path, which
                                    upstream owns it (the one real business
                                    decision a gateway makes)
      forwarder.go                 Forwarder: the port a capability
                                    adapter must implement to actually
                                    move bytes to/from the matched target
    config/                        shared-foundations layer — env var
                                    loading. Deliberately domain-agnostic
                                    (RouteSpec, not core.Route) since
                                    shared foundations sits beneath domain
                                    and must not import upward into it
    adapter/
      saas/                        composition adapter for the SaaS surface:
                                    Build() = migrations + identity,
                                    features, billing, mailer wiring; also
                                    the billing sink/event store, the
                                    new-workspace free plan and the
                                    in-memory auth rate limiter
      proxy/                       capabilities/adapter layer — implements
                                    core.Forwarder using
                                    net/http/httputil.ReverseProxy
      httpapi/                     transport layer — validate, authenticate,
                                    authorize, delegate; nothing more
        server.go                  builds the *amaro.App: mounts probes,
                                    the Connect handler (amaro.App.Mount),
                                    and the proxy catch-all gated by
                                    Amaro's rate-limit/key-auth
                                    middleware; depends only on core's
                                    interfaces, never on adapter/proxy
                                    directly
        health.go                  /healthz, /readyz
        gatewayservice.go          Connect GatewayService.Ping implementation
        saas.go                    SaaS struct, consumer-side interfaces
                                    (authService, workspaceService,
                                    featureChecker, checkoutStarter) and the
                                    mounting of the SaaS routes
        authservice.go, workspaceservice.go,
        featureservice.go, billingservice.go
                                   one Connect handler per service
        interceptor.go             bearer-token authentication; the public
                                    procedure allow-list
        errors.go                  the one domain-error -> Connect mapper
        middleware/                the one hand-rolled middleware left:
                                    structured (slog) panic recovery.
                                    Logging, auth, and rate limiting come
                                    directly from
                                    github.com/buildwithgo/amaro/middlewares
```

Why no UI layer: this is a backend-only service with nothing that "expresses
user intent" in the UI sense, so that layer is simply absent — not every
deployable populates all five layers.

Dependency direction is enforced by what each package is allowed to import:
`core` imports only the standard library (`core/errors.go` holds the one
SaaS-facing sentinel, `ErrNoBillingCustomer`); `adapter/proxy` and `adapter/httpapi`
import `core` (never the reverse); `config` imports neither `core` nor any
adapter; `main.go` is the only place that imports a concrete adapter package
directly and wires it in behind the interface (`core.Forwarder`,
`*core.Router`) that the transport layer actually depends on.

### Scaling-and-infra choices worth calling out

- **Auth is a no-op by default.** There's no token-issuance scheme in this
  template yet, so `server.go` only registers Amaro's
  `middlewares.KeyAuthWithConfig` (configured to read a `Bearer` token from
  the `Authorization` header) once `GATEWAY_AUTH_TOKEN` is set — a local
  check, never a call to a central auth service per request (see
  `scaling-and-infra.md`, AuthN vs authZ).
- **Rate limiting ships disabled by default** (`RATE_LIMIT_RPS` unset). Per
  `scaling-and-infra.md`'s rate-limiting trigger/action rule, it should stay
  off until there's a concrete, observed abuse or cost-blowup pattern — set
  `RATE_LIMIT_RPS`/`RATE_LIMIT_BURST` once that pattern actually exists rather
  than shipping a guessed number turned on from day one. The limiter is
  Amaro's own `middlewares.RateLimiter`: an in-memory token bucket (no Redis,
  no new backing service) that self-evicts idle entries every minute — the
  smallest mechanism that satisfies "checked at the edge."
  **Known limitation:** it keys on the raw `http.Request.RemoteAddr`
  (host:port), not a stripped client IP, so two requests from the same
  client over two different connections (the common case — most clients
  don't keep one connection alive across requests) are tracked as different
  "clients" and each gets its own fresh bucket. This under-limits in
  practice; if that becomes a real problem, either wrap the middleware to
  normalize `RemoteAddr` (via `net.SplitHostPort`, and `X-Forwarded-For` if
  this sits behind another proxy) before calling into Amaro's limiter, or
  reach for a real edge/gateway-level rate limiter per
  `scaling-and-infra.md`.
- **No gateway-of-gateways, no service split.** This is one stateless Go
  binary; horizontal scaling is just running more replicas behind a load
  balancer, per the Twelve-Factor statelessness and horizontal-first-scaling
  guidance — nothing here assumes in-process state survives a restart or a
  request landing on a different replica.

## Regenerating the Connect/protobuf code

The generated code under `internal/gen/` is checked in, so a plain `go build`
never needs the buf toolchain. Regenerate it after changing
`proto/gateway/v1/gateway.proto` or anything under `proto/saas/v1/`:

```sh
# one-time setup
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest
# buf itself: https://buf.build/docs/installation

cd apps/gateway
buf lint
buf generate
```

Commit the resulting diff under `internal/gen/` along with the `.proto` change.
