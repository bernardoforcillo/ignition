package httpapi

import (
	"net/http"
	"net/netip"
	"testing"
)

func prefixes(t *testing.T, cidrs ...string) []netip.Prefix {
	t.Helper()
	out := make([]netip.Prefix, len(cidrs))
	for i, c := range cidrs {
		out[i] = netip.MustParsePrefix(c)
	}
	return out
}

func TestResolve(t *testing.T) {
	trusted := prefixes(t, "10.0.0.0/8", "fd00::/8")
	tests := []struct {
		name    string
		trusted []netip.Prefix
		peer    string
		xff     []string
		want    string
	}{
		{"no trusted proxies: peer wins, header ignored", nil, "203.0.113.9:4000", []string{"198.51.100.1"}, "203.0.113.9"},
		{"untrusted peer cannot spoof the header", trusted, "203.0.113.9:4000", []string{"198.51.100.1"}, "203.0.113.9"},
		{"trusted proxy: the forwarded client", trusted, "10.1.2.3:5000", []string{"198.51.100.1"}, "198.51.100.1"},
		{"chain of trusted hops is skipped from the right", trusted, "10.1.2.3:5000", []string{"198.51.100.1, 10.9.9.9"}, "198.51.100.1"},
		{"client-supplied entries to the left are ignored", trusted, "10.1.2.3:5000", []string{"1.1.1.1, 198.51.100.1"}, "198.51.100.1"},
		{"several header lines are read in order", trusted, "10.1.2.3:5000", []string{"1.1.1.1", "198.51.100.1, 10.4.4.4"}, "198.51.100.1"},
		{"no header from a trusted proxy: the proxy itself", trusted, "10.1.2.3:5000", nil, "10.1.2.3"},
		{"only trusted hops: the peer", trusted, "10.1.2.3:5000", []string{"10.5.5.5, 10.6.6.6"}, "10.1.2.3"},
		{"garbled entry stops the walk and falls back to the peer", trusted, "10.1.2.3:5000", []string{"198.51.100.1, not-an-ip"}, "10.1.2.3"},
		{"ipv6 client", trusted, "10.1.2.3:5000", []string{"2001:db8::7"}, "2001:db8::7"},
		{"ipv6 trusted proxy", trusted, "[fd00::1]:5000", []string{"198.51.100.1"}, "198.51.100.1"},
		{"ipv4-mapped peer matches an ipv4 prefix", trusted, "[::ffff:10.1.2.3]:5000", []string{"198.51.100.1"}, "198.51.100.1"},
		{"bare host peer", nil, "203.0.113.9", nil, "203.0.113.9"},
		{"unparsable peer is returned as is", trusted, "pipe", []string{"198.51.100.1"}, "pipe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.Header{}
			for _, line := range tt.xff {
				h.Add("X-Forwarded-For", line)
			}

			if got := NewClientIPResolver(tt.trusted).Resolve(tt.peer, h); got != tt.want {
				t.Errorf("Resolve(%q, %v) = %q, want %q", tt.peer, tt.xff, got, tt.want)
			}
		})
	}
}

func TestResolve_NilResolverTrustsNothing(t *testing.T) {
	var r *ClientIPResolver
	h := http.Header{"X-Forwarded-For": {"198.51.100.1"}}

	if got := r.Resolve("203.0.113.9:1", h); got != "203.0.113.9" {
		t.Errorf("got %q, want the peer", got)
	}
}
