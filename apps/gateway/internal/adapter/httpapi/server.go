// Package httpapi is the gateway's transport layer. Per
// code-organization.md its job is deliberately narrow — validate,
// authenticate, authorize, delegate — nothing more: it mounts the
// gateway's own Connect RPC surface, its health/readiness probes, and
// the reverse-proxy catch-all, wraps them in the middleware chain, and
// delegates every real decision to internal/core (what to route where)
// or to whatever Forwarder was handed to it (how to actually get a
// request there). It depends only on internal/core's exported
// interfaces (Router, Forwarder) — never on a concrete adapter package
// — so the only place a concrete adapter (internal/adapter/proxy) gets
// wired in is the composition root, main.go.
//
// The HTTP framework itself is github.com/buildwithgo/amaro (a small,
// zero-dependency, Hono-inspired router/framework) rather than plain
// net/http: routing, request logging, rate limiting, and bearer-token
// auth all come from Amaro's own facilities (amaro.App.Mount,
// middlewares.Logger, middlewares.RateLimiter, middlewares.KeyAuthWithConfig);
// see internal/adapter/httpapi/middleware/recover.go for the one piece
// that's still custom (structured slog panic logging, to match every
// other log line this service emits).
//
// NOTE: this pins github.com/buildwithgo/amaro to a post-v0.4.0 commit
// on its main branch (a pseudo-version, not a tagged release) because
// App.Mount — what this file uses to host the generated Connect
// handler as a standard http.Handler — was added after v0.4.0 was cut.
// Amaro is a very young, single-maintainer project (1 GitHub star at
// the time of writing); treat this dependency as less stable than a
// typical tagged release and re-pin to a real tag once one exists that
// includes Mount.
package httpapi

import (
	"context"
	stdlog "log"
	"log/slog"
	"net/http"
	"time"

	"github.com/buildwithgo/amaro"
	"github.com/buildwithgo/amaro/middlewares"
	"github.com/buildwithgo/amaro/routers"

	"github.com/bernardoforcillo/ignition/apps/gateway/internal/adapter/httpapi/middleware"
	"github.com/bernardoforcillo/ignition/apps/gateway/internal/core"
	"github.com/bernardoforcillo/ignition/apps/gateway/internal/gen/gateway/v1/gatewayv1connect"
)

// ServerConfig holds everything NewServer needs to build the
// gateway's HTTP handler. Router and Forwarder are domain/port types,
// not concrete adapters — see the package doc above.
type ServerConfig struct {
	Router         *core.Router
	Forwarder      core.Forwarder
	AuthToken      string
	RateLimitRPS   float64
	RateLimitBurst int
	Logger         *slog.Logger

	// SaaS is the optional accounts/workspaces/features/billing surface;
	// nil serves the pure proxy (plus Ping) exactly as before.
	SaaS *SaaS
}

