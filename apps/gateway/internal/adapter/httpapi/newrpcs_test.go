package httpapi

import (
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/bernardoforcillo/ignition/go-packages/identity/account"
	"github.com/bernardoforcillo/ignition/go-packages/identity/auth"
	"github.com/bernardoforcillo/ignition/go-packages/identity/workspace"

	"github.com/bernardoforcillo/ignition/apps/gateway/internal/core"

	saasv1 "github.com/bernardoforcillo/ignition/go-packages/proto/gen/saas/v1"
	"github.com/bernardoforcillo/ignition/go-packages/proto/gen/saas/v1/saasv1connect"
)

func TestAuthService_RequestPasswordResetAnswersTheSameForAnyAddress(t *testing.T) {
	// The service returns nil for known and unknown addresses; the handler adds nothing.
	h := newHarness(t, false)
	c := saasv1connect.NewAuthServiceClient(h.srv.Client(), h.srv.URL)

	a, errA := c.RequestPasswordReset(t.Context(), connect.NewRequest(&saasv1.RequestPasswordResetRequest{Email: "known@x.io"}))
	b, errB := c.RequestPasswordReset(t.Context(), connect.NewRequest(&saasv1.RequestPasswordResetRequest{Email: "unknown@x.io"}))
	if errA != nil || errB != nil {
		t.Fatalf("errors: %v, %v", errA, errB)
	}
	if a.Msg.String() != b.Msg.String() {
		t.Fatalf("responses differ: %q vs %q", a.Msg, b.Msg)
	}
	if len(h.auth.resetRequests) != 2 || h.auth.clientIP == "" {
		t.Fatalf("service saw %v from %q; want both requests and the client address", h.auth.resetRequests, h.auth.clientIP)
	}
}

func TestAuthService_ResetPasswordPassesTokenAndPassword(t *testing.T) {
	h := newHarness(t, false)
	c := saasv1connect.NewAuthServiceClient(h.srv.Client(), h.srv.URL)

	if _, err := c.ResetPassword(t.Context(), connect.NewRequest(&saasv1.ResetPasswordRequest{Token: "tok", NewPassword: "New-Pass-1!"})); err != nil {
		t.Fatal(err)
	}
	if h.auth.resetWith != "tok/New-Pass-1!" {
		t.Fatalf("service saw %q", h.auth.resetWith)
	}
}

func TestAuthService_PasswordResetFailuresMapToConnectCodes(t *testing.T) {
	tests := []struct {
		name string
		call func(c saasv1connect.AuthServiceClient) error
		err  error
		want connect.Code
	}{
		{"request: rate limited", func(c saasv1connect.AuthServiceClient) error {
			_, err := c.RequestPasswordReset(t.Context(), connect.NewRequest(&saasv1.RequestPasswordResetRequest{Email: "a@b.c"}))
			return err
		}, auth.ErrRateLimited, connect.CodeResourceExhausted},
		{"request: mailer outage is internal", func(c saasv1connect.AuthServiceClient) error {
			_, err := c.RequestPasswordReset(t.Context(), connect.NewRequest(&saasv1.RequestPasswordResetRequest{Email: "a@b.c"}))
			return err
		}, errBoom, connect.CodeInternal},
		{"reset: weak password", func(c saasv1connect.AuthServiceClient) error {
			_, err := c.ResetPassword(t.Context(), connect.NewRequest(&saasv1.ResetPasswordRequest{Token: "t", NewPassword: "x"}))
			return err
		}, auth.ErrWeakPassword, connect.CodeInvalidArgument},
		{"reset: expired or used token", func(c saasv1connect.AuthServiceClient) error {
			_, err := c.ResetPassword(t.Context(), connect.NewRequest(&saasv1.ResetPasswordRequest{Token: "t", NewPassword: "x"}))
			return err
		}, auth.ErrTokenInvalid, connect.CodeUnauthenticated},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, false)
			h.auth.err = tc.err
			err := tc.call(saasv1connect.NewAuthServiceClient(h.srv.Client(), h.srv.URL))
			if connect.CodeOf(err) != tc.want {
				t.Fatalf("code = %v, want %v", connect.CodeOf(err), tc.want)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatalf("error leaks internal detail: %v", err)
			}
		})
	}
}

