package httpapi

import (
	"context"
	"errors"
	"strings"
	"time"

	"connectrpc.com/connect"

	"github.com/bernardoforcillo/ignition/go-packages/billing"
	"github.com/bernardoforcillo/ignition/go-packages/identity/permissions"
	"github.com/bernardoforcillo/ignition/go-packages/identity/workspace"

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
	members workspaceMembership
	appURL  string
}

// GetSubscription needs only membership: every member may see the plan, only those allowed to
// update the workspace (and only once a provider customer exists) get can_manage.
func (h *billingHandler) GetSubscription(ctx context.Context, req *connect.Request[saasv1.GetSubscriptionRequest]) (*connect.Response[saasv1.GetSubscriptionResponse], error) {
	user, err := subjectOrErr(ctx)
	if err != nil {
		return nil, err
	}
	wsID := req.Msg.GetWorkspaceId()
	if wsID == "" {
		return nil, requiredField("workspace_id")
	}
	if _, err := h.members.Get(ctx, user, wsID); err != nil {
		return nil, toConnectError(ctx, err)
	}
	info, err := h.billing.Subscription(ctx, wsID)
	if err != nil {
		return nil, toConnectError(ctx, err)
	}
	canManage := false
	if info.HasCustomer {
		switch err := h.authz.Authorize(ctx, user, wsID, permissions.ResourceWorkspace, permissions.ActionUpdate); {
		case err == nil:
			canManage = true
		case !errors.Is(err, workspace.ErrForbidden):
			return nil, toConnectError(ctx, err)
		}
	}
	resp := &saasv1.GetSubscriptionResponse{
		PlanId: info.PlanID, AddOnIds: info.AddOnIDs, Status: info.Status, CanManage: canManage,
	}
	if !info.CurrentPeriodEnd.IsZero() {
		resp.CurrentPeriodEnd = info.CurrentPeriodEnd.UTC().Format(time.RFC3339)
	}
	return connect.NewResponse(resp), nil
}

// ListPrices is the catalog from configuration; it holds nothing workspace-specific.
func (h *billingHandler) ListPrices(_ context.Context, _ *connect.Request[saasv1.ListPricesRequest]) (*connect.Response[saasv1.ListPricesResponse], error) {
	prices := h.billing.Prices()
	out := make([]*saasv1.Price, len(prices))
	for i, p := range prices {
		out[i] = &saasv1.Price{PriceId: p.PriceID, Kind: p.Kind, Id: p.ID}
	}
	return connect.NewResponse(&saasv1.ListPricesResponse{Prices: out}), nil
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