// NewServer builds the gateway's top-level http.Handler (an
// *amaro.App, which satisfies http.Handler via ServeHTTP — the
// composition root in main.go hands it to a plain http.Server exactly
// as it would any other http.Handler):
//
//   - /healthz, /readyz — plain HTTP probes, registered with no
//     route-specific middleware so a kubelet probe never has to pass
//     an auth check or count against a rate limit meant for real
//     traffic.
//   - the generated Connect handler for the gateway's own minimal
//     control-plane RPC (see internal/gen and
//     proto/gateway/v1/gateway.proto), mounted via amaro.App.Mount.
//   - when cfg.SaaS is set: the SaaS Connect services (auth, workspace,
//     feature, billing) and POST /webhooks/stripe, registered before the
//     catch-all so they always win over it.
//   - everything else — the reverse-proxy catch-all, gated by
//     Amaro's own rate-limit and key-auth middleware, and delegating
//     the actual routing decision to Router.Match and the actual
//     byte-moving to Forwarder.Forward.
//
// Structured request logging and panic recovery wrap the entire app
// (global middleware, via app.Use), including the probes and the
// Connect surface, so every request produces one log line and no
// single request can take the process down.
func NewServer(cfg ServerConfig) *amaro.App {
	app := amaro.New(amaro.WithRouter(routers.NewTrieRouter()))

	// Structured request logging: reuse Amaro's own Logger middleware,
	// but override its print function to emit through this service's
	// slog logger (JSON) instead of Amaro's default ANSI-colored
	// *log.Logger output, so every log line this service emits — this
	// one included — is structured the same way.
	app.Use(middlewares.Logger(middlewares.WithLoggerLogFunc(
		func(_ *stdlog.Logger, duration time.Duration, c *amaro.Context, statusCode int) {
			cfg.Logger.Info("request",
				"method", c.Request.Method,
				"path", c.Request.URL.Path,
				"status", statusCode,
				"duration_ms", duration.Milliseconds(),
				"remote", c.Request.RemoteAddr,
			)
		},
	)))
	app.Use(middleware.Recover(cfg.Logger))

	app.GET("/healthz", healthzHandler)
	var ready func(context.Context) error
	if cfg.SaaS != nil {
		ready = cfg.SaaS.Ready
	}
	app.GET("/readyz", readyzHandler(cfg.Router, ready))

	connectPath, connectHandler := gatewayv1connect.NewGatewayServiceHandler(newGatewayService())
	app.Mount(connectPath, connectHandler)

	if cfg.SaaS != nil {
		mountSaaS(app, cfg.SaaS)
	}

	// The reverse-proxy catch-all. Registered the same way
	// amaro.App.Mount registers a mounted http.Handler internally
	// (exact match on "/" plus a "/*filepath" wildcard for everything
	// under it), but through app.Any directly instead of Mount, since
	// Mount takes no per-route middleware and this route is the one
	// place rate-limiting and auth actually apply.
	var proxyMiddlewares []amaro.Middleware
	if cfg.RateLimitRPS > 0 {
		proxyMiddlewares = append(proxyMiddlewares, middlewares.RateLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst))
	}
	if cfg.AuthToken != "" {
		authConfig := middlewares.DefaultKeyAuthConfig()
		authConfig.KeyLookup = "header:Authorization"
		authConfig.AuthScheme = "Bearer"
		authConfig.Validator = bearerTokenValidator(cfg.AuthToken)
		proxyMiddlewares = append(proxyMiddlewares, middlewares.KeyAuthWithConfig(authConfig))
	}
	app.Any("/", proxyHandler(cfg.Router, cfg.Forwarder), proxyMiddlewares...)
	app.Any("/*filepath", proxyHandler(cfg.Router, cfg.Forwarder), proxyMiddlewares...)

	return app
}

// proxyHandler delegates the routing decision to Router.Match and the
// actual byte-moving to Forwarder.Forward; it owns neither.
func proxyHandler(router *core.Router, forwarder core.Forwarder) amaro.Handler {
	return func(c *amaro.Context) error {
		route, ok := router.Match(c.Request.URL.Path)
		if !ok {
			return amaro.NewHTTPError(http.StatusNotFound, "no upstream route for "+c.Request.URL.Path)
		}
		forwarder.Forward(c.Writer, c.Request, route.Upstream)
		return nil
	}
}

// bearerTokenValidator adapts a single shared token — the same "no
// token-issuance scheme yet, validate locally at the edge" design this
// gateway used before adopting Amaro (see scaling-and-infra.md's
// AuthN/authZ section) — to middlewares.KeyAuthWithConfig's validator
// shape. NewServer only registers KeyAuth at all when AuthToken is
// non-empty, so an empty token still means "no auth", not "reject
// everything".
func bearerTokenValidator(token string) func(key string, c *amaro.Context) (bool, error) {
	return func(key string, _ *amaro.Context) (bool, error) {
		return key == token, nil
	}
}
