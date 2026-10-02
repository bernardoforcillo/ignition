// Command gateway is the composition root: the one place in this
// service allowed to know about concrete adapters (internal/adapter/*)
// as well as the domain (internal/core) and shared foundations
// (internal/config). Every other package only ever depends on an
// interface one layer in from itself — see internal/core/forwarder.go
// and internal/adapter/httpapi/server.go for where that's enforced.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"syscall"

	"github.com/bernardoforcillo/ignition/go-packages/database"

	"github.com/bernardoforcillo/ignition/apps/gateway/internal/adapter/httpapi"
	"github.com/bernardoforcillo/ignition/apps/gateway/internal/adapter/proxy"
	"github.com/bernardoforcillo/ignition/apps/gateway/internal/adapter/saas"
	"github.com/bernardoforcillo/ignition/apps/gateway/internal/config"
	"github.com/bernardoforcillo/ignition/apps/gateway/internal/core"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(logger); err != nil {
		logger.Error("gateway exited with error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Translate config's plain RouteSpec values into the domain's
	// Route type. This conversion belongs here, not in package config,
	// because config is the shared-foundations layer (beneath domain)
	// and must not import core — see internal/config/config.go.
	domainRoutes := make([]core.Route, len(cfg.Routes))
	for i, rs := range cfg.Routes {
		domainRoutes[i] = core.Route{PathPrefix: rs.PathPrefix, Upstream: rs.Upstream}
	}
	router := core.NewRouter(domainRoutes)

	// The only place a concrete adapter gets constructed and handed to
	// the transport layer as its interface type (core.Forwarder).
	forwarder := proxy.New()

	// The SaaS surface (accounts, workspaces, features, billing) exists
	// only when DATABASE_URL is set; otherwise the gateway is the pure
	// proxy it always was.
	var saasAPI *httpapi.SaaS
	if cfg.SaaS != nil {
		services, err := buildSaaS(context.Background(), *cfg.SaaS, logger)
		if err != nil {
			return err
		}
		defer func() { _ = services.Close() }()
		saasAPI = newSaaSAPI(services, cfg.SaaS.AppURL, cfg.TrustedProxies)
	}

	handler := httpapi.NewServer(httpapi.ServerConfig{
		Router:         router,
		Forwarder:      forwarder,
		AuthToken:      cfg.AuthToken,
		RateLimitRPS:   cfg.RateLimitRPS,
		RateLimitBurst: cfg.RateLimitBurst,
		Logger:         logger,
		SaaS:           saasAPI,
	})

	// Connect RPC needs HTTP/2 to stream, and this service has no TLS
	// termination of its own (that's expected to sit in front of it —
	// an ingress, a load balancer). Go 1.24 added native support for
	// cleartext ("unencrypted") HTTP/2 directly on http.Server via
	// Protocols, so no golang.org/x/net/http2/h2c wrapper is needed
	// here; HTTP/1.1 stays enabled alongside it for /healthz, /readyz,
	// and any plain HTTP client.
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	httpServer := &http.Server{
		Addr:      cfg.ListenAddr,
		Handler:   handler,
		Protocols: protocols,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("gateway listening", "addr", cfg.ListenAddr, "routes", len(domainRoutes))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-serveErr:
		if err != nil {
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return err
	}
	logger.Info("gateway stopped")
	return nil
}

// buildSaaS opens the database and wires the SaaS services over it,
// running their migrations. The returned services own the database.
func buildSaaS(ctx context.Context, cfg config.SaaS, logger *slog.Logger) (*saas.Services, error) {
	db, err := database.Open(ctx, database.Config{DSN: cfg.DatabaseURL})
	if err != nil {
		return nil, err
	}
	services, err := saas.Build(ctx, cfg, db, logger)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return services, nil
}

// newSaaSAPI hands the built services to the transport as the interfaces
// it declares. Billing is left nil (not a typed-nil interface) when it is
// not configured, which is how the transport knows to skip its routes.
func newSaaSAPI(s *saas.Services, appURL string, trustedProxies []netip.Prefix) *httpapi.SaaS {
	api := &httpapi.SaaS{
		Auth:       s.Auth,
		Tokens:     s.Auth,
		Account:    s.Account,
		Workspaces: s.Workspaces,
		Features:   s.Features,
		Directory:  s,
		Clients:    httpapi.NewClientIPResolver(trustedProxies),
		AppURL:     appURL,
		Ready:      s.Ready,
	}
	if s.Billing != nil {
		api.Billing = s.Billing
		api.BillingWebhook = s.BillingWebhook
	}
	return api
}
