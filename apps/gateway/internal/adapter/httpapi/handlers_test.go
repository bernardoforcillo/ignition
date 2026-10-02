package httpapi

import (
	"net/http"
	"net/netip"
	"strings"
	"testing"

	"connectrpc.com/connect"
	featurelayer "github.com/bernardoforcillo/featurelayer"
	"github.com/bernardoforcillo/featurelayer/entitlement"

	"github.com/bernardoforcillo/ignition/go-packages/identity/auth"
	"github.com/bernardoforcillo/ignition/go-packages/identity/workspace"

	saasv1 "github.com/bernardoforcillo/ignition/go-packages/proto/gen/saas/v1"
	"github.com/bernardoforcillo/ignition/go-packages/proto/gen/saas/v1/saasv1connect"
)

func TestAuthService_SignUpAnswersTheSameForNewAndExistingAddress(t *testing.T) {
	// The identity service returns nil in both cases; the handler must
	// add nothing that could tell them apart.
	h := newHarness(t, false)
	c := saasv1connect.NewAuthServiceClient(h.srv.Client(), h.srv.URL)

	a, errA := c.SignUp(t.Context(), connect.NewRequest(&saasv1.SignUpRequest{Email: "new@x.io", Password: "pw"}))
	b, errB := c.SignUp(t.Context(), connect.NewRequest(&saasv1.SignUpRequest{Email: "taken@x.io", Password: "pw"}))
	if errA != nil || errB != nil {
		t.Fatalf("errors: %v, %v", errA, errB)
	}
	if a.Msg.String() != b.Msg.String() {
		t.Fatalf("responses differ: %q vs %q", a.Msg, b.Msg)
	}
	if len(h.auth.signUps) != 2 {
		t.Fatalf("service saw %d sign-ups, want 2", len(h.auth.signUps))
	}
}

func TestAuthService_LoginReturnsTokensAndPassesClientContext(t *testing.T) {
	h := newHarness(t, false)
	c := saasv1connect.NewAuthServiceClient(h.srv.Client(), h.srv.URL)
	req := connect.NewRequest(&saasv1.LoginRequest{Email: "a@b.c", Password: "pw"})
	req.Header().Set("User-Agent", "test-agent")

	resp, err := c.Login(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Msg.GetAccessToken() != "acc" || resp.Msg.GetRefreshToken() != "ref" || resp.Msg.GetUser().GetId() != "u1" {
		t.Fatalf("unexpected response %v", resp.Msg)
	}
	if h.auth.userAgent != "test-agent" {
		t.Errorf("user agent = %q", h.auth.userAgent)
	}
	if h.auth.clientIP == "" || strings.Contains(h.auth.clientIP, ":") && !strings.Contains(h.auth.clientIP, "::") {
		t.Errorf("client ip %q should be a bare host", h.auth.clientIP)
	}
}

func TestAuthService_RateLimitKeyIsTheClientBehindATrustedProxy(t *testing.T) {
	tests := []struct {
		name    string
		trusted []netip.Prefix
		want    string
	}{
		// The test server is reached from loopback, standing in for the ingress controller.
		{"loopback is a trusted proxy: the forwarded client", []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("::1/128")}, "198.51.100.7"},
		{"no trusted proxies: a spoofed header is ignored", nil, "127.0.0.1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarnessWith(t, false, NewClientIPResolver(tc.trusted))
			c := saasv1connect.NewAuthServiceClient(h.srv.Client(), h.srv.URL)
			req := connect.NewRequest(&saasv1.LoginRequest{Email: "a@b.c", Password: "pw"})
			req.Header().Set("X-Forwarded-For", "198.51.100.7")

			if _, err := c.Login(t.Context(), req); err != nil {
				t.Fatal(err)
			}

			got := h.auth.clientIP
			if tc.want == "127.0.0.1" && got == "::1" {
				got = "127.0.0.1" // the listener may be IPv6 loopback on some hosts
			}
			if got != tc.want {
				t.Errorf("client ip = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAuthService_FailuresMapToConnectCodes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want connect.Code
	}{
		{"bad credentials", auth.ErrInvalidCredentials, connect.CodeUnauthenticated},
		{"rate limited", auth.ErrRateLimited, connect.CodeResourceExhausted},
		{"store failure", errBoom, connect.CodeInternal},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, false)
			h.auth.err = tc.err
			c := saasv1connect.NewAuthServiceClient(h.srv.Client(), h.srv.URL)
			_, err := c.Login(t.Context(), connect.NewRequest(&saasv1.LoginRequest{Email: "a@b.c", Password: "pw"}))
			if connect.CodeOf(err) != tc.want {
				t.Fatalf("code = %v, want %v", connect.CodeOf(err), tc.want)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatalf("error leaks internal detail: %v", err)
			}
		})
	}
}

