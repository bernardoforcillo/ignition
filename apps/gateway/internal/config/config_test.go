package config

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// saasEnvKeys are every variable loadSaaS reads; tests blank them so the
// ambient environment cannot leak in.
var saasEnvKeys = []string{
	"DATABASE_URL", "AUTH_SECRET", "APP_URL", "COMPANY_NAME", "MAIL_FROM", "MAIL_REPLY_TO",
	"ASSET_BASE_URL", "RESEND_API_KEY", "ACCESS_TTL", "REFRESH_TTL",
	"STRIPE_WEBHOOK_SECRET", "STRIPE_API_KEY", "BILLING_PRICES", "BILLING_FREE_PLAN", "TRUSTED_PROXIES",
	"RESEND_BASE_URL", "STRIPE_API_BASE_URL",
}

// legacyEnv sets the minimum the proxy needs and clears every SaaS variable.
func legacyEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GATEWAY_ROUTES", "")
	t.Setenv("GATEWAY_UPSTREAM_URL", "http://localhost:1")
	for _, k := range saasEnvKeys {
		t.Setenv(k, "")
	}
}

// saasEnv is legacyEnv plus a complete, valid SaaS configuration.
func saasEnv(t *testing.T) {
	t.Helper()
	legacyEnv(t)
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/app")
	t.Setenv("AUTH_SECRET", strings.Repeat("s", 32))
	t.Setenv("APP_URL", "https://app.example.com")
	t.Setenv("COMPANY_NAME", "Acme")
	t.Setenv("MAIL_FROM", "Acme <hi@example.com>")
}

func TestLoad_SaaSIsOffWithoutDatabaseURL(t *testing.T) {
	legacyEnv(t)
	// Variables for the SaaS surface are ignored while it is off.
	t.Setenv("AUTH_SECRET", "short")
	t.Setenv("STRIPE_WEBHOOK_SECRET", "whsec")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SaaS != nil {
		t.Fatalf("SaaS = %+v, want nil", cfg.SaaS)
	}
}

func TestLoad_SaaSDefaults(t *testing.T) {
	saasEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	s := cfg.SaaS
	if s == nil {
		t.Fatal("SaaS is nil with DATABASE_URL set")
	}
	if s.AccessTTL != 15*time.Minute || s.RefreshTTL != 720*time.Hour {
		t.Errorf("ttls = %v / %v, want 15m / 720h", s.AccessTTL, s.RefreshTTL)
	}
	if s.Billing != nil {
		t.Errorf("Billing = %+v, want nil without STRIPE_WEBHOOK_SECRET", s.Billing)
	}
	if s.ResendAPIKey != "" || s.AssetBaseURL != "" || s.MailReplyTo != "" {
		t.Errorf("optional fields should default empty: %+v", s)
	}
}

func TestLoad_SaaSRequiredVariables(t *testing.T) {
	tests := []struct {
		missing string
		value   string
	}{
		{"AUTH_SECRET", ""},
		{"AUTH_SECRET", strings.Repeat("s", 31)},
		{"APP_URL", ""},
		{"COMPANY_NAME", ""},
		{"MAIL_FROM", ""},
	}
	for _, tc := range tests {
		t.Run(tc.missing+"="+tc.value, func(t *testing.T) {
			saasEnv(t)
			t.Setenv(tc.missing, tc.value)
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), tc.missing) {
				t.Fatalf("err = %v, want one naming %s", err, tc.missing)
			}
		})
	}
}

func TestLoad_SaaSTTLOverridesAndValidation(t *testing.T) {
	saasEnv(t)
	t.Setenv("ACCESS_TTL", "5m")
	t.Setenv("REFRESH_TTL", "24h")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SaaS.AccessTTL != 5*time.Minute || cfg.SaaS.RefreshTTL != 24*time.Hour {
		t.Errorf("ttls = %v / %v", cfg.SaaS.AccessTTL, cfg.SaaS.RefreshTTL)
	}

	for _, bad := range []string{"soon", "0s", "-1m"} {
		t.Setenv("ACCESS_TTL", bad)
		if _, err := Load(); err == nil {
			t.Errorf("ACCESS_TTL=%q accepted", bad)
		}
	}
}

