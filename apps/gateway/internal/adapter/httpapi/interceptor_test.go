package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"connectrpc.com/connect"

	saasv1 "github.com/bernardoforcillo/ignition/go-packages/proto/gen/saas/v1"
	"github.com/bernardoforcillo/ignition/go-packages/proto/gen/saas/v1/saasv1connect"
)

func bearer[T any](req *connect.Request[T], token string) *connect.Request[T] {
	req.Header().Set("Authorization", "Bearer "+token)
	return req
}

func TestAuthInterceptor_PublicProceduresNeedNoToken(t *testing.T) {
	h := newHarness(t, false)
	c := saasv1connect.NewAuthServiceClient(h.srv.Client(), h.srv.URL)
	ctx := t.Context()

	if _, err := c.SignUp(ctx, connect.NewRequest(&saasv1.SignUpRequest{Email: "a@b.c", Password: "pw"})); err != nil {
		t.Errorf("SignUp: %v", err)
	}
	if _, err := c.VerifyEmail(ctx, connect.NewRequest(&saasv1.VerifyEmailRequest{Token: "t"})); err != nil {
		t.Errorf("VerifyEmail: %v", err)
	}
	if _, err := c.Login(ctx, connect.NewRequest(&saasv1.LoginRequest{Email: "a@b.c", Password: "pw"})); err != nil {
		t.Errorf("Login: %v", err)
	}
	if _, err := c.Refresh(ctx, connect.NewRequest(&saasv1.RefreshRequest{RefreshToken: "r"})); err != nil {
		t.Errorf("Refresh: %v", err)
	}
	if _, err := c.RequestPasswordReset(ctx, connect.NewRequest(&saasv1.RequestPasswordResetRequest{Email: "a@b.c"})); err != nil {
		t.Errorf("RequestPasswordReset: %v", err)
	}
	if _, err := c.ResetPassword(ctx, connect.NewRequest(&saasv1.ResetPasswordRequest{Token: "t", NewPassword: "pw"})); err != nil {
		t.Errorf("ResetPassword: %v", err)
	}
}

func TestAuthInterceptor_ProtectedProceduresRejectMissingAndBadTokens(t *testing.T) {
	h := newHarness(t, true)
	procedures := []string{
		saasv1connect.AuthServiceLogoutProcedure,
		saasv1connect.WorkspaceServiceCreateWorkspaceProcedure,
		saasv1connect.WorkspaceServiceGetWorkspaceProcedure,
		saasv1connect.WorkspaceServiceListMembersProcedure,
		saasv1connect.WorkspaceServiceInviteMemberProcedure,
		saasv1connect.WorkspaceServiceAcceptInviteProcedure,
		saasv1connect.FeatureServiceCheckFeatureProcedure,
		saasv1connect.BillingServiceStartCheckoutProcedure,
		saasv1connect.BillingServiceOpenPortalProcedure,
		saasv1connect.BillingServiceGetSubscriptionProcedure,
		saasv1connect.BillingServiceListPricesProcedure,
		saasv1connect.WorkspaceServiceListWorkspacesProcedure,
		saasv1connect.AccountServiceGetMeProcedure,
		saasv1connect.AccountServiceExportDataProcedure,
		saasv1connect.AccountServiceDeleteAccountProcedure,
	}
	headers := map[string]string{
		"no header":      "",
		"wrong scheme":   "Basic " + goodToken,
		"empty bearer":   "Bearer ",
		"invalid bearer": "Bearer nope",
	}
	for _, proc := range procedures {
		for name, authz := range headers {
			t.Run(proc+"/"+name, func(t *testing.T) {
				req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, h.srv.URL+proc, strings.NewReader("{}"))
				req.Header.Set("Content-Type", "application/json")
				if authz != "" {
					req.Header.Set("Authorization", authz)
				}
				resp, err := h.srv.Client().Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = resp.Body.Close() }()
				if resp.StatusCode != http.StatusUnauthorized {
					t.Fatalf("status = %d, want 401 (Connect Unauthenticated)", resp.StatusCode)
				}
			})
		}
	}
	if h.workspace.gotUser != "" || h.auth.loggedOutBy != "" || h.account.user != "" || h.account.deletedWith != "" {
		t.Fatal("a rejected request reached a handler")
	}
}

func TestAuthInterceptor_ValidTokenPutsSubjectInContext(t *testing.T) {
	h := newHarness(t, false)
	c := saasv1connect.NewWorkspaceServiceClient(h.srv.Client(), h.srv.URL)

	_, err := c.GetWorkspace(t.Context(), bearer(connect.NewRequest(&saasv1.GetWorkspaceRequest{WorkspaceId: "ws-1"}), goodToken))
	if err != nil {
		t.Fatal(err)
	}
	if h.workspace.gotUser != testUser {
		t.Fatalf("handler saw user %q, want %q", h.workspace.gotUser, testUser)
	}
}

func TestAuthInterceptor_UnauthenticatedMessageIsConstant(t *testing.T) {
	h := newHarness(t, false)
	c := saasv1connect.NewWorkspaceServiceClient(h.srv.Client(), h.srv.URL)
	_, err := c.GetWorkspace(t.Context(), bearer(connect.NewRequest(&saasv1.GetWorkspaceRequest{WorkspaceId: "x"}), "nope"))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("code = %v", connect.CodeOf(err))
	}
	if !strings.Contains(err.Error(), errUnauthenticated.Error()) {
		t.Fatalf("message %q should be the constant one", err)
	}
}
