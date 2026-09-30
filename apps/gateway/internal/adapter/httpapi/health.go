package httpapi

import (
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
// routes has nothing to serve.
func readyzHandler(router *core.Router) amaro.Handler {
	return func(c *amaro.Context) error {
		if router.Len() == 0 {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "no routes configured"})
		}
		return c.String(http.StatusOK, "ready")
	}
}
