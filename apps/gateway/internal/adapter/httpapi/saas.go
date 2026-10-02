package httpapi

import (
	"context"
	"net/http"

	"connectrpc.com/connect"
	featurelayer "github.com/bernardoforcillo/featurelayer"
	"github.com/bernardoforcillo/featurelayer/catalog"
	"github.com/buildwithgo/amaro"

	"github.com/bernardoforcillo/authlayer/access"

	"github.com/bernardoforcillo/ignition/go-packages/billing"
	"github.com/bernardoforcillo/ignition/go-packages/identity/auth"
	"github.com/bernardoforcillo/ignition/go-packages/identity/workspace"

	"github.com/bernardoforcillo/ignition/go-packages/proto/gen/saas/v1/saasv1connect"
)

// The interfaces below are declared here, by their consumer, and
// satisfied structurally by the identity, features and billing
// libraries (wired in internal/adapter/saas and main.go). Each handler
// takes only the slice of them it uses.

type authService interface {
	SignUp(ctx context.Context, email, password, clientIP string) error
	VerifyEmail(ctx context.Context, token string) error
	Login(ctx context.Context, email, password, clientIP, userAgent string) (auth.Tokens, error)
	Refresh(ctx context.Context, refreshToken string) (auth.Tokens, error)
	Logout(ctx context.Context, refreshToken string) error
}

type workspaceService interface {
	Create(ctx context.Context, userID, name, slug string) (workspace.Workspace, error)
	Get(ctx context.Context, userID, workspaceID string) (workspace.Workspace, error)
	ListMembers(ctx context.Context, userID, workspaceID string) ([]workspace.Member, error)
	Invite(ctx context.Context, userID, workspaceID, email, roleKey string) (workspace.Invitation, error)
	AcceptInvite(ctx context.Context, userID, token string) (workspace.Workspace, error)
}

type workspaceMembership interface {
	Get(ctx context.Context, userID, workspaceID string) (workspace.Workspace, error)
}

type workspaceAuthorizer interface {
	Authorize(ctx context.Context, userID, workspaceID, resource string, actions ...access.Action) error
}

type featureChecker interface {
	Evaluate(ctx context.Context, key catalog.Key, workspaceID, userID string) featurelayer.Decision
	Usage(ctx context.Context, key catalog.Key, workspaceID, userID string) (featurelayer.Decision, error)
}

type checkoutStarter interface {
	StartCheckout(ctx context.Context, req billing.CheckoutRequest) (string, error)
	OpenPortal(ctx context.Context, workspaceID, returnURL string) (string, error)
}

// SaaS is everything the transport needs to serve the SaaS surface. The
// composition root builds it (see internal/adapter/saas); a nil
// ServerConfig.SaaS means "no SaaS routes at all".
type SaaS struct {
	Auth       authService
	Tokens     tokenVerifier
	Workspaces interface {
		workspaceService
		workspaceAuthorizer
	}
	Features featureChecker

	// Billing and BillingWebhook are nil when billing is not configured;
	// BillingService and POST /webhooks/stripe are then not served.
	Billing        checkoutStarter
	BillingWebhook http.Handler

	// AppURL is the public web app origin; checkout and portal return
	// URLs are built from it, never taken from the client.
	AppURL string
	// Ready reports whether the SaaS dependencies (the database) are
	// reachable; /readyz fails while it errors. May be nil.
	Ready func(ctx context.Context) error
}

// stripeWebhookPath is the public path the payment provider posts to.
const stripeWebhookPath = "/webhooks/stripe"

// mountSaaS registers the Connect services and the webhook on app. It
// runs before the proxy catch-all is registered; the trie router prefers
// these static paths over the catch-all wildcard, exactly as it does for
// the Ping RPC.
func mountSaaS(app *amaro.App, s *SaaS) {
	opts := connect.WithInterceptors(authInterceptor(s.Tokens))

	mustMount(app, func() (string, http.Handler) {
		return saasv1connect.NewAuthServiceHandler(&authHandler{auth: s.Auth}, opts)
	})
	mustMount(app, func() (string, http.Handler) {
		return saasv1connect.NewWorkspaceServiceHandler(&workspaceHandler{workspaces: s.Workspaces}, opts)
	})
	mustMount(app, func() (string, http.Handler) {
		return saasv1connect.NewFeatureServiceHandler(&featureHandler{features: s.Features, members: s.Workspaces}, opts)
	})

	if s.Billing != nil {
		mustMount(app, func() (string, http.Handler) {
			return saasv1connect.NewBillingServiceHandler(
				&billingHandler{billing: s.Billing, authz: s.Workspaces, appURL: s.AppURL}, opts)
		})
	}
	if s.BillingWebhook != nil {
		if err := app.Any(stripeWebhookPath, amaro.WrapHTTPHandler(s.BillingWebhook)); err != nil {
			panic("httpapi: mounting " + stripeWebhookPath + ": " + err.Error())
		}
	}
}

func mustMount(app *amaro.App, build func() (string, http.Handler)) {
	path, h := build()
	if err := app.Mount(path, h); err != nil {
		panic("httpapi: mounting " + path + ": " + err.Error())
	}
}
