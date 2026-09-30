package core

import (
	"net/http"
	"net/url"
)

// Forwarder is the port the domain depends on to actually move bytes
// to and from a matched upstream target. Route/Router above decide
// *where* a request should go; Forwarder is what performs the
// mechanical work of getting it there. The concrete implementation
// lives in internal/adapter/proxy and imports this package — never the
// reverse — per the dependency rule in code-organization.md: domain
// defines the interface, the adapter implements it.
type Forwarder interface {
	Forward(w http.ResponseWriter, r *http.Request, target *url.URL)
}
