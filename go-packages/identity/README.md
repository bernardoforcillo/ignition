# identity

Product-domain layer over [authlayer](https://github.com/bernardoforcillo/authlayer)
for the template: authentication, workspaces (the tenant), workspace RBAC and
invitations. authlayer is the engine; this module owns the product's rules
(rate-limit budgets, slug rules, the permission set, "unverified accounts
cannot sign in", mail links) and the sentinel errors a transport layer maps to
status codes. It holds no transport, no SMTP and no database driver.

| Package | What it gives you |
|---|---|
| `identity/auth` | `SignUp`, `VerifyEmail`, `Login`, `Refresh` (rotation, reuse detection), `Logout`, `LogoutAll`, `VerifyAccessToken` |
| `identity/workspace` | `Create`, `Get`, `Can`/`Authorize`, members, ownership transfer, `Invite`/`AcceptInvite`/`RevokeInvitation` |
| `identity/permissions` | The permission statement set and `NewAccess()` (owner / admin / member default roles) |

Ports defined here, implemented by the product: `auth.Mailer`
(`SendVerification`, `SendAccountExists`), `auth.RateLimiter`
(`Allow(ctx, key, limit, window)`, nil = unlimited, fails open on error) and
`workspace.Mailer` (`SendInvitation`). Storage is authlayer's store
interfaces, injected.

## Wiring

In-memory (tests, local experiments), no external services:

```go
import (
	"github.com/bernardoforcillo/authlayer/org"
	"github.com/bernardoforcillo/authlayer/store/memory"

	"github.com/bernardoforcillo/ignition/go-packages/identity/auth"
	"github.com/bernardoforcillo/ignition/go-packages/identity/permissions"
	"github.com/bernardoforcillo/ignition/go-packages/identity/workspace"
)

authSvc, err := auth.NewService(memory.NewAuthStore(), mailer, limiter, auth.Config{
	Secret:     secret, // >= 32 bytes, from your secret manager
	AccessTTL:  15 * time.Minute,
	RefreshTTL: 30 * 24 * time.Hour,
	BaseURL:    "https://app.example.com",
})
wsSvc := workspace.NewService(
	permissions.NewAccess(),
	memory.New[org.Organization, org.Member](),
	memory.NewInviteStore(),
	mailer, "https://app.example.com",
)
```

PostgreSQL, through the sibling `go-packages/database` module's `*DB` (the
`drops` handle that `go-packages/database` returns; authlayer's drops stores
take `*pg.DB`):

```go
import dropsstore "github.com/bernardoforcillo/authlayer/store/drops"

authStore := dropsstore.NewAuthStore(db)
wsStore := dropsstore.New[org.Organization, org.Member](db)
inviteStore := dropsstore.NewInviteStore(db)

// Once, from a migration: idempotent, creates users/sessions/verifications,
// the org tables and invitations.
_ = authStore.CreateSchema(ctx)
```

Then pass those three stores to the constructors above unchanged. The memory
stores do not enforce unique slugs or emails across racing writers; Postgres
does, surfacing `workspace.ErrSlugTaken`.

## Using it from a handler (Connect / HTTP, conceptual)

The handler validates, authenticates, delegates and maps errors; it decides
nothing.

```go
func (h *Handler) CreateInvitation(ctx context.Context, req *connect.Request[pb.InviteRequest]) (*connect.Response[pb.InviteResponse], error) {
	claims, err := h.auth.VerifyAccessToken(bearer(req.Header())) // middleware, usually
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	_, err = h.ws.Invite(ctx, claims.Subject, req.Msg.WorkspaceId, req.Msg.Email, req.Msg.RoleKey)
	return nil, toConnectError(err)
}

func toConnectError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, workspace.ErrNotMember), errors.Is(err, workspace.ErrNotFound):
		return connect.NewError(connect.CodeNotFound, err) // do not reveal existence
	case errors.Is(err, workspace.ErrForbidden), errors.Is(err, workspace.ErrPrivilegeEscalation),
		errors.Is(err, workspace.ErrOwnerOnly):
		return connect.NewError(connect.CodePermissionDenied, err)
	case errors.Is(err, auth.ErrInvalidCredentials), errors.Is(err, auth.ErrEmailNotVerified),
		errors.Is(err, auth.ErrTokenInvalid):
		return connect.NewError(connect.CodeUnauthenticated, err)
	case errors.Is(err, auth.ErrRateLimited):
		return connect.NewError(connect.CodeResourceExhausted, err)
	case errors.Is(err, workspace.ErrSlugTaken):
		return connect.NewError(connect.CodeAlreadyExists, err)
	default:
		return connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
}
```

Sign-up is enumeration-safe: it returns `nil` whether or not the address
exists, so the handler must answer identically either way (the existing
holder gets an `auth.Mailer.SendAccountExists` email instead). Access tokens
are stateless: `LogoutAll` revokes refresh tokens at once, but an issued
access token lives until it expires.

## Roles and permissions

`owner` holds everything, `admin` everything except deleting the workspace,
`member` nothing beyond membership. A privilege-escalation guard stops anyone
granting (by invite or role change) a role more powerful than their own, so an
admin can never mint an owner.

Check a permission with `workspace.Service.Can` / `Authorize`:

```go
err := ws.Authorize(ctx, userID, workspaceID, permissions.ResourceProject, permissions.ActionCreate)
```

To add a product resource, edit `permissions/permissions.go` only: add a
`Resource...` constant and list its actions in `Statements()` (see the marked
example). Owner and admin pick it up automatically; members get nothing until
you grant it. Statements are stored by name, so adding or renaming does not
corrupt saved roles. Custom roles per workspace (granting a subset to
members) are available on the underlying engine; this module does not wrap
them yet.

## Not included

SMTP or any mail implementation, a rate limiter implementation (Redis etc.),
password reset, MFA, magic links, OAuth and account deletion (reachable via
`auth.Service.Authlayer()` for the first ones), workspace rename/delete and
custom-role management, a database driver.
