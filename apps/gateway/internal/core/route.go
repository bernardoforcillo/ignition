// Package core is the gateway's domain layer. It owns the one real
// business decision a gateway makes — given an inbound request, which
// upstream target should handle it — and defines the port(s) that
// capability adapters must implement to carry that decision out. It
// performs no I/O and imports nothing outside the standard library, so
// it can be tested in complete isolation from HTTP, the network, or any
// concrete adapter (see route_test.go).
package core

import (
	"net/url"
	"sort"
	"strings"
)

// Route maps a URL path prefix to the upstream service that should
// handle requests under it.
type Route struct {
	// PathPrefix is matched against the start of an inbound request's
	// URL path. "/" matches everything and acts as a catch-all.
	PathPrefix string
	// Upstream is the base URL of the service that owns PathPrefix.
	Upstream *url.URL
}

// Router holds the gateway's routing table. It is the domain's central
// type: everything else in this service exists to get a request to
// Router.Match and then carry out whatever it decides.
type Router struct {
	routes []Route
}

// NewRouter builds a Router from routes, ordering them so the longest
// (most specific) PathPrefix is tried first — this is what lets a
// specific route like "/api/users" win over a catch-all "/" even
// though both would otherwise match.
func NewRouter(routes []Route) *Router {
	sorted := make([]Route, len(routes))
	copy(sorted, routes)
	sort.SliceStable(sorted, func(i, j int) bool {
		return len(sorted[i].PathPrefix) > len(sorted[j].PathPrefix)
	})
	return &Router{routes: sorted}
}

// Match returns the most specific configured route whose PathPrefix
// prefixes path, and false if no route matches.
func (r *Router) Match(path string) (Route, bool) {
	for _, route := range r.routes {
		if strings.HasPrefix(path, route.PathPrefix) {
			return route, true
		}
	}
	return Route{}, false
}

// Len reports how many routes are configured. Used by the readiness
// check: a gateway with no routes has nothing to serve and should not
// be reported ready.
func (r *Router) Len() int {
	return len(r.routes)
}
