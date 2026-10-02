package httpapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"github.com/bernardoforcillo/ignition/go-packages/billing"
	"github.com/bernardoforcillo/ignition/go-packages/features"
	"github.com/bernardoforcillo/ignition/go-packages/identity/auth"
	"github.com/bernardoforcillo/ignition/go-packages/identity/workspace"

	"github.com/bernardoforcillo/ignition/apps/gateway/internal/core"
)

func TestToConnectError_MapsSentinelsToCodes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want connect.Code
	}{
		{"invalid credentials", auth.ErrInvalidCredentials, connect.CodeUnauthenticated},
		{"email not verified", auth.ErrEmailNotVerified, connect.CodeUnauthenticated},
		{"bad token", auth.ErrTokenInvalid, connect.CodeUnauthenticated},
		{"weak password", auth.ErrWeakPassword, connect.CodeInvalidArgument},
		{"rate limited", auth.ErrRateLimited, connect.CodeResourceExhausted},
		{"workspace missing", workspace.ErrNotFound, connect.CodeNotFound},
		{"not a member hides existence", workspace.ErrNotMember, connect.CodeNotFound},
		{"forbidden", workspace.ErrForbidden, connect.CodePermissionDenied},
		{"privilege escalation", workspace.ErrPrivilegeEscalation, connect.CodePermissionDenied},
		{"owner only", workspace.ErrOwnerOnly, connect.CodePermissionDenied},
		{"role not found", workspace.ErrRoleNotFound, connect.CodeInvalidArgument},
		{"last owner", workspace.ErrLastOwner, connect.CodeFailedPrecondition},
		{"slug taken", workspace.ErrSlugTaken, connect.CodeAlreadyExists},
		{"invite invalid", workspace.ErrInviteInvalid, connect.CodeNotFound},
		{"invite expired", workspace.ErrInviteExpired, connect.CodeFailedPrecondition},
		{"invalid input", workspace.ErrInvalidInput, connect.CodeInvalidArgument},
		{"not entitled", features.ErrNotEntitled, connect.CodePermissionDenied},
		{"limit reached", features.ErrLimitReached, connect.CodeResourceExhausted},
		{"feature disabled", features.ErrDisabled, connect.CodeUnavailable},
		{"unknown price", billing.ErrUnknownPrice, connect.CodeInvalidArgument},
		{"no billing customer", core.ErrNoBillingCustomer, connect.CodeFailedPrecondition},
		{"canceled", context.Canceled, connect.CodeCanceled},
		{"deadline", context.DeadlineExceeded, connect.CodeDeadlineExceeded},
		{"unknown failure", errors.New("pq: connection refused at 10.0.0.5"), connect.CodeInternal},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := toConnectError(context.Background(), tc.err)
			if code := connect.CodeOf(got); code != tc.want {
				t.Fatalf("code = %v, want %v", code, tc.want)
			}
		})
	}
}

func TestToConnectError_WrappedSentinelStillMaps(t *testing.T) {
	err := fmt.Errorf("handler: %w", workspace.ErrSlugTaken)
	if code := connect.CodeOf(toConnectError(context.Background(), err)); code != connect.CodeAlreadyExists {
		t.Fatalf("code = %v, want AlreadyExists", code)
	}
}

func TestToConnectError_NeverLeaksErrorText(t *testing.T) {
	secret := "password authentication failed for user admin at db.internal:5432"
	tests := []error{
		errors.New(secret),
		fmt.Errorf("%w: %s", auth.ErrWeakPassword, secret),
		fmt.Errorf("%w: %s", workspace.ErrInvalidInput, secret),
		fmt.Errorf("%w: %s", features.ErrDisabled, secret),
	}
	for _, in := range tests {
		got := toConnectError(context.Background(), in)
		if strings.Contains(got.Error(), "db.internal") || strings.Contains(got.Error(), "admin") {
			t.Fatalf("client-visible error leaks detail: %q", got.Error())
		}
	}
}

func TestToConnectError_UnknownErrorIsConstantInternal(t *testing.T) {
	got := toConnectError(context.Background(), errors.New("boom"))
	ce, ok := errors.AsType[*connect.Error](got)
	if !ok || ce.Code() != connect.CodeInternal || ce.Message() != errInternal {
		t.Fatalf("got %v, want internal %q", got, errInternal)
	}
}

func TestToConnectError_NilAndConnectErrorsPassThrough(t *testing.T) {
	if toConnectError(context.Background(), nil) != nil {
		t.Fatal("nil must stay nil")
	}
	in := connect.NewError(connect.CodeUnauthenticated, errors.New("x"))
	if got := toConnectError(context.Background(), in); got != in {
		t.Fatalf("connect error must pass through unchanged, got %v", got)
	}
}
