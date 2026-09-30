package core

import (
	"net/url"
	"testing"
)

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u
}

func TestRouterMatch(t *testing.T) {
	router := NewRouter([]Route{
		{PathPrefix: "/", Upstream: mustURL(t, "http://default.internal")},
		{PathPrefix: "/api/users", Upstream: mustURL(t, "http://users.internal")},
		{PathPrefix: "/api", Upstream: mustURL(t, "http://api.internal")},
	})

	cases := []struct {
		path     string
		wantHost string
		wantOK   bool
	}{
		{"/api/users/42", "users.internal", true},
		{"/api/orders", "api.internal", true},
		{"/healthz-not-registered", "default.internal", true},
	}

	for _, tc := range cases {
		route, ok := router.Match(tc.path)
		if ok != tc.wantOK {
			t.Fatalf("Match(%q) ok = %v, want %v", tc.path, ok, tc.wantOK)
		}
		if ok && route.Upstream.Host != tc.wantHost {
			t.Errorf("Match(%q) = %q, want %q", tc.path, route.Upstream.Host, tc.wantHost)
		}
	}
}

func TestRouterMatchNoRoutes(t *testing.T) {
	router := NewRouter(nil)
	if _, ok := router.Match("/anything"); ok {
		t.Fatal("expected no match on an empty router")
	}
	if router.Len() != 0 {
		t.Fatalf("Len() = %d, want 0", router.Len())
	}
}