func TestAuthService_RejectsMissingFields(t *testing.T) {
	h := newHarness(t, false)
	c := saasv1connect.NewAuthServiceClient(h.srv.Client(), h.srv.URL)
	if _, err := c.SignUp(t.Context(), connect.NewRequest(&saasv1.SignUpRequest{Email: "a@b.c"})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("SignUp without password: %v", err)
	}
	if _, err := c.Refresh(t.Context(), connect.NewRequest(&saasv1.RefreshRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("Refresh without token: %v", err)
	}
	if len(h.auth.signUps) != 0 {
		t.Error("an invalid request reached the service")
	}
}

func TestAuthService_RefreshAndLogout(t *testing.T) {
	h := newHarness(t, false)
	c := saasv1connect.NewAuthServiceClient(h.srv.Client(), h.srv.URL)

	r, err := c.Refresh(t.Context(), connect.NewRequest(&saasv1.RefreshRequest{RefreshToken: "r1"}))
	if err != nil || r.Msg.GetRefreshToken() != "r1-rotated" {
		t.Fatalf("Refresh = %v, %v", r, err)
	}
	if _, err := c.Logout(t.Context(), bearer(connect.NewRequest(&saasv1.LogoutRequest{RefreshToken: "r1"}), goodToken)); err != nil {
		t.Fatal(err)
	}
	if h.auth.loggedOutBy != "r1" {
		t.Fatalf("logout token = %q", h.auth.loggedOutBy)
	}
}

func TestWorkspaceService_HappyPaths(t *testing.T) {
	h := newHarness(t, false)
	c := saasv1connect.NewWorkspaceServiceClient(h.srv.Client(), h.srv.URL)
	ctx := t.Context()

	created, err := c.CreateWorkspace(ctx, bearer(connect.NewRequest(&saasv1.CreateWorkspaceRequest{Name: "Acme"}), goodToken))
	if err != nil || created.Msg.GetWorkspace().GetId() != "ws-1" || created.Msg.GetWorkspace().GetName() != "Acme" {
		t.Fatalf("CreateWorkspace = %v, %v", created, err)
	}
	if h.workspace.gotUser != testUser {
		t.Errorf("create ran as %q, want the token subject", h.workspace.gotUser)
	}

	got, err := c.GetWorkspace(ctx, bearer(connect.NewRequest(&saasv1.GetWorkspaceRequest{WorkspaceId: "ws-7"}), goodToken))
	if err != nil || got.Msg.GetWorkspace().GetId() != "ws-7" {
		t.Fatalf("GetWorkspace = %v, %v", got, err)
	}

	members, err := c.ListMembers(ctx, bearer(connect.NewRequest(&saasv1.ListMembersRequest{WorkspaceId: "ws-1"}), goodToken))
	if err != nil || len(members.Msg.GetMembers()) != 1 || members.Msg.GetMembers()[0].GetRoleKey() != "owner" {
		t.Fatalf("ListMembers = %v, %v", members, err)
	}

	inv, err := c.InviteMember(ctx, bearer(connect.NewRequest(&saasv1.InviteMemberRequest{WorkspaceId: "ws-1", Email: "n@x.io", RoleKey: "member"}), goodToken))
	if err != nil || inv.Msg.GetInvitation().GetEmail() != "n@x.io" || inv.Msg.GetInvitation().GetId() != "inv-1" {
		t.Fatalf("InviteMember = %v, %v", inv, err)
	}

	acc, err := c.AcceptInvite(ctx, bearer(connect.NewRequest(&saasv1.AcceptInviteRequest{Token: "tok"}), goodToken))
	if err != nil || acc.Msg.GetWorkspace().GetId() != "ws-9" {
		t.Fatalf("AcceptInvite = %v, %v", acc, err)
	}
}

func TestWorkspaceService_FailuresMapToConnectCodes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want connect.Code
	}{
		{"outsider sees not found", workspace.ErrNotMember, connect.CodeNotFound},
		{"member without permission", workspace.ErrForbidden, connect.CodePermissionDenied},
		{"slug taken", workspace.ErrSlugTaken, connect.CodeAlreadyExists},
		{"store failure", errBoom, connect.CodeInternal},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, false)
			h.workspace.err = tc.err
			c := saasv1connect.NewWorkspaceServiceClient(h.srv.Client(), h.srv.URL)
			_, err := c.InviteMember(t.Context(), bearer(connect.NewRequest(&saasv1.InviteMemberRequest{WorkspaceId: "ws-1", Email: "n@x.io", RoleKey: "member"}), goodToken))
			if connect.CodeOf(err) != tc.want {
				t.Fatalf("code = %v, want %v", connect.CodeOf(err), tc.want)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatalf("error leaks internal detail: %v", err)
			}
		})
	}
}

