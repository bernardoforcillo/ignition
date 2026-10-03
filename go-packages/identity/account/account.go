// Package account is the signed-in user's own account: who they are, a machine-readable export
// of what the product holds about them (GDPR art. 15/20) and erasure (art. 17).
//
// It orchestrates the authentication and workspace domains, which know nothing of each other, so
// it is the one place that decides what deleting an account means for workspaces: a workspace
// the user owns alone is deleted with them, one they own that still has other members blocks the
// deletion until ownership is transferred, and plain memberships are dropped.
//
// Persistence that authlayer's ports do not cover (deleting a workspace, finding invitations by
// address) is the Store port, implemented by the product over its database.
package account

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/bernardoforcillo/ignition/go-packages/identity/auth"
	"github.com/bernardoforcillo/ignition/go-packages/identity/workspace"
)

// ErrOwnsSharedWorkspace refuses a deletion while the user is the owner of a workspace that
// still has other members: someone has to take it over first (workspace ownership transfer).
var ErrOwnsSharedWorkspace = errors.New("transfer ownership of your shared workspaces first")

// ErrActiveSubscription refuses a deletion that would erase a workspace the payment provider is
// still billing: cancel the subscription first (billing portal).
var ErrActiveSubscription = errors.New("cancel the workspace subscription first")

// Auth is the part of the authentication service the account needs; *auth.Service satisfies it.
type Auth interface {
	User(ctx context.Context, userID string) (auth.User, error)
	DeleteAccount(ctx context.Context, userID, password string, cleanup func(ctx context.Context, userID string) error) error
}

// Workspaces is the part of the workspace service the account needs; *workspace.Service
// satisfies it.
type Workspaces interface {
	ListFor(ctx context.Context, userID string) ([]workspace.Membership, error)
	ListMembers(ctx context.Context, userID, workspaceID string) ([]workspace.Member, error)
	Leave(ctx context.Context, userID, workspaceID string) error
}

// PendingInvitation is an invitation addressed to an email address that nobody has accepted.
type PendingInvitation struct {
	WorkspaceName string
	RoleKey       string
	CreatedAt     time.Time
	ExpiresAt     time.Time
}

// Store is the persistence the engines do not offer. Both methods are idempotent: a deletion that
// failed part-way is retried by the user.
type Store interface {
	// CanErase returns ErrActiveSubscription when the workspace is still being billed, nil
	// otherwise. It is asked for every workspace before anything is removed.
	CanErase(ctx context.Context, workspaceID string) error
	// EraseWorkspace removes a workspace and everything keyed on it (members, invitations,
	// entitlements, billing customer link).
	EraseWorkspace(ctx context.Context, workspaceID string) error
	// PendingInvitations lists the unexpired invitations addressed to email, across workspaces.
	PendingInvitations(ctx context.Context, email string) ([]PendingInvitation, error)
	// DeleteInvitations removes every invitation addressed to email.
	DeleteInvitations(ctx context.Context, email string) error
}

// Service is the account use cases.
type Service struct {
	auth       Auth
	workspaces Workspaces
	store      Store
	now        func() time.Time
}

// NewService wires the use cases over the two domains and the store.
func NewService(a Auth, w Workspaces, s Store) *Service {
	return &Service{auth: a, workspaces: w, store: s, now: time.Now}
}

// Me returns the account.
func (s *Service) Me(ctx context.Context, userID string) (auth.User, error) {
	return s.auth.User(ctx, userID)
}

// The export document. Secrets (password hash, tokens, sessions) are deliberately not part of it.
type exportDoc struct {
	GeneratedAt string             `json:"generated_at"`
	Account     exportAccount      `json:"account"`
	Memberships []exportMembership `json:"memberships"`
	Invitations []exportInvitation `json:"pending_invitations"`
}

type exportAccount struct {
	ID              string `json:"id"`
	Email           string `json:"email"`
	EmailVerifiedAt string `json:"email_verified_at,omitempty"`
	CreatedAt       string `json:"created_at"`
}

type exportMembership struct {
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Role        string `json:"role"`
	Owner       bool   `json:"owner"`
	CreatedAt   string `json:"created_at"`
}

type exportInvitation struct {
	WorkspaceName string `json:"workspace_name"`
	Role          string `json:"role"`
	CreatedAt     string `json:"created_at"`
	ExpiresAt     string `json:"expires_at"`
}

