// Package proxy is a capabilities/adapter package: it wraps exactly
// one external concern (making an HTTP request to an upstream service
// over the network) behind the core.Forwarder interface that the
// domain layer defines. It imports internal/core to reference that
// interface; internal/core never imports this package back — see the
// dependency rule in code-organization.md.
package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"

	"github.com/bernardoforcillo/ignition/apps/gateway/internal/core"
)

// compile-time assertion that Proxy satisfies the domain-defined port.
var _ core.Forwarder = (*Proxy)(nil)

// Proxy forwards requests to their matched upstream target using
// net/http/httputil.ReverseProxy, caching one ReverseProxy per
// upstream so repeat requests to the same target reuse it rather than
// rebuilding it on every call.
type Proxy struct {
	mu      sync.RWMutex
	proxies map[string]*httputil.ReverseProxy
}

// New returns an empty Proxy. Reverse proxies are built lazily, on
// first use of a given target, rather than pre-warmed from the
// routing table — the routing table already lives in core.Router, and
// duplicating it here would be two sources of truth for the same
// data.
func New() *Proxy {
	return &Proxy{proxies: make(map[string]*httputil.ReverseProxy)}
}

// Forward implements core.Forwarder.
func (p *Proxy) Forward(w http.ResponseWriter, r *http.Request, target *url.URL) {
	p.mu.RLock()
	rp, ok := p.proxies[target.String()]
	p.mu.RUnlock()

	if !ok {
		rp = httputil.NewSingleHostReverseProxy(target)
		p.mu.Lock()
		p.proxies[target.String()] = rp
		p.mu.Unlock()
	}

	rp.ServeHTTP(w, r)
}
