// Package middleware holds the one piece of the gateway's
// transport-layer cross-cutting behavior that the Amaro framework
// doesn't already ship in a form that fits this service: panic
// recovery that logs through the same structured slog logger every
// other line in this service uses. Amaro's own amaro.Recovery() is
// still enabled by default underneath this one (see amaro.New() in
// server.go) as a last-resort safety net; it only fires if this
// middleware itself panics.
//
// Everything else that used to live here — request logging,
// authentication, rate limiting — now comes directly from
// github.com/buildwithgo/amaro and github.com/buildwithgo/amaro/middlewares,
// wired up in server.go, rather than being hand-rolled. Per
// code-organization.md, none of it owns a business decision; that
// stays in internal/core.
package middleware

import (
	"log/slog"

	"github.com/buildwithgo/amaro"
)

// Recover catches panics from any handler further down the chain,
// logs them as a structured line, and responds with 500 instead of
// taking the whole process down. A single malformed upstream response
// or a handler bug should not crash a shared gateway process that has
// every other in-flight request routed through it.
func Recover(logger *slog.Logger) amaro.Middleware {
	return func(next amaro.Handler) amaro.Handler {
		return func(c *amaro.Context) (err error) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic recovered",
						"panic", rec,
						"method", c.Request.Method,
						"path", c.Request.URL.Path,
					)
					err = amaro.NewHTTPError(500, "internal server error")
				}
			}()
			return next(c)
		}
	}
}