func TestAuthService_PasswordResetRejectsMissingFields(t *testing.T) {
	h := newHarness(t, false)
	c := saasv1connect.NewAuthServiceClient(h.srv.Client(), h.srv.URL)
	if _, err := c.RequestPasswordReset(t.Context(), connect.NewRequest(&saasv1.RequestPasswordResetRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("empty email: %v", err)
	}
	for _, req := range []*saasv1.ResetPasswordRequest{{NewPassword: "p"}, {Token: "t"}} {
		if _, err := c.ResetPassword(t.Context(), connect.NewRequest(req)); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("%v: %v", req, err)
		}
	}
	if len(h.auth.resetRequests) != 0 || h.auth.resetWith != "" {
		t.Fatal("an invalid request reached the service")
	}
}

func TestWorkspaceService_ListWorkspacesReturnsTheCallersMemberships(t *testing.T) {
	h := newHarness(t, false)
	c := saasv1connect.NewWorkspaceServiceClient(h.srv.Client(), h.srv.URL)

	resp, err := c.ListWorkspaces(t.Context(), bearer(connect.NewRequest(&saasv1.ListWorkspacesRequest{}), goodToken))
	if err != nil {
		t.Fatal(err)
	}
	got := resp.Msg.GetWorkspaces()
	if len(got) != 2 || got[0].GetWorkspace().GetId() != "ws-1" || got[0].GetRoleKey() != "owner" || got[1].GetRoleKey() != "member" {
		t.Fatalf("workspaces = %v", got)
	}
	if h.workspace.gotUser != testUser {
		t.Fatalf("listed for %q, want the token's subject", h.workspace.gotUser)
	}

	h.workspace.err = errBoom
	_, err = c.ListWorkspaces(t.Context(), bearer(connect.NewRequest(&saasv1.ListWorkspacesRequest{}), goodToken))
	if connect.CodeOf(err) != connect.CodeInternal || strings.Contains(err.Error(), "secret") {
		t.Fatalf("store failure: %v", err)
	}
}

func TestAccountService_GetMeAndExportActOnTheTokenSubject(t *testing.T) {
	h := newHarness(t, false)
	c := saasv1connect.NewAccountServiceClient(h.srv.Client(), h.srv.URL)

	me, err := c.GetMe(t.Context(), bearer(connect.NewRequest(&saasv1.GetMeRequest{}), goodToken))
	if err != nil || me.Msg.GetUser().GetId() != testUser || me.Msg.GetUser().GetEmail() != "ada@example.com" {
		t.Fatalf("GetMe = %v, %v", me, err)
	}
	exp, err := c.ExportData(t.Context(), bearer(connect.NewRequest(&saasv1.ExportDataRequest{}), goodToken))
	if err != nil || string(exp.Msg.GetData()) != `{"account":{}}` || exp.Msg.GetFilename() != "ignition-export-2026-10-02.json" {
		t.Fatalf("ExportData = %v, %v", exp, err)
	}
	if h.account.user != testUser {
		t.Fatalf("acted for %q", h.account.user)
	}
}

func TestAccountService_DeleteAccount(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want connect.Code
	}{
		{"success", nil, 0},
		{"wrong password is not an expired session", auth.ErrInvalidCredentials, connect.CodePermissionDenied},
		{"sole owner of a shared workspace", account.ErrOwnsSharedWorkspace, connect.CodeFailedPrecondition},
		{"workspace still billed", account.ErrActiveSubscription, connect.CodeFailedPrecondition},
		{"account already gone", auth.ErrAccountNotFound, connect.CodeUnauthenticated},
		{"store failure", errBoom, connect.CodeInternal},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, false)
			h.account.err = tc.err
			c := saasv1connect.NewAccountServiceClient(h.srv.Client(), h.srv.URL)

			_, err := c.DeleteAccount(t.Context(), bearer(connect.NewRequest(&saasv1.DeleteAccountRequest{Password: "pw"}), goodToken))

			if tc.err == nil {
				if err != nil || h.account.deletedWith != "pw" || h.account.user != testUser {
					t.Fatalf("err = %v, deleted %q for %q", err, h.account.deletedWith, h.account.user)
				}
				return
			}
			if connect.CodeOf(err) != tc.want || strings.Contains(err.Error(), "secret") {
				t.Fatalf("code = %v (%v), want %v", connect.CodeOf(err), err, tc.want)
			}
		})
	}
}

