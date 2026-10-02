// Package workspace is the tenant: an organization-style scope with an owner
// and members, role-based permissions and email invitations. The engine is
// authlayer's org/scope/invite; this package owns the product rules (slug
// normalization, invitation links and mail) and the sentinel errors a
// transport layer maps to status codes.
//
// Every method takes the acting user's id explicitly and builds the context
// authlayer reads, so callers never touch authlayer's context helpers.
package workspace

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/bernardoforcillo/authlayer/access"
	"github.com/bernardoforcillo/authlayer/invite"
	"github.com/bernardoforcillo/authlayer/org"
	"github.com/bernardoforcillo/authlayer/scope"
)

// Workspace and Member are authlayer's organization types; a workspace is an
// organization with a product name.
type (
	Workspace  = org.Organization
	Member     = org.Member
	Invitation = invite.EmailInvite
	// Store and InviteStore are the persistence ports (memory or drops).
	Store       = org.Store
	InviteStore = invite.Store
)

// Sentinel errors for the transport layer. Match with errors.Is.
var (
	ErrNotFound            = errors.New("workspace not found")
	ErrNotMember           = errors.New("not a member of this workspace")
	ErrForbidden           = errors.New("insufficient permissions")
	ErrPrivilegeEscalation = errors.New("cannot grant permissions you do not have")
	ErrRoleNotFound        = errors.New("role not found")
	ErrLastOwner           = errors.New("cannot remove or demote the owner")
	ErrOwnerOnly           = errors.New("only the owner may do this")
	ErrSlugTaken           = errors.New("workspace slug already taken")
	ErrInviteInvalid       = errors.New("invitation invalid")
	ErrInviteExpired       = errors.New("invitation expired")
	ErrInvalidInput        = errors.New("invalid input")
)

// Mailer delivers workspace emails. Implementations live in the product.
type Mailer interface {
	// SendInvitation delivers the accept link for an invitation to a workspace.
	SendInvitation(ctx context.Context, to, workspaceName, link string) error
}

// Service is the workspace domain.
type Service struct {
	sc     *org.Service
	inv    *invite.Service[org.Organization, org.Member, *org.Organization, *org.Member]
	mailer Mailer
	base   string
}

// NewService wires the engines over the injected stores. ac comes from
// permissions.NewAccess; baseURL prefixes invitation links; inviteOpts tune
// the invitation engine (invite.WithInviteExpiry, default seven days).
func NewService(ac *access.Access, store Store, inviteStore InviteStore, mailer Mailer, baseURL string, inviteOpts ...invite.Option) *Service {
	sc := org.New(ac, store)
	return &Service{
		sc:     sc,
		inv:    invite.New(sc.Service, inviteStore, inviteOpts...),
		mailer: mailer,
		base:   strings.TrimRight(baseURL, "/"),
	}
}

func actor(ctx context.Context, userID, workspaceID string) context.Context {
	return scope.WithScope(scope.WithSubject(ctx, userID), workspaceID)
}

// Create makes a workspace with userID as its owner. An empty slug is derived
// from the name.
func (s *Service) Create(ctx context.Context, userID, name, slug string) (Workspace, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Workspace{}, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if slug == "" {
		slug = name
	}
	slug = Slug(slug)
	if slug == "" {
		return Workspace{}, fmt.Errorf("%w: slug has no usable characters", ErrInvalidInput)
	}
	ws, err := s.sc.CreateOrganization(scope.WithSubject(ctx, userID), name, slug)
	return ws, mapError(err)
}

// Get returns the workspace to a member.
func (s *Service) Get(ctx context.Context, userID, workspaceID string) (Workspace, error) {
	if _, _, err := s.sc.Standing(ctx, workspaceID, userID); err != nil {
		return Workspace{}, mapError(err)
	}
	ws, err := s.sc.Container(ctx, workspaceID)
	return ws, mapError(err)
}

// Can reports whether userID holds every action on resource in the workspace.
// A non-member is not an error here: they simply cannot.
func (s *Service) Can(ctx context.Context, userID, workspaceID, resource string, actions ...access.Action) (bool, error) {
	ok, err := s.sc.HasPermission(ctx, workspaceID, userID, map[string][]access.Action{resource: actions})
	if errors.Is(err, scope.ErrNotMember) {
		return false, nil
	}
	return ok, mapError(err)
}

