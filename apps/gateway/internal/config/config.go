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
	"net/netip"
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

	// TrustedProxies are the reverse-proxy networks whose X-Forwarded-For is believed. Empty
	// means the TCP peer is always the client.
	TrustedProxies []netip.Prefix

	// SaaS is nil unless DATABASE_URL is set; nil means the gateway is a
	// pure proxy (plus the Ping RPC).
	SaaS *SaaS
}

// SaaS holds the settings of the optional SaaS surface (accounts,
// workspaces, features, billing). All fields are plain values: the
// composition root and internal/adapter/saas turn them into services.
type SaaS struct {
	DatabaseURL string

	AuthSecret []byte
	AccessTTL  time.Duration
	RefreshTTL time.Duration

	AppURL       string
	AssetBaseURL string
	CompanyName  string
	MailFrom     string
	MailReplyTo  string
	// ResendAPIKey empty means "log mails instead of sending them".
	ResendAPIKey string

	// Billing is nil unless STRIPE_WEBHOOK_SECRET is set.
	Billing *Billing
}

// Billing holds the Stripe settings; it exists only when the webhook
// secret is configured.
type Billing struct {
	StripeAPIKey        string
	StripeWebhookSecret string
	// FreePlan empty means "the feature catalog's free plan"; the saas
	// adapter fills it in, since config must not know the catalog.
	FreePlan string
	Prices   []PriceSpec
}

// PriceSpec maps one provider price id to a plan or add-on.
type PriceSpec struct {
	ProviderPriceID string
	Kind            string // "plan" or "addon"
	ID              string
}

const (
	minAuthSecretLen  = 32
	defaultAccessTTL  = 15 * time.Minute
	defaultRefreshTTL = 720 * time.Hour
)

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
//
// The SaaS surface is optional and enabled only when DATABASE_URL is set;
// with it unset none of the variables below are read.
//
//	TRUSTED_PROXIES           optional; comma-separated CIDRs/IPs of reverse proxies whose
//	                          X-Forwarded-For is trusted (the per-IP auth rate limits key on it)
//	DATABASE_URL              Postgres DSN; enables the SaaS surface
//	AUTH_SECRET               required with SaaS: access-token signing key, >= 32 bytes
//	APP_URL                   required with SaaS: public web app URL (mail links, checkout return URLs)
//	COMPANY_NAME              required with SaaS: name shown in emails
//	MAIL_FROM                 required with SaaS: sender, e.g. "Ignition <hello@example.com>"
//	MAIL_REPLY_TO             optional reply-to address
//	ASSET_BASE_URL            optional origin serving email images, default APP_URL
//	RESEND_API_KEY            optional; unset logs mails (recipient+subject) instead of sending
//	ACCESS_TTL                access-token lifetime, default "15m"
//	REFRESH_TTL               refresh-token lifetime, default "720h"
//	STRIPE_WEBHOOK_SECRET     enables billing (POST /webhooks/stripe, BillingService) when set
//	STRIPE_API_KEY            Stripe key for checkout/portal calls (with billing)
//	BILLING_PRICES            "price_id=plan:<plan>,price_id=addon:<add-on>,..." price catalog
//	BILLING_FREE_PLAN         plan a lapsed workspace falls back to, default the features catalog's free plan
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

	proxies, err := parseTrustedProxies(os.Getenv("TRUSTED_PROXIES"))
	if err != nil {
		return Config{}, err
	}
	cfg.TrustedProxies = proxies

	saas, err := loadSaaS()
	if err != nil {
		return Config{}, err
	}
	cfg.SaaS = saas

	return cfg, nil
}

// parseTrustedProxies reads "10.0.0.0/8, 192.168.1.5, fd00::/8". A bare address is a single-host
// prefix. An empty value is valid and trusts no proxy.
func parseTrustedProxies(raw string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if p, err := netip.ParsePrefix(part); err == nil {
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(part)
		if err != nil {
			return nil, fmt.Errorf("config: invalid TRUSTED_PROXIES entry %q: want a CIDR or an IP", part)
		}
		out = append(out, netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen()))
	}
	return out, nil
}

// loadSaaS returns nil when DATABASE_URL is unset (SaaS disabled).
func loadSaaS() (*SaaS, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, nil
	}
	s := &SaaS{
		DatabaseURL:  dsn,
		AuthSecret:   []byte(os.Getenv("AUTH_SECRET")),
		AccessTTL:    defaultAccessTTL,
		RefreshTTL:   defaultRefreshTTL,
		AppURL:       os.Getenv("APP_URL"),
		AssetBaseURL: os.Getenv("ASSET_BASE_URL"),
		CompanyName:  os.Getenv("COMPANY_NAME"),
		MailFrom:     os.Getenv("MAIL_FROM"),
		MailReplyTo:  os.Getenv("MAIL_REPLY_TO"),
		ResendAPIKey: os.Getenv("RESEND_API_KEY"),
	}
	if len(s.AuthSecret) < minAuthSecretLen {
		return nil, fmt.Errorf("config: AUTH_SECRET must be at least %d bytes when DATABASE_URL is set", minAuthSecretLen)
	}
	for _, r := range []struct{ name, value string }{
		{"APP_URL", s.AppURL},
		{"COMPANY_NAME", s.CompanyName},
		{"MAIL_FROM", s.MailFrom},
	} {
		if r.value == "" {
			return nil, fmt.Errorf("config: %s is required when DATABASE_URL is set", r.name)
		}
	}
	for _, d := range []struct {
		name string
		dst  *time.Duration
	}{{"ACCESS_TTL", &s.AccessTTL}, {"REFRESH_TTL", &s.RefreshTTL}} {
		v := os.Getenv(d.name)
		if v == "" {
			continue
		}
		parsed, err := time.ParseDuration(v)
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("config: invalid %s %q: want a positive duration", d.name, v)
		}
		*d.dst = parsed
	}

	if secret := os.Getenv("STRIPE_WEBHOOK_SECRET"); secret != "" {
		prices, err := parsePrices(os.Getenv("BILLING_PRICES"))
		if err != nil {
			return nil, err
		}
		s.Billing = &Billing{
			StripeAPIKey:        os.Getenv("STRIPE_API_KEY"),
			StripeWebhookSecret: secret,
			FreePlan:            os.Getenv("BILLING_FREE_PLAN"),
			Prices:              prices,
		}
	}
	return s, nil
}

// parsePrices reads "price_id=plan:pro,price_id=addon:extra".
func parsePrices(raw string) ([]PriceSpec, error) {
	var prices []PriceSpec
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		id, target, ok := strings.Cut(pair, "=")
		kind, name, ok2 := strings.Cut(target, ":")
		id, kind, name = strings.TrimSpace(id), strings.TrimSpace(kind), strings.TrimSpace(name)
		if !ok || !ok2 || id == "" || name == "" || (kind != "plan" && kind != "addon") {
			return nil, fmt.Errorf("config: invalid BILLING_PRICES entry %q, want price_id=plan:<id> or price_id=addon:<id>", pair)
		}
		prices = append(prices, PriceSpec{ProviderPriceID: id, Kind: kind, ID: name})
	}
	return prices, nil
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
