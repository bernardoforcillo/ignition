package httpapi

import (
	"context"
	"errors"
	"log/slog"

	"connectrpc.com/connect"

	"github.com/bernardoforcillo/ignition/go-packages/billing"
	"github.com/bernardoforcillo/ignition/go-packages/features"
	"github.com/bernardoforcillo/ignition/go-packages/identity/account"
	"github.com/bernardoforcillo/ignition/go-packages/identity/auth"
	"github.com/bernardoforcillo/ignition/go-packages/identity/workspace"

	"github.com/bernardoforcillo/ignition/apps/gateway/internal/core"
)

// errInternal is the only text a client ever sees for a failure we did
// not anticipate; the detail goes to the log.
const errInternal = "internal error"

// errorMapping is one domain sentinel and the Connect status it maps to.
// The message is a constant so no underlying error text (SQL, hostnames)
// can reach the client.
type errorMapping struct {
	sentinel error
	code     connect.Code
	message  string
}

// errorMappings is ordered: the first sentinel an error matches wins.
// A not-a-member answer is "not found" so a workspace's existence is
// never revealed to outsiders.
var errorMappings = []errorMapping{
	{auth.ErrInvalidCredentials, connect.CodeUnauthenticated, "invalid credentials"},
	{auth.ErrEmailNotVerified, connect.CodeUnauthenticated, "email not verified"},
	{auth.ErrTokenInvalid, connect.CodeUnauthenticated, "token expired or invalid"},
	// A valid access token whose account was deleted since.
	{auth.ErrAccountNotFound, connect.CodeUnauthenticated, "missing or invalid access token"},
	{auth.ErrWeakPassword, connect.CodeInvalidArgument, "password does not meet requirements"},
	{auth.ErrRateLimited, connect.CodeResourceExhausted, "too many attempts; try again later"},

	{account.ErrOwnsSharedWorkspace, connect.CodeFailedPrecondition, "transfer ownership of your shared workspaces first"},
	{account.ErrActiveSubscription, connect.CodeFailedPrecondition, "cancel your workspace subscriptions first"},

	{workspace.ErrNotFound, connect.CodeNotFound, "workspace not found"},
	{workspace.ErrNotMember, connect.CodeNotFound, "workspace not found"},
	{workspace.ErrForbidden, connect.CodePermissionDenied, "insufficient permissions"},
	{workspace.ErrPrivilegeEscalation, connect.CodePermissionDenied, "insufficient permissions"},
	{workspace.ErrOwnerOnly, connect.CodePermissionDenied, "insufficient permissions"},
	{workspace.ErrRoleNotFound, connect.CodeInvalidArgument, "role not found"},
	{workspace.ErrLastOwner, connect.CodeFailedPrecondition, "cannot remove or demote the owner"},
	{workspace.ErrSlugTaken, connect.CodeAlreadyExists, "workspace slug already taken"},
	{workspace.ErrInviteInvalid, connect.CodeNotFound, "invitation invalid"},
	{workspace.ErrInviteExpired, connect.CodeFailedPrecondition, "invitation expired"},
	{workspace.ErrInvalidInput, connect.CodeInvalidArgument, "invalid input"},

	{features.ErrNotEntitled, connect.CodePermissionDenied, "feature not available on this plan"},
	{features.ErrLimitReached, connect.CodeResourceExhausted, "feature limit reached"},
	{features.ErrDisabled, connect.CodeUnavailable, "feature disabled"},

	{billing.ErrUnknownPrice, connect.CodeInvalidArgument, "unknown price"},
	{core.ErrNoBillingCustomer, connect.CodeFailedPrecondition, "no subscription to manage yet"},

	// Files. The messages are shown to the user, so they say what to do, never what the storage
	// service answered.
	{core.ErrFileNotFound, connect.CodeNotFound, "file not found"},
	{core.ErrFileInvalid, connect.CodeInvalidArgument, "the file name, type or size is not valid"},
	{core.ErrFileTooLarge, connect.CodeInvalidArgument, "the file is larger than the maximum allowed size"},
	{core.ErrFileTypeNotAllowed, connect.CodeInvalidArgument, "this file type is not allowed"},
	{core.ErrQuotaExceeded, connect.CodeResourceExhausted, "the workspace's storage quota is used up"},
	{core.ErrUploadMissing, connect.CodeFailedPrecondition, "the file has not been uploaded yet"},
	{core.ErrUploadMismatch, connect.CodeFailedPrecondition, "the uploaded file does not match what was announced"},
	{core.ErrInvalidPageToken, connect.CodeInvalidArgument, "invalid page token"},
}

// toConnectError is the one place domain errors become Connect errors:
// a known sentinel maps to its code with a constant message, an error
// that is already a *connect.Error passes through, and anything else is
// logged in full and answered with a constant internal error.
func toConnectError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ce, ok := errors.AsType[*connect.Error](err); ok {
		return ce
	}
	for _, m := range errorMappings {
		if errors.Is(err, m.sentinel) {
			return connect.NewError(m.code, errors.New(m.message))
		}
	}
	switch {
	case errors.Is(err, context.Canceled):
		return connect.NewError(connect.CodeCanceled, errors.New("request canceled"))
	case errors.Is(err, context.DeadlineExceeded):
		return connect.NewError(connect.CodeDeadlineExceeded, errors.New("deadline exceeded"))
	}
	slog.ErrorContext(ctx, "rpc failed", "error", err)
	return connect.NewError(connect.CodeInternal, errors.New(errInternal))
}

// requiredField is the answer to a request missing a mandatory field.
func requiredField(name string) error {
	return connect.NewError(connect.CodeInvalidArgument, errors.New(name+" is required"))
}
