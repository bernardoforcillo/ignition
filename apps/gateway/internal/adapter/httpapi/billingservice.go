package httpapi

import (
	"context"
	"strings"

	"connectrpc.com/connect"

	"github.com/bernardoforcillo/ignition/go-packages/billing"
	"github.com/bernardoforcillo/ignition/go-packages/identity/permissions"

	saasv1 "github.com/bernardoforcillo/ignition/go-packages/proto/gen/saas/v1"
)

// Paths on the web app the payment provider returns the user to.
const (
	checkoutSuccessPath = "/billing/success"
	checkoutCancelPath  = "/billing/cancel"
	portalReturnPath    = "/billing"
)

// billingHandler implements saasv1connect.BillingServiceHandler. Only a
// member allowed to update the workspace (owner, admin) may spend its
// money; return URLs come from configuration, never from the client.
type billingHandler struct {
	billing checkoutStarter
	authz   workspaceAuthorizer
	appURL  string
}

func (h *billingHandler) StartCheckout(ctx context.Context, req *connect.Request[saasv1.StartCheckoutRequest]) (*connect.Response[saasv1.StartCheckoutResponse], error) {
	user, err := subjectOrErr(ctx)
	if err != nil {
		return nil, err
	}
	wsID := req.Msg.GetWorkspaceId()
	switch {
	case wsID == "":
		return nil, requiredField("workspace_id")
	case req.Msg.GetPriceId() == "":
		return nil, requiredField("price_id")
	}
	if err := h.authz.Authorize(ctx, user, wsID, permissions.ResourceWorkspace, permissions.ActionUpdate); err != nil {
		return nil, toConnectError(ctx, err)
	}
	url, err := h.billing.StartCheckout(ctx, billing.CheckoutRequest{
		WorkspaceID: wsID,
		PriceID:     req.Msg.GetPriceId(),
		SuccessURL:  h.appURLFor(checkoutSuccessPath),
		CancelURL:   h.appURLFor(checkoutCancelPath),
	})
	if err != nil {
		return nil, toConnectError(ctx, err)
	}
	return connect.NewResponse(&saasv1.StartCheckoutResponse{Url: url}), nil
}

func (h *billingHandler) OpenPortal(ctx context.Context, req *connect.Request[saasv1.OpenPortalRequest]) (*connect.Response[saasv1.OpenPortalResponse], error) {
	user, err := subjectOrErr(ctx)
	if err != nil {
		return nil, err
	}
	wsID := req.Msg.GetWorkspaceId()
	if wsID == "" {
		return nil, requiredField("workspace_id")
	}
	if err := h.authz.Authorize(ctx, user, wsID, permissions.ResourceWorkspace, permissions.ActionUpdate); err != nil {
		return nil, toConnectError(ctx, err)
	}
	url, err := h.billing.OpenPortal(ctx, wsID, h.appURLFor(portalReturnPath))
	if err != nil {
		return nil, toConnectError(ctx, err)
	}
	return connect.NewResponse(&saasv1.OpenPortalResponse{Url: url}), nil
}

func (h *billingHandler) appURLFor(path string) string {
	return strings.TrimRight(h.appURL, "/") + path
}