func TestWorkspaceService_RejectsMissingFields(t *testing.T) {
	h := newHarness(t, false)
	c := saasv1connect.NewWorkspaceServiceClient(h.srv.Client(), h.srv.URL)
	_, err := c.CreateWorkspace(t.Context(), bearer(connect.NewRequest(&saasv1.CreateWorkspaceRequest{}), goodToken))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", connect.CodeOf(err))
	}
	_, err = c.InviteMember(t.Context(), bearer(connect.NewRequest(&saasv1.InviteMemberRequest{WorkspaceId: "ws-1", Email: "n@x.io"}), goodToken))
	if connect.CodeOf(err) != connect.CodeInvalidArgument || h.workspace.inviteEmail != "" {
		t.Fatalf("invite without role: code %v, reached service %q", connect.CodeOf(err), h.workspace.inviteEmail)
	}
}

func limited(max, remaining int64) *featurelayer.UsageInfo {
	return &featurelayer.UsageInfo{Max: max, Remaining: remaining}
}

func TestFeatureService_CheckFeature(t *testing.T) {
	limit := &entitlement.Resolution{Limit: &entitlement.Limit{Max: 1000, Period: entitlement.Month}}
	tests := []struct {
		name          string
		evaluate      featurelayer.Decision
		usage         featurelayer.Decision
		usageErr      error
		wantEnabled   bool
		wantReason    string
		wantLimit     *int64
		wantRemaining *int64
		wantCode      connect.Code
	}{
		{
			name:        "boolean feature on",
			evaluate:    featurelayer.Decision{Enabled: true, Reason: featurelayer.ReasonFlagDefault},
			wantEnabled: true, wantReason: string(featurelayer.ReasonFlagDefault),
		},
		{
			name:       "not entitled is an answer, not an error",
			evaluate:   featurelayer.Decision{Reason: featurelayer.ReasonNotEntitled},
			wantReason: string(featurelayer.ReasonNotEntitled),
		},
		{
			name:        "metered feature reports the budget",
			evaluate:    featurelayer.Decision{Enabled: true, Reason: featurelayer.ReasonFlagDefault, Entitlement: limit},
			usage:       featurelayer.Decision{Enabled: true, Reason: featurelayer.ReasonFlagDefault, Usage: limited(1000, 250)},
			wantEnabled: true, wantReason: string(featurelayer.ReasonFlagDefault),
			wantLimit: ptr(int64(1000)), wantRemaining: ptr(int64(250)),
		},
		{
			name:        "unlimited meter sets no limit",
			evaluate:    featurelayer.Decision{Enabled: true, Reason: featurelayer.ReasonFlagDefault, Entitlement: limit},
			usage:       featurelayer.Decision{Enabled: true, Reason: featurelayer.ReasonFlagDefault, Usage: limited(-1, -1)},
			wantEnabled: true, wantReason: string(featurelayer.ReasonFlagDefault),
		},
		{
			name:     "usage read failure is internal",
			evaluate: featurelayer.Decision{Enabled: true, Entitlement: limit},
			usageErr: errBoom, wantCode: connect.CodeInternal,
		},
		{
			name:     "store failure while evaluating is internal",
			evaluate: featurelayer.Decision{Reason: featurelayer.ReasonStoreError, Err: errBoom},
			wantCode: connect.CodeInternal,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, false)
			h.features.evaluate, h.features.usage, h.features.usageErr = tc.evaluate, tc.usage, tc.usageErr
			c := saasv1connect.NewFeatureServiceClient(h.srv.Client(), h.srv.URL)

			resp, err := c.CheckFeature(t.Context(), bearer(connect.NewRequest(&saasv1.CheckFeatureRequest{WorkspaceId: "ws-1", FeatureKey: "api.calls"}), goodToken))
			if tc.wantCode != 0 {
				if connect.CodeOf(err) != tc.wantCode || strings.Contains(err.Error(), "secret") {
					t.Fatalf("err = %v, want code %v without detail", err, tc.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			m := resp.Msg
			if m.GetEnabled() != tc.wantEnabled || m.GetReason() != tc.wantReason {
				t.Fatalf("got enabled=%v reason=%q", m.GetEnabled(), m.GetReason())
			}
			if (tc.wantLimit == nil) != (m.Limit == nil) || (tc.wantLimit != nil && *m.Limit != *tc.wantLimit) {
				t.Errorf("limit = %v, want %v", m.Limit, tc.wantLimit)
			}
			if (tc.wantRemaining == nil) != (m.Remaining == nil) || (tc.wantRemaining != nil && *m.Remaining != *tc.wantRemaining) {
				t.Errorf("remaining = %v, want %v", m.Remaining, tc.wantRemaining)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }

func TestFeatureService_NonMemberCannotReadEntitlements(t *testing.T) {
	h := newHarness(t, false)
	h.workspace.err = workspace.ErrNotMember
	h.features.evaluate = featurelayer.Decision{Enabled: true}
	c := saasv1connect.NewFeatureServiceClient(h.srv.Client(), h.srv.URL)

	_, err := c.CheckFeature(t.Context(), bearer(connect.NewRequest(&saasv1.CheckFeatureRequest{WorkspaceId: "ws-1", FeatureKey: "api.calls"}), goodToken))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("code = %v, want NotFound", connect.CodeOf(err))
	}
}

func TestBillingService_StartCheckoutAuthorizesAndUsesConfiguredURLs(t *testing.T) {
	h := newHarness(t, true)
	c := saasv1connect.NewBillingServiceClient(h.srv.Client(), h.srv.URL)

	resp, err := c.StartCheckout(t.Context(), bearer(connect.NewRequest(&saasv1.StartCheckoutRequest{WorkspaceId: "ws-1", PriceId: "price_pro"}), goodToken))
	if err != nil || resp.Msg.GetUrl() != "https://checkout.example/session" {
		t.Fatalf("StartCheckout = %v, %v", resp, err)
	}
	if got := h.workspace.authorized; len(got) != 1 || got[0] != "organization:update" {
		t.Errorf("authorized %v, want the workspace update permission", got)
	}
	r := h.checkout.req
	if r.WorkspaceID != "ws-1" || r.PriceID != "price_pro" ||
		r.SuccessURL != "https://app.example.com/billing/success" || r.CancelURL != "https://app.example.com/billing/cancel" {
		t.Errorf("checkout request = %+v", r)
	}
}

func TestBillingService_ForbiddenMemberNeverReachesProvider(t *testing.T) {
	h := newHarness(t, true)
	h.workspace.authzErr = workspace.ErrForbidden
	c := saasv1connect.NewBillingServiceClient(h.srv.Client(), h.srv.URL)

	_, err := c.StartCheckout(t.Context(), bearer(connect.NewRequest(&saasv1.StartCheckoutRequest{WorkspaceId: "ws-1", PriceId: "p"}), goodToken))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied", connect.CodeOf(err))
	}
	if h.checkout.req.WorkspaceID != "" {
		t.Fatal("provider was called for an unauthorized caller")
	}
}

func TestBillingService_OpenPortal(t *testing.T) {
	h := newHarness(t, true)
	c := saasv1connect.NewBillingServiceClient(h.srv.Client(), h.srv.URL)

	resp, err := c.OpenPortal(t.Context(), bearer(connect.NewRequest(&saasv1.OpenPortalRequest{WorkspaceId: "ws-1"}), goodToken))
	if err != nil || resp.Msg.GetUrl() != "https://portal.example/session" {
		t.Fatalf("OpenPortal = %v, %v", resp, err)
	}
	if h.checkout.returnURL != "https://app.example.com/billing" {
		t.Errorf("return url = %q", h.checkout.returnURL)
	}

	h.checkout.err = errBoom
	_, err = c.OpenPortal(t.Context(), bearer(connect.NewRequest(&saasv1.OpenPortalRequest{WorkspaceId: "ws-1"}), goodToken))
	if connect.CodeOf(err) != connect.CodeInternal || strings.Contains(err.Error(), "secret") {
		t.Fatalf("provider failure: %v", err)
	}
}

func TestServer_SaaSRoutesWinOverProxyCatchAll(t *testing.T) {
	h := newHarness(t, true)

	resp, err := h.srv.Client().Post(h.srv.URL+"/webhooks/stripe", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || h.webhook.calls != 1 {
		t.Fatalf("webhook: status %d, handler calls %d", resp.StatusCode, h.webhook.calls)
	}

	c := saasv1connect.NewAuthServiceClient(h.srv.Client(), h.srv.URL)
	if _, err := c.Login(t.Context(), connect.NewRequest(&saasv1.LoginRequest{Email: "a@b.c", Password: "pw"})); err != nil {
		t.Fatal(err)
	}

	// An unrelated path still goes to the proxy.
	resp, err = h.srv.Client().Get(h.srv.URL + "/anything")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusTeapot || h.proxy.calls != 1 {
		t.Fatalf("proxy: status %d, calls %d (want exactly the one unrelated request)", resp.StatusCode, h.proxy.calls)
	}
}

func TestServer_BillingRoutesAbsentWhenBillingIsOff(t *testing.T) {
	h := newHarness(t, false)

	resp, err := h.srv.Client().Post(h.srv.URL+"/webhooks/stripe", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	// Falls through to the proxy catch-all rather than a billing handler.
	if resp.StatusCode != http.StatusTeapot || h.webhook.calls != 0 {
		t.Fatalf("status %d, webhook calls %d", resp.StatusCode, h.webhook.calls)
	}

	c := saasv1connect.NewBillingServiceClient(h.srv.Client(), h.srv.URL)
	_, err = c.OpenPortal(t.Context(), bearer(connect.NewRequest(&saasv1.OpenPortalRequest{WorkspaceId: "ws-1"}), goodToken))
	if err == nil {
		t.Fatal("BillingService must not be served when billing is off")
	}
}

func TestReadyz_ReportsDatabaseFailureWithoutDetail(t *testing.T) {
	h := newHarness(t, false)
	get := func() (int, string) {
		resp, err := h.srv.Client().Get(h.srv.URL + "/readyz")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		buf := make([]byte, 256)
		n, _ := resp.Body.Read(buf)
		return resp.StatusCode, string(buf[:n])
	}
	if code, _ := get(); code != http.StatusOK {
		t.Fatalf("healthy: status %d", code)
	}
	h.readyErr = errBoom
	code, body := get()
	if code != http.StatusServiceUnavailable || strings.Contains(body, "secret") {
		t.Fatalf("db down: status %d body %q", code, body)
	}
}
