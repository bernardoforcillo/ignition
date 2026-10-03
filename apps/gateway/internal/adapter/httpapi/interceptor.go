package httpapi

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"

	"github.com/bernardoforcillo/ignition/go-packages/identity/auth"
	"github.com/bernardoforcillo/ignition/go-packages/telemetry"

	"github.com/bernardoforcillo/ignition/go-packages/proto/gen/saas/v1/saasv1connect"
)

// publicProcedures are the only RPCs callable without an access token.
// Everything else is denied by default, so a newly added RPC is
// protected until someone deliberately lists it here.
var publicProcedures = map[string]bool{
	saasv1connect.AuthServiceSignUpProcedure:      true,
	saasv1connect.AuthServiceVerifyEmailProcedure: true,
	saasv1connect.AuthServiceLoginProcedure:       true,
	saasv1connect.AuthServiceRefreshProcedure:     true,

	saasv1connect.AuthServiceRequestPasswordResetProcedure: true,
	saasv1connect.AuthServiceResetPasswordProcedure:        true,

	// External sign-in: the web lists the configured providers, then redeems the one-time code
	// the OAuth callback redirected back with. Neither has a session yet.
	saasv1connect.AuthServiceListAuthProvidersProcedure: true,
	saasv1connect.AuthServiceExchangeOAuthCodeProcedure: true,

	// The purchasable catalog is public by design: the landing page shows prices to visitors who
	// have no account yet. It holds nothing workspace-specific.
	saasv1connect.BillingServiceListPricesProcedure: true,
}

// tokenVerifier checks a bearer access token; *auth.Service satisfies it.
type tokenVerifier interface {
	VerifyAccessToken(raw string) (auth.Claims, error)
}

type subjectKey struct{}

// subjectFrom returns the authenticated user id the interceptor stored.
func subjectFrom(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(subjectKey{}).(string)
	return id, ok && id != ""
}

var errUnauthenticated = errors.New("missing or invalid access token")

// authInterceptor authenticates every non-public RPC from the
// "Authorization: Bearer <access token>" header and stores the token's
// subject in the context. Authorization (what the subject may do in a
// workspace) stays with the handlers and the workspace service.
func authInterceptor(verifier tokenVerifier) connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if publicProcedures[req.Spec().Procedure] {
				return next(ctx, req)
			}
			raw, ok := strings.CutPrefix(req.Header().Get("Authorization"), "Bearer ")
			if !ok || raw == "" {
				return nil, connect.NewError(connect.CodeUnauthenticated, errUnauthenticated)
			}
			claims, err := verifier.VerifyAccessToken(raw)
			if err != nil || claims.Subject == "" {
				return nil, connect.NewError(connect.CodeUnauthenticated, errUnauthenticated)
			}
			// The account id also attributes any error logged while serving this request.
			ctx = telemetry.WithDistinctID(ctx, claims.Subject)
			return next(context.WithValue(ctx, subjectKey{}, claims.Subject), req)
		}
	})
}
