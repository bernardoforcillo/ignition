# gateway

The API gateway for this workspace. It is the single public entry point that:

- reverse-proxies HTTP traffic to internal upstream services based on a
  configured path-prefix routing table (`internal/core` + `internal/adapter/proxy`),
- serves a small Connect RPC control-plane API of its own (`proto/gateway/v1`,
  currently just a `Ping` RPC to prove the transport is wired end-to-end), and
- exposes `/healthz` and `/readyz` as plain HTTP endpoints for liveness/readiness
  probes.

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

```sh
docker build -t gateway apps/gateway
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

At least one of `GATEWAY_ROUTES` or `GATEWAY_UPSTREAM_URL` must be set — the
process refuses to start with zero routes configured.

Example multi-route config:

```
GATEWAY_ROUTES="/api/users=http://users-service:8081,/api/orders=http://orders-service:8082,/=http://web-frontend:3000"
```

## Endpoints

- `GET /healthz` — liveness. Always 200 once the process is up; no dependency checks.
- `GET /readyz` — readiness. 200 once at least one route is configured, 503 otherwise.
- `POST /gateway.v1.GatewayService/Ping` (Connect, gRPC, or gRPC-Web) — the
  gateway's own control-plane RPC. Try it with `curl` over Connect's plain
  HTTP/1.1 JSON protocol:

  ```sh
  curl -s http://localhost:8080/gateway.v1.GatewayService/Ping \
    -H "Content-Type: application/json" \
    -d '{"message":"hello"}'
  ```

- Everything else — reverse-proxied to whichever configured route's path
  prefix matches, longest prefix wins. Gated by the rate-limit and auth
  middleware (both no-ops by default; see the config table above).

## Package layout

This mirrors `code-organization.md`'s five-layer, dependency-direction shape,
applied via that file's own "Applying this to Go" section:

```
apps/gateway/
  main.go                          composition root — the only file that
                                    imports a concrete adapter directly
  proto/gateway/v1/gateway.proto   Connect RPC schema (source of truth)
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
`core` imports only the standard library; `adapter/proxy` and `adapter/httpapi`
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
`proto/gateway/v1/gateway.proto`:

```sh
# one-time setup
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest
# buf itself: https://buf.build/docs/installation

cd apps/gateway
buf generate
```

Commit the resulting diff under `internal/gen/` along with the `.proto` change.