// Export returns everything the product holds about the user as an indented JSON document and a
// suggested file name.
func (s *Service) Export(ctx context.Context, userID string) (data []byte, filename string, err error) {
	u, err := s.auth.User(ctx, userID)
	if err != nil {
		return nil, "", err
	}
	memberships, err := s.workspaces.ListFor(ctx, userID)
	if err != nil {
		return nil, "", err
	}
	invitations, err := s.store.PendingInvitations(ctx, u.Email)
	if err != nil {
		return nil, "", fmt.Errorf("listing invitations: %w", err)
	}

	now := s.now().UTC()
	doc := exportDoc{
		GeneratedAt: ts(now),
		Account:     exportAccount{ID: u.ID, Email: u.Email, CreatedAt: ts(u.CreatedAt)},
		Memberships: make([]exportMembership, 0, len(memberships)),
		Invitations: make([]exportInvitation, 0, len(invitations)),
	}
	if u.VerifiedAt != nil {
		doc.Account.EmailVerifiedAt = ts(*u.VerifiedAt)
	}
	for _, m := range memberships {
		doc.Memberships = append(doc.Memberships, exportMembership{
			WorkspaceID: m.Workspace.ID, Name: m.Workspace.Name, Slug: m.Workspace.Slug,
			Role: m.RoleKey, Owner: m.Workspace.OwnerID == userID, CreatedAt: ts(m.Workspace.CreatedAt),
		})
	}
	for _, inv := range invitations {
		doc.Invitations = append(doc.Invitations, exportInvitation{
			WorkspaceName: inv.WorkspaceName, Role: inv.RoleKey,
			CreatedAt: ts(inv.CreatedAt), ExpiresAt: ts(inv.ExpiresAt),
		})
	}
	data, err = json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, "", fmt.Errorf("encoding export: %w", err)
	}
	return data, "ignition-export-" + now.Format(time.DateOnly) + ".json", nil
}

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// Delete erases the account after re-checking password (auth.ErrInvalidCredentials otherwise).
// Once the password is verified and before anything is removed, it refuses with
// ErrOwnsSharedWorkspace if the user owns a workspace that has other members, or
// ErrActiveSubscription if one they own alone is still billed; otherwise it
// deletes the workspaces the user owns alone, leaves the others and drops invitations addressed
// to their address. Then authentication removes sessions, tokens and the user itself. Safe to
// retry after a failure.
func (s *Service) Delete(ctx context.Context, userID, password string) error {
	u, err := s.auth.User(ctx, userID)
	if err != nil {
		return err
	}
	return s.auth.DeleteAccount(ctx, userID, password, func(ctx context.Context, userID string) error {
		return s.cleanup(ctx, userID, u.Email)
	})
}

func (s *Service) cleanup(ctx context.Context, userID, email string) error {
	memberships, err := s.workspaces.ListFor(ctx, userID)
	if err != nil {
		return err
	}
	// Decide everything before changing anything, so a refusal leaves no half-done state.
	var owned, joined []workspace.Workspace
	for _, m := range memberships {
		if m.Workspace.OwnerID != userID {
			joined = append(joined, m.Workspace)
			continue
		}
		members, err := s.workspaces.ListMembers(ctx, userID, m.Workspace.ID)
		if err != nil {
			return err
		}
		if len(members) > 1 {
			return ErrOwnsSharedWorkspace
		}
		if err := s.store.CanErase(ctx, m.Workspace.ID); err != nil {
			return err
		}
		owned = append(owned, m.Workspace)
	}
	for _, ws := range joined {
		if err := s.workspaces.Leave(ctx, userID, ws.ID); err != nil {
			return fmt.Errorf("leaving workspace %s: %w", ws.ID, err)
		}
	}
	for _, ws := range owned {
		if err := s.store.EraseWorkspace(ctx, ws.ID); err != nil {
			return fmt.Errorf("erasing workspace %s: %w", ws.ID, err)
		}
	}
	if err := s.store.DeleteInvitations(ctx, email); err != nil {
		return fmt.Errorf("deleting invitations: %w", err)
	}
	return nil
}

var (
	_ Auth       = (*auth.Service)(nil)
	_ Workspaces = (*workspace.Service)(nil)
)
