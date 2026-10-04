package httpapi

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	"github.com/bernardoforcillo/ignition/go-packages/identity/auth"

	saasv1 "github.com/bernardoforcillo/ignition/go-packages/proto/gen/saas/v1"
)

// authHandler implements saasv1connect.AuthServiceHandler: validate,
// delegate to the identity service, map errors. SignUp returns the same
// empty success whether or not the address was already registered.
type authHandler struct {
	auth    authService
	clients *ClientIPResolver
}

func (h *authHandler) SignUp(ctx context.Context, req *connect.Request[saasv1.SignUpRequest]) (*connect.Response[saasv1.SignUpResponse], error) {
	if req.Msg.GetEmail() == "" {
		return nil, requiredField("email")
	}
	if req.Msg.GetPassword() == "" {
		return nil, requiredField("password")
	}
	if err := h.auth.SignUp(ctx, req.Msg.GetEmail(), req.Msg.GetPassword(), h.clients.Resolve(req.Peer().Addr, req.Header())); err != nil {
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
	t, err := h.auth.Login(ctx, req.Msg.GetEmail(), req.Msg.GetPassword(), h.clients.Resolve(req.Peer().Addr, req.Header()), req.Header().Get("User-Agent"))
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

// RequestPasswordReset answers with the same empty success whether or not the address has an
// account; only the per-IP rate limit can make it fail, and that depends on the caller alone.
func (h *authHandler) RequestPasswordReset(ctx context.Context, req *connect.Request[saasv1.RequestPasswordResetRequest]) (*connect.Response[saasv1.RequestPasswordResetResponse], error) {
	if req.Msg.GetEmail() == "" {
		return nil, requiredField("email")
	}
	if err := h.auth.RequestPasswordReset(ctx, req.Msg.GetEmail(), h.clients.Resolve(req.Peer().Addr, req.Header())); err != nil {
		return nil, toConnectError(ctx, err)
	}
	return connect.NewResponse(&saasv1.RequestPasswordResetResponse{}), nil
}

func (h *authHandler) ResetPassword(ctx context.Context, req *connect.Request[saasv1.ResetPasswordRequest]) (*connect.Response[saasv1.ResetPasswordResponse], error) {
	if req.Msg.GetToken() == "" {
		return nil, requiredField("token")
	}
	if req.Msg.GetNewPassword() == "" {
		return nil, requiredField("new_password")
	}
	if err := h.auth.ResetPassword(ctx, req.Msg.GetToken(), req.Msg.GetNewPassword()); err != nil {
		return nil, toConnectError(ctx, err)
	}
	return connect.NewResponse(&saasv1.ResetPasswordResponse{}), nil
}

func userMessage(u auth.User) *saasv1.User { return &saasv1.User{Id: u.ID, Email: u.Email} }

// ListAuthProviders reports the external sign-in providers configured here. None until the OAuth
// wiring lands, which is also the correct answer for a deployment that never configures one.
func (h *authHandler) ListAuthProviders(_ context.Context, _ *connect.Request[saasv1.ListAuthProvidersRequest]) (*connect.Response[saasv1.ListAuthProvidersResponse], error) {
	return connect.NewResponse(&saasv1.ListAuthProvidersResponse{}), nil
}

// StartOAuth is not implemented until external sign-in is configured.
func (h *authHandler) StartOAuth(context.Context, *connect.Request[saasv1.StartOAuthRequest]) (*connect.Response[saasv1.StartOAuthResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("external sign-in is not configured"))
}

// ExchangeOAuthCode is not implemented until external sign-in is configured.
func (h *authHandler) ExchangeOAuthCode(context.Context, *connect.Request[saasv1.ExchangeOAuthCodeRequest]) (*connect.Response[saasv1.ExchangeOAuthCodeResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("external sign-in is not configured"))
}