func TestAccountService_DeleteAccountRequiresAPassword(t *testing.T) {
	h := newHarness(t, false)
	c := saasv1connect.NewAccountServiceClient(h.srv.Client(), h.srv.URL)
	_, err := c.DeleteAccount(t.Context(), bearer(connect.NewRequest(&saasv1.DeleteAccountRequest{}), goodToken))
	if connect.CodeOf(err) != connect.CodeInvalidArgument || h.account.user != "" {
		t.Fatalf("err = %v, service reached for %q", err, h.account.user)
	}
}

func TestBillingService_GetSubscription(t *testing.T) {
	end := time.Date(2026, 11, 1, 12, 30, 0, 0, time.FixedZone("x", 3600))
	tests := []struct {
		name          string
		info          core.SubscriptionInfo
		authzErr      error
		wantManage    bool
		wantPeriodEnd string
	}{
		{"owner of a paying workspace can manage", core.SubscriptionInfo{PlanID: "pro", AddOnIDs: []string{"extra"}, Status: "active", CurrentPeriodEnd: end, HasCustomer: true}, nil, true, "2026-11-01T11:30:00Z"},
		{"member without update permission cannot manage", core.SubscriptionInfo{PlanID: "pro", Status: "active", HasCustomer: true}, workspace.ErrForbidden, false, ""},
		{"no provider customer yet: nothing to manage", core.SubscriptionInfo{PlanID: "free"}, nil, false, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, true)
			h.checkout.info = tc.info
			h.workspace.authzErr = tc.authzErr
			c := saasv1connect.NewBillingServiceClient(h.srv.Client(), h.srv.URL)

			resp, err := c.GetSubscription(t.Context(), bearer(connect.NewRequest(&saasv1.GetSubscriptionRequest{WorkspaceId: "ws-1"}), goodToken))
			if err != nil {
				t.Fatal(err)
			}
			m := resp.Msg
			if m.GetPlanId() != tc.info.PlanID || m.GetStatus() != tc.info.Status || m.GetCanManage() != tc.wantManage ||
				m.GetCurrentPeriodEnd() != tc.wantPeriodEnd || strings.Join(m.GetAddOnIds(), ",") != strings.Join(tc.info.AddOnIDs, ",") {
				t.Fatalf("response = %v", m)
			}
		})
	}
}

func TestBillingService_GetSubscriptionHidesWorkspacesOfNonMembers(t *testing.T) {
	h := newHarness(t, true)
	h.workspace.err = workspace.ErrNotMember
	h.checkout.info = core.SubscriptionInfo{PlanID: "pro"}
	c := saasv1connect.NewBillingServiceClient(h.srv.Client(), h.srv.URL)

	resp, err := c.GetSubscription(t.Context(), bearer(connect.NewRequest(&saasv1.GetSubscriptionRequest{WorkspaceId: "ws-1"}), goodToken))
	if connect.CodeOf(err) != connect.CodeNotFound || resp != nil {
		t.Fatalf("code = %v, want NotFound", connect.CodeOf(err))
	}
}

func TestBillingService_ListPricesReturnsTheCatalog(t *testing.T) {
	h := newHarness(t, true)
	c := saasv1connect.NewBillingServiceClient(h.srv.Client(), h.srv.URL)

	resp, err := c.ListPrices(t.Context(), bearer(connect.NewRequest(&saasv1.ListPricesRequest{}), goodToken))
	if err != nil {
		t.Fatal(err)
	}
	got := resp.Msg.GetPrices()
	if len(got) != 2 || got[0].GetPriceId() != "price_pro" || got[0].GetKind() != "plan" || got[1].GetKind() != "addon" || got[1].GetId() != "extra" {
		t.Fatalf("prices = %v", got)
	}
}