// Authorize is Can that returns ErrForbidden (or ErrNotMember) instead of false.
func (s *Service) Authorize(ctx context.Context, userID, workspaceID, resource string, actions ...access.Action) error {
	return mapError(s.sc.Authorize(actor(ctx, userID, workspaceID), resource, actions...))
}

// ListMembers needs no permission beyond membership.
func (s *Service) ListMembers(ctx context.Context, userID, workspaceID string) ([]Member, error) {
	ms, err := s.sc.ListMembers(actor(ctx, userID, workspaceID))
	return ms, mapError(err)
}

// ChangeMemberRole moves target to roleKey. The escalation guard stops anyone
// granting a role more powerful than their own.
func (s *Service) ChangeMemberRole(ctx context.Context, userID, workspaceID, targetUserID, roleKey string) error {
	return mapError(s.sc.ChangeMemberRole(actor(ctx, userID, workspaceID), targetUserID, roleKey))
}

// RemoveMember removes target; the owner cannot be removed.
func (s *Service) RemoveMember(ctx context.Context, userID, workspaceID, targetUserID string) error {
	return mapError(s.sc.RemoveMember(actor(ctx, userID, workspaceID), targetUserID))
}

// Leave removes userID from the workspace; the owner must transfer first.
func (s *Service) Leave(ctx context.Context, userID, workspaceID string) error {
	return mapError(s.sc.LeaveContainer(actor(ctx, userID, workspaceID)))
}

// TransferOwnership makes newOwnerID the owner; only the current owner may.
func (s *Service) TransferOwnership(ctx context.Context, userID, workspaceID, newOwnerID string) error {
	return mapError(s.sc.TransferOwnership(actor(ctx, userID, workspaceID), newOwnerID))
}

// Invite mints an invitation for email at roleKey and mails the accept link.
// Re-inviting an address replaces its pending invitation.
func (s *Service) Invite(ctx context.Context, userID, workspaceID, email, roleKey string) (Invitation, error) {
	ac := actor(ctx, userID, workspaceID)
	inv, token, err := s.inv.InviteByEmail(ac, email, roleKey)
	if err != nil {
		return Invitation{}, mapError(err)
	}
	ws, err := s.sc.Container(ctx, workspaceID)
	if err != nil {
		return Invitation{}, mapError(err)
	}
	link := s.base + "/accept-invite?token=" + url.QueryEscape(token)
	if err := s.mailer.SendInvitation(ctx, inv.Email, ws.Name, link); err != nil {
		return Invitation{}, fmt.Errorf("send invitation: %w", err)
	}
	return inv, nil
}

// ListInvitations returns the pending invitations.
func (s *Service) ListInvitations(ctx context.Context, userID, workspaceID string) ([]Invitation, error) {
	invs, err := s.inv.ListInvites(actor(ctx, userID, workspaceID))
	return invs, mapError(err)
}

// RevokeInvitation cancels a pending invitation.
func (s *Service) RevokeInvitation(ctx context.Context, userID, workspaceID, invitationID string) error {
	return mapError(s.inv.RevokeInvite(actor(ctx, userID, workspaceID), invitationID))
}

// AcceptInvite adds userID to the invited workspace with the invited role.
// The invite token is the credential: authlayer does not check that userID's
// email matches the invited address.
func (s *Service) AcceptInvite(ctx context.Context, userID, token string) (Workspace, error) {
	ws, err := s.inv.AcceptInvite(scope.WithSubject(ctx, userID), token)
	return ws, mapError(err)
}

// Slug lowercases, keeps [a-z0-9] and collapses everything else into single
// dashes, the form the unique constraint is enforced on.
func Slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// mapError translates authlayer's sentinels into this package's. Anything
// unrecognised (a store failure) passes through untouched.
func mapError(err error) error {
	pairs := []struct{ from, to error }{
		{scope.ErrContainerNotFound, ErrNotFound},
		{scope.ErrNotMember, ErrNotMember},
		{scope.ErrForbidden, ErrForbidden},
		{scope.ErrPrivilegeEscalation, ErrPrivilegeEscalation},
		{scope.ErrRoleNotFound, ErrRoleNotFound},
		{scope.ErrLastOwner, ErrLastOwner},
		{scope.ErrOwnerOnly, ErrOwnerOnly},
		{scope.ErrConflict, ErrSlugTaken},
		{invite.ErrInviteNotFound, ErrInviteInvalid},
		{invite.ErrInviteExpired, ErrInviteExpired},
	}
	for _, p := range pairs {
		if errors.Is(err, p.from) {
			return p.to
		}
	}
	return err
}
