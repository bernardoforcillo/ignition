package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/bernardoforcillo/authlayer/access"
	featurelayer "github.com/bernardoforcillo/featurelayer"
	"github.com/bernardoforcillo/featurelayer/catalog"

	"github.com/bernardoforcillo/ignition/go-packages/billing"
	"github.com/bernardoforcillo/ignition/go-packages/identity/auth"
	"github.com/bernardoforcillo/ignition/go-packages/identity/workspace"

	"github.com/bernardoforcillo/ignition/apps/gateway/internal/core"
)

const (
	goodToken = "good-access-token"
	testUser  = "user-1"
)

// fakeVerifier accepts exactly goodToken.
type fakeVerifier struct{}

func (fakeVerifier) VerifyAccessToken(raw string) (auth.Claims, error) {
	if raw != goodToken {
		return auth.Claims{}, auth.ErrTokenInvalid
	}
	return auth.Claims{Subject: testUser}, nil
}

// fakeAuth records its inputs and returns err (or tokens) as configured.
type fakeAuth struct {
	err                              error
	signUps                          []string
	clientIP, userAgent, loggedOutBy string
}

func (f *fakeAuth) SignUp(_ context.Context, email, _, ip string) error {
	f.signUps = append(f.signUps, email)
	f.clientIP = ip
	return f.err
}
func (f *fakeAuth) VerifyEmail(context.Context, string) error { return f.err }
func (f *fakeAuth) Login(_ context.Context, email, _, ip, ua string) (auth.Tokens, error) {
	f.clientIP, f.userAgent = ip, ua
	if f.err != nil {
		return auth.Tokens{}, f.err
	}
	return auth.Tokens{User: auth.User{ID: "u1", Email: email}, AccessToken: "acc", RefreshToken: "ref"}, nil
}
func (f *fakeAuth) Refresh(_ context.Context, rt string) (auth.Tokens, error) {
	if f.err != nil {
		return auth.Tokens{}, f.err
	}
	return auth.Tokens{User: auth.User{ID: "u1", Email: "a@b.c"}, AccessToken: "acc2", RefreshToken: rt + "-rotated"}, nil
}
func (f *fakeAuth) Logout(_ context.Context, rt string) error {
	f.loggedOutBy = rt
	return f.err
}

// fakeWorkspaces implements workspaceService and workspaceAuthorizer.
type fakeWorkspaces struct {
	err      error
	authzErr error

	gotUser     string
	authorized  []string // "resource:action"
	inviteEmail string
}

func (f *fakeWorkspaces) Create(_ context.Context, user, name, slug string) (workspace.Workspace, error) {
	f.gotUser = user
	if f.err != nil {
		return workspace.Workspace{}, f.err
	}
	if slug == "" {
		slug = name
	}
	return wsOf("ws-1", name, slug), nil
}
func (f *fakeWorkspaces) Get(_ context.Context, user, id string) (workspace.Workspace, error) {
	f.gotUser = user
	if f.err != nil {
		return workspace.Workspace{}, f.err
	}
	return wsOf(id, "Acme", "acme"), nil
}
func (f *fakeWorkspaces) ListMembers(_ context.Context, _, _ string) ([]workspace.Member, error) {
	if f.err != nil {
		return nil, f.err
	}
	var m workspace.Member
	m.UserID, m.RoleKey = "u1", "owner"
	return []workspace.Member{m}, nil
}
func (f *fakeWorkspaces) Invite(_ context.Context, _, _, email, role string) (workspace.Invitation, error) {
	f.inviteEmail = email
	if f.err != nil {
		return workspace.Invitation{}, f.err
	}
	return workspace.Invitation{ID: "inv-1", Email: email, RoleKey: role}, nil
}
func (f *fakeWorkspaces) AcceptInvite(_ context.Context, _, _ string) (workspace.Workspace, error) {
	if f.err != nil {
		return workspace.Workspace{}, f.err
	}
	return wsOf("ws-9", "Joined", "joined"), nil
}
func (f *fakeWorkspaces) Authorize(_ context.Context, _, _, resource string, actions ...access.Action) error {
	for _, a := range actions {
		f.authorized = append(f.authorized, resource+":"+string(a))
	}
	return f.authzErr
}

func wsOf(id, name, slug string) workspace.Workspace {
	var w workspace.Workspace
	w.ID, w.Name, w.Slug = id, name, slug
	return w
}

// fakeFeatures returns canned decisions.
type fakeFeatures struct {
	evaluate featurelayer.Decision
	usage    featurelayer.Decision
	usageErr error
}

func (f *fakeFeatures) Evaluate(context.Context, catalog.Key, string, string) featurelayer.Decision {
	return f.evaluate
}
func (f *fakeFeatures) Usage(context.Context, catalog.Key, string, string) (featurelayer.Decision, error) {
	return f.usage, f.usageErr
}

// fakeCheckout records the requests it gets.
type fakeCheckout struct {
	err       error
	req       billing.CheckoutRequest
	returnURL string
}

func (f *fakeCheckout) StartCheckout(_ context.Context, req billing.CheckoutRequest) (string, error) {
	f.req = req
	return "https://checkout.example/session", f.err
}
func (f *fakeCheckout) OpenPortal(_ context.Context, _, returnURL string) (string, error) {
	f.returnURL = returnURL
	return "https://portal.example/session", f.err
}

// fakeForwarder fails the test if a request reaches the proxy.
type fakeForwarder struct{ calls int }

func (f *fakeForwarder) Forward(w http.ResponseWriter, _ *http.Request, _ *url.URL) {
	f.calls++
	w.WriteHeader(http.StatusTeapot)
}

// harness is a full NewServer over fakes, served by httptest.
type harness struct {
	srv       *httptest.Server
	auth      *fakeAuth
	workspace *fakeWorkspaces
	features  *fakeFeatures
	checkout  *fakeCheckout
	proxy     *fakeForwarder
	webhook   *recordingHandler
	readyErr  error
}

type recordingHandler struct{ calls int }

func (h *recordingHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	h.calls++
	w.WriteHeader(http.StatusNoContent)
}

func newHarness(t *testing.T, withBilling bool) *harness {
	t.Helper()
	h := &harness{
		auth: &fakeAuth{}, workspace: &fakeWorkspaces{}, features: &fakeFeatures{},
		checkout: &fakeCheckout{}, proxy: &fakeForwarder{}, webhook: &recordingHandler{},
	}
	upstream, _ := url.Parse("http://upstream.invalid")
	saas := &SaaS{
		Auth: h.auth, Tokens: fakeVerifier{}, Workspaces: h.workspace, Features: h.features,
		AppURL: "https://app.example.com/",
		Ready:  func(context.Context) error { return h.readyErr },
	}
	if withBilling {
		saas.Billing = h.checkout
		saas.BillingWebhook = h.webhook
	}
	app := NewServer(ServerConfig{
		Router:    core.NewRouter([]core.Route{{PathPrefix: "/", Upstream: upstream}}),
		Forwarder: h.proxy,
		Logger:    discardLogger(),
		SaaS:      saas,
	})
	h.srv = httptest.NewServer(app)
	t.Cleanup(h.srv.Close)
	return h
}

var errBoom = errors.New("store exploded: dsn=postgres://secret")
