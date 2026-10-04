package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/buildwithgo/amaro"

	"github.com/bernardoforcillo/ignition/apps/gateway/internal/core"
)

// healthzHandler is a liveness probe: it answers as soon as the
// process can handle an HTTP request at all, with no dependency
// checks. Deliberately plain HTTP (not Connect) so a kubelet or load
// balancer probe never has to speak RPC framing to check liveness.
func healthzHandler(c *amaro.Context) error {
	return c.String(http.StatusOK, "ok")
}

// readyzHandler is a readiness probe: it additionally checks that the
// gateway has at least one route configured, since a gateway with no
// routes has nothing to serve, and, when ping is non-nil (the SaaS
// surface is enabled), that its database answers. The failure detail is
// never sent to the prober.
func readyzHandler(router *core.Router, ping func(context.Context) error) amaro.Handler {
	return func(c *amaro.Context) error {
		if router.Len() == 0 {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "no routes configured"})
		}
		if ping != nil {
			if err := ping(c.Request.Context()); err != nil {
				slog.ErrorContext(c.Request.Context(), "readiness check failed", "error", err)
				return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "dependency unavailable"})
			}
		}
		return c.String(http.StatusOK, "ready")
	}
}
