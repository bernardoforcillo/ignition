package httpapi

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	"github.com/bernardoforcillo/ignition/go-packages/identity/auth"

	saasv1 "github.com/bernardoforcillo/ignition/go-packages/proto/gen/saas/v1"
)

// accountHandler implements saasv1connect.AccountServiceHandler. Every RPC acts on the
// authenticated subject only; no request carries a user id.
type accountHandler struct{ account accountService }

func (h *accountHandler) GetMe(ctx context.Context, _ *connect.Request[saasv1.GetMeRequest]) (*connect.Response[saasv1.GetMeResponse], error) {
	user, err := subjectOrErr(ctx)
	if err != nil {
		return nil, err
	}
	u, err := h.account.Me(ctx, user)
	if err != nil {
		return nil, toConnectError(ctx, err)
	}
	return connect.NewResponse(&saasv1.GetMeResponse{User: userMessage(u)}), nil
}

func (h *accountHandler) ExportData(ctx context.Context, _ *connect.Request[saasv1.ExportDataRequest]) (*connect.Response[saasv1.ExportDataResponse], error) {
	user, err := subjectOrErr(ctx)
	if err != nil {
		return nil, err
	}
	data, filename, err := h.account.Export(ctx, user)
	if err != nil {
		return nil, toConnectError(ctx, err)
	}
	return connect.NewResponse(&saasv1.ExportDataResponse{Data: data, Filename: filename}), nil
}

// DeleteAccount answers a wrong password with PermissionDenied, not the Unauthenticated the
// sign-in mapping gives it: the caller is signed in, and Unauthenticated would read to a client
// as an expired session and sign the user out instead of showing "wrong password".
func (h *accountHandler) DeleteAccount(ctx context.Context, req *connect.Request[saasv1.DeleteAccountRequest]) (*connect.Response[saasv1.DeleteAccountResponse], error) {
	user, err := subjectOrErr(ctx)
	if err != nil {
		return nil, err
	}
	if req.Msg.GetPassword() == "" {
		return nil, requiredField("password")
	}
	if err := h.account.Delete(ctx, user, req.Msg.GetPassword()); err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			return nil, connect.NewError(connect.CodePermissionDenied, errors.New("incorrect password"))
		}
		return nil, toConnectError(ctx, err)
	}
	return connect.NewResponse(&saasv1.DeleteAccountResponse{}), nil
}
