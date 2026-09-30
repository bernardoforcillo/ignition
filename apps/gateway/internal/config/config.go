// Package config is the shared-foundations layer: cross-cutting,
// environment-derived settings, imported by main.go (the composition
// root) and free to be imported from more than one place per
// code-organization.md. It deliberately holds no domain type — Config
// exposes RouteSpec (a plain prefix+URL pair), not core.Route, because
// shared foundations sits beneath domain in the five-layer ordering
// and must never import upward into it; main.go does the trivial
// RouteSpec -> core.Route translation when it wires the domain router
// together.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// RouteSpec is a plain, domain-agnostic path-prefix-to-upstream pair
// as read from the environment.
type RouteSpec struct {
	PathPrefix string
	Upstream   *url.URL
}

// Config holds every environment-derived setting the gateway needs.
type Config struct {
	ListenAddr      string
	Routes          []RouteSpec
	AuthToken       string
	RateLimitRPS    float64
	RateLimitBurst  int
	ShutdownTimeout time.Duration
}

// Load builds a Config from environment variables:
//
//	GATEWAY_LISTEN_ADDR       listen address, default ":8080"
//	GATEWAY_ROUTES            "prefix=url,prefix=url,..." routing table
//	GATEWAY_UPSTREAM_URL      single catch-all upstream ("/" -> url);
//	                          used only if GATEWAY_ROUTES is unset
//	GATEWAY_AUTH_TOKEN        shared bearer token; empty disables auth
//	RATE_LIMIT_RPS            requests/sec per client IP; unset or <=0 disables
//	RATE_LIMIT_BURST          token bucket burst size, default 20
//	GATEWAY_SHUTDOWN_TIMEOUT  graceful shutdown timeout, default "10s"
func Load() (Config, error) {
	cfg := Config{
		ListenAddr:      getEnv("GATEWAY_LISTEN_ADDR", ":8080"),
		AuthToken:       os.Getenv("GATEWAY_AUTH_TOKEN"),
		ShutdownTimeout: 10 * time.Second,
		RateLimitBurst:  20,
	}

	routes, err := parseRoutes()
	if err != nil {
		return Config{}, err
	}
	if len(routes) == 0 {
		return Config{}, errors.New("config: no upstream routes configured; set GATEWAY_ROUTES or GATEWAY_UPSTREAM_URL")
	}
	cfg.Routes = routes

	if v := os.Getenv("RATE_LIMIT_RPS"); v != "" {
		rps, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return Config{}, fmt.Errorf("config: invalid RATE_LIMIT_RPS: %w", err)
		}
		cfg.RateLimitRPS = rps
	}
	if v := os.Getenv("RATE_LIMIT_BURST"); v != "" {
		burst, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("config: invalid RATE_LIMIT_BURST: %w", err)
		}
		cfg.RateLimitBurst = burst
	}
	if v := os.Getenv("GATEWAY_SHUTDOWN_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("config: invalid GATEWAY_SHUTDOWN_TIMEOUT: %w", err)
		}
		cfg.ShutdownTimeout = d
	}

	return cfg, nil
}

func parseRoutes() ([]RouteSpec, error) {
	if raw := os.Getenv("GATEWAY_ROUTES"); raw != "" {
		var routes []RouteSpec
		for _, pair := range strings.Split(raw, ",") {
			pair = strings.TrimSpace(pair)
			if pair == "" {
				continue
			}
			parts := strings.SplitN(pair, "=", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf("config: invalid GATEWAY_ROUTES entry %q, want prefix=url", pair)
			}
			u, err := url.Parse(strings.TrimSpace(parts[1]))
			if err != nil {
				return nil, fmt.Errorf("config: invalid upstream URL in GATEWAY_ROUTES entry %q: %w", pair, err)
			}
			routes = append(routes, RouteSpec{PathPrefix: strings.TrimSpace(parts[0]), Upstream: u})
		}
		return routes, nil
	}

	if raw := os.Getenv("GATEWAY_UPSTREAM_URL"); raw != "" {
		u, err := url.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("config: invalid GATEWAY_UPSTREAM_URL: %w", err)
		}
		return []RouteSpec{{PathPrefix: "/", Upstream: u}}, nil
	}

	return nil, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