func TestLoad_BillingEnabledOnlyWithWebhookSecret(t *testing.T) {
	saasEnv(t)
	t.Setenv("STRIPE_API_KEY", "sk_test")
	t.Setenv("BILLING_PRICES", "price_1=plan:pro")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SaaS.Billing != nil {
		t.Fatal("billing enabled without STRIPE_WEBHOOK_SECRET")
	}

	t.Setenv("STRIPE_WEBHOOK_SECRET", "whsec_1")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	b := cfg.SaaS.Billing
	if b == nil || b.StripeAPIKey != "sk_test" || b.StripeWebhookSecret != "whsec_1" || b.FreePlan != "" {
		t.Fatalf("Billing = %+v", b)
	}
}

func TestLoad_TestOnlyProviderBaseURLs(t *testing.T) {
	saasEnv(t)
	t.Setenv("STRIPE_WEBHOOK_SECRET", "whsec_1")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SaaS.ResendBaseURL != "" || cfg.SaaS.Billing.StripeAPIBaseURL != "" {
		t.Fatalf("base URLs must default to empty (the real APIs): %+v", cfg.SaaS)
	}

	t.Setenv("RESEND_BASE_URL", "http://127.0.0.1:9001")
	t.Setenv("STRIPE_API_BASE_URL", "http://127.0.0.1:9002")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SaaS.ResendBaseURL != "http://127.0.0.1:9001" || cfg.SaaS.Billing.StripeAPIBaseURL != "http://127.0.0.1:9002" {
		t.Fatalf("base URLs = %q / %q", cfg.SaaS.ResendBaseURL, cfg.SaaS.Billing.StripeAPIBaseURL)
	}
}

func TestLoad_BillingFreePlanOverride(t *testing.T) {
	saasEnv(t)
	t.Setenv("STRIPE_WEBHOOK_SECRET", "whsec_1")
	t.Setenv("BILLING_FREE_PLAN", "starter")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SaaS.Billing.FreePlan != "starter" {
		t.Fatalf("FreePlan = %q", cfg.SaaS.Billing.FreePlan)
	}
}

func TestParsePrices(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    []PriceSpec
		wantErr bool
	}{
		{"empty is no prices", "", nil, false},
		{"plan and add-on", "price_123=plan:pro,price_456=addon:extra-api-calls", []PriceSpec{
			{"price_123", "plan", "pro"}, {"price_456", "addon", "extra-api-calls"},
		}, false},
		{"whitespace and trailing comma tolerated", " price_1 = plan:pro , ", []PriceSpec{{"price_1", "plan", "pro"}}, false},
		{"missing kind", "price_1=pro", nil, true},
		{"unknown kind", "price_1=bundle:pro", nil, true},
		{"missing id", "=plan:pro", nil, true},
		{"missing target name", "price_1=plan:", nil, true},
		{"no equals", "price_1", nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePrices(tc.raw)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestLoad_InvalidPricesFailStartup(t *testing.T) {
	saasEnv(t)
	t.Setenv("STRIPE_WEBHOOK_SECRET", "whsec_1")
	t.Setenv("BILLING_PRICES", "nonsense")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "BILLING_PRICES") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoad_LegacyProxyConfigStillRequiresRoutes(t *testing.T) {
	legacyEnv(t)
	t.Setenv("GATEWAY_UPSTREAM_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("Load succeeded with no routes")
	}
}

func TestLoad_TrustedProxies(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    []string
		wantErr string
	}{
		{"unset trusts nothing", "", nil, ""},
		{"cidrs and bare addresses", "10.0.0.0/8, 192.168.1.5 ,fd00::/8", []string{"10.0.0.0/8", "192.168.1.5/32", "fd00::/8"}, ""},
		{"host bits are masked", "10.1.2.3/8", []string{"10.0.0.0/8"}, ""},
		{"empty entries are skipped", "10.0.0.0/8,,", []string{"10.0.0.0/8"}, ""},
		{"garbage is a startup error", "10.0.0.0/8,proxy.internal", nil, "proxy.internal"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			legacyEnv(t)
			t.Setenv("TRUSTED_PROXIES", tc.raw)

			cfg, err := Load()

			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want one mentioning %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, p := range cfg.TrustedProxies {
				got = append(got, p.String())
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("trusted proxies = %v, want %v", got, tc.want)
			}
		})
	}
}
