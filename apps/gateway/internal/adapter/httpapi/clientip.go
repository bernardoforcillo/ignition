package httpapi

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ClientIPResolver finds the address of the real client behind reverse proxies.
//
// X-Forwarded-For is client-controlled until a proxy we trust has appended to it, so it is only
// read when the TCP peer itself is a trusted proxy, and then walked from the right: every hop that
// is a trusted proxy is skipped, and the first address that is not one is the client. Entries to
// the left of it were supplied by the client and are never used. With no trusted proxies configured
// the peer address is always the answer, which is the safe default for a gateway that is exposed
// directly.
type ClientIPResolver struct{ trusted []netip.Prefix }

// NewClientIPResolver builds a resolver that trusts the given proxy networks.
func NewClientIPResolver(trusted []netip.Prefix) *ClientIPResolver {
	return &ClientIPResolver{trusted: trusted}
}

// Resolve returns the client IP for a request whose TCP peer is peerAddr (host:port or a bare
// host) and whose headers are h. It never returns an empty string for a parsable peer.
func (r *ClientIPResolver) Resolve(peerAddr string, h http.Header) string {
	peer := parseAddr(peerAddr)
	if !peer.IsValid() {
		return peerAddr
	}
	if r == nil || !r.isTrusted(peer) {
		return peer.String()
	}

	hops := forwardedFor(h)
	for i := len(hops) - 1; i >= 0; i-- {
		hop := parseAddr(hops[i])
		if !hop.IsValid() {
			// A malformed entry means the chain cannot be trusted past this point.
			break
		}
		if !r.isTrusted(hop) {
			return hop.String()
		}
	}
	// Every hop was a trusted proxy (or the header was absent or garbled): the peer is the best
	// address we can vouch for.
	return peer.String()
}

func (r *ClientIPResolver) isTrusted(a netip.Addr) bool {
	for _, p := range r.trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// forwardedFor flattens every X-Forwarded-For header line into its comma-separated entries, in
// the order the proxies appended them.
func forwardedFor(h http.Header) []string {
	var hops []string
	for _, line := range h.Values("X-Forwarded-For") {
		for _, part := range strings.Split(line, ",") {
			if part = strings.TrimSpace(part); part != "" {
				hops = append(hops, part)
			}
		}
	}
	return hops
}

// parseAddr accepts "host", "host:port", "[v6]:port" and IPv4-mapped IPv6, and returns an invalid
// Addr for anything else (a hostname, "unknown", garbage).
func parseAddr(s string) netip.Addr {
	s = strings.TrimSpace(s)
	if host, _, err := net.SplitHostPort(s); err == nil {
		s = host
	}
	s = strings.TrimSuffix(strings.TrimPrefix(s, "["), "]")
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}
	}
	return a.WithZone("").Unmap()
}
