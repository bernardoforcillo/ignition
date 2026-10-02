package httpapi

import (
	"context"
	"net"

	"connectrpc.com/connect"

	"github.com/bernardoforcillo/ignition/go-packages/identity/auth"

	saasv1 "github.com/bernardoforcillo/ignition/apps/gateway/internal/gen/saas/v1"
)

// authHandler implements saasv1connect.AuthServiceHandler: validate,
// delegate to the identity service, map errors. SignUp returns the same
// empty success whether or not the address was already registered.
type authHandler struct{ auth authService }

func (h *authHandler) SignUp(ctx context.Context, req *connect.Request[saasv1.SignUpRequest]) (*connect.Response[saasv1.SignUpResponse], error) {
	if req.Msg.GetEmail() == "" {
		return nil, requiredField("email")
	}
	if req.Msg.GetPassword() == "" {
		return nil, requiredField("password")
	}
	if err := h.auth.SignUp(ctx, req.Msg.GetEmail(), req.Msg.GetPassword(), clientIP(req.Peer().Addr)); err != nil {
		return nil, toConnectError(ctx, err)
	}
	return connect.NewResponse(&saasv1.SignUpResponse{}), nil
}

func (h *authHandler) VerifyEmail(ctx context.Context, req *connect.Request[saasv1.VerifyEmailRequest]) (*connect.Response[saasv1.VerifyEmailResponse], error) {
	if req.Msg.GetToken() == "" {
		return nil, requiredField("token")
	}
	if err := h.auth.VerifyEmail(ctx, req.Msg.GetToken()); err != nil {
		return nil, toConnectError(ctx, err)
	}
	return connect.NewResponse(&saasv1.VerifyEmailResponse{}), nil
}

func (h *authHandler) Login(ctx context.Context, req *connect.Request[saasv1.LoginRequest]) (*connect.Response[saasv1.LoginResponse], error) {
	if req.Msg.GetEmail() == "" {
		return nil, requiredField("email")
	}
	if req.Msg.GetPassword() == "" {
		return nil, requiredField("password")
	}
	t, err := h.auth.Login(ctx, req.Msg.GetEmail(), req.Msg.GetPassword(), clientIP(req.Peer().Addr), req.Header().Get("User-Agent"))
	if err != nil {
		return nil, toConnectError(ctx, err)
	}
	return connect.NewResponse(&saasv1.LoginResponse{
		User: userMessage(t.User), AccessToken: t.AccessToken, RefreshToken: t.RefreshToken,
	}), nil
}

func (h *authHandler) Refresh(ctx context.Context, req *connect.Request[saasv1.RefreshRequest]) (*connect.Response[saasv1.RefreshResponse], error) {
	if req.Msg.GetRefreshToken() == "" {
		return nil, requiredField("refresh_token")
	}
	t, err := h.auth.Refresh(ctx, req.Msg.GetRefreshToken())
	if err != nil {
		return nil, toConnectError(ctx, err)
	}
	return connect.NewResponse(&saasv1.RefreshResponse{
		User: userMessage(t.User), AccessToken: t.AccessToken, RefreshToken: t.RefreshToken,
	}), nil
}

func (h *authHandler) Logout(ctx context.Context, req *connect.Request[saasv1.LogoutRequest]) (*connect.Response[saasv1.LogoutResponse], error) {
	if req.Msg.GetRefreshToken() == "" {
		return nil, requiredField("refresh_token")
	}
	if err := h.auth.Logout(ctx, req.Msg.GetRefreshToken()); err != nil {
		return nil, toConnectError(ctx, err)
	}
	return connect.NewResponse(&saasv1.LogoutResponse{}), nil
}

func userMessage(u auth.User) *saasv1.User { return &saasv1.User{Id: u.ID, Email: u.Email} }

// clientIP is the host part of the peer address. Behind a proxy or load
// balancer this is the proxy's address, so the per-IP rate-limit budgets
// of the identity service then apply to all clients together; resolving
// a trusted X-Forwarded-For is deliberately not done here.
func clientIP(peerAddr string) string {
	host, _, err := net.SplitHostPort(peerAddr)
	if err != nil {
		return peerAddr
	}
	return host
}
