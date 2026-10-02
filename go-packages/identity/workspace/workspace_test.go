package workspace

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	aa "github.com/bernardoforcillo/authlayer/access"
	"github.com/bernardoforcillo/authlayer/invite"
	"github.com/bernardoforcillo/authlayer/org"
	"github.com/bernardoforcillo/authlayer/scope"
	"github.com/bernardoforcillo/authlayer/store/memory"

	"github.com/bernardoforcillo/ignition/go-packages/identity/permissions"
)

type sentInvite struct{ to, workspace, link string }

type fakeMailer struct {
	sent []sentInvite
	err  error
}

func (m *fakeMailer) SendInvitation(_ context.Context, to, workspaceName, link string) error {
	m.sent = append(m.sent, sentInvite{to, workspaceName, link})
	return m.err
}

func (m *fakeMailer) lastToken(t *testing.T) string {
	t.Helper()
	if len(m.sent) == 0 {
		t.Fatal("no invitation email was sent")
	}
	u, err := url.Parse(m.sent[len(m.sent)-1].link)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query().Get("token")
}

// uniqueSlugStore adds the slug uniqueness the memory store deliberately
// leaves to the database, so the ErrSlugTaken mapping can be exercised.
type uniqueSlugStore struct {
	Store
	slugs map[string]bool
}

func (u *uniqueSlugStore) CreateContainer(ctx context.Context, c org.Organization) (org.Organization, error) {
	if u.slugs[c.Slug] {
		return org.Organization{}, scope.ErrConflict
	}
	u.slugs[c.Slug] = true
	return u.Store.CreateContainer(ctx, c)
}

// WithTx keeps the wrapper in play: the engine creates a workspace inside a
// transaction and would otherwise bypass CreateContainer above.
func (u *uniqueSlugStore) WithTx(ctx context.Context, fn func(Store) error) error {
	return u.Store.WithTx(ctx, func(tx Store) error {
		return fn(&uniqueSlugStore{Store: tx, slugs: u.slugs})
	})
}

func newTestService(t *testing.T, inviteOpts ...invite.Option) (*Service, *fakeMailer) {
	t.Helper()
	m := &fakeMailer{}
	store := &uniqueSlugStore{Store: memory.New[org.Organization, org.Member](), slugs: map[string]bool{}}
	s := NewService(permissions.NewAccess(), store, memory.NewInviteStore(), m, "https://app.test/", inviteOpts...)
	return s, m
}

// inviteAndAccept admits userID to ws at roleKey through the real invite flow.
func inviteAndAccept(t *testing.T, s *Service, m *fakeMailer, ownerID, wsID, userID, roleKey string) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.Invite(ctx, ownerID, wsID, userID+"@example.com", roleKey); err != nil {
		t.Fatalf("Invite %s: %v", roleKey, err)
	}
	if _, err := s.AcceptInvite(ctx, userID, m.lastToken(t)); err != nil {
		t.Fatalf("AcceptInvite %s: %v", userID, err)
	}
}

func TestCreate_OwnerHoldsEverything(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestService(t)
	ws, err := s.Create(ctx, "owner", "  Acme Inc.  ", "")
	if err != nil {
		t.Fatal(err)
	}
	if ws.Name != "Acme Inc." || ws.Slug != "acme-inc" {
		t.Fatalf("workspace = %+v, want trimmed name and derived slug", ws)
	}
	for _, c := range []struct{ res, act string }{
		{permissions.ResourceWorkspace, "delete"},
		{permissions.ResourceMember, "delete"},
		{permissions.ResourceProject, "create"},
	} {
		ok, err := s.Can(ctx, "owner", ws.ID, c.res, act(c.act))
		if err != nil || !ok {
			t.Fatalf("owner Can(%s,%s) = %v, %v; want true", c.res, c.act, ok, err)
		}
	}
}

func TestCreate_Validation(t *testing.T) {
	s, _ := newTestService(t)
	tests := []struct {
		name, ws, slug string
		want           error
	}{
		{"blank name", "  ", "x", ErrInvalidInput},
		{"slug without usable characters", "Acme", "!!!", ErrInvalidInput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := s.Create(context.Background(), "u", tt.ws, tt.slug); !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCreate_DuplicateSlugIsRejected(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestService(t)
	if _, err := s.Create(ctx, "a", "Acme", "acme"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, "b", "Other", "ACME"); !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("err = %v, want ErrSlugTaken", err)
	}
}

func TestSlug(t *testing.T) {
	tests := map[string]string{
		"Acme Inc.":   "acme-inc",
		"  --Hi!!  ":  "hi",
		"a_b  c":      "a-b-c",
		"日本":          "",
		"already-ok1": "already-ok1",
	}
	for in, want := range tests {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInviteAccept_MemberJoinsWithInvitedRole(t *testing.T) {
	ctx := context.Background()
	s, m := newTestService(t)
	ws, _ := s.Create(ctx, "owner", "Acme", "")

	inv, err := s.Invite(ctx, "owner", ws.ID, "  Bob@Example.com ", permissions.RoleMember)
	if err != nil {
		t.Fatal(err)
	}
	if inv.Email != "bob@example.com" {
		t.Fatalf("invitation email = %q, want normalized", inv.Email)
	}
	if got := m.sent[0]; got.to != "bob@example.com" || got.workspace != "Acme" {
		t.Fatalf("mail = %+v", got)
	}
	if got := m.sent[0].link; len(got) < 36 || got[:36] != "https://app.test/accept-invite?token" {
		t.Fatalf("link = %q", got)
	}

	if _, err := s.Get(ctx, "bob", ws.ID); !errors.Is(err, ErrNotMember) {
		t.Fatalf("before accepting: err = %v, want ErrNotMember", err)
	}
	got, err := s.AcceptInvite(ctx, "bob", m.lastToken(t))
	if err != nil || got.ID != ws.ID {
		t.Fatalf("AcceptInvite = %+v, %v", got, err)
	}
	if _, err := s.Get(ctx, "bob", ws.ID); err != nil {
		t.Fatalf("after accepting: %v", err)
	}
	members, err := s.ListMembers(ctx, "bob", ws.ID)
	if err != nil || len(members) != 2 {
		t.Fatalf("members = %d, %v; want 2", len(members), err)
	}
}

func TestInvite_FailuresAreMapped(t *testing.T) {
	ctx := context.Background()
	s, m := newTestService(t)
	ws, _ := s.Create(ctx, "owner", "Acme", "")

	t.Run("unknown token", func(t *testing.T) {
		if _, err := s.AcceptInvite(ctx, "bob", "bogus"); !errors.Is(err, ErrInviteInvalid) {
			t.Fatalf("err = %v, want ErrInviteInvalid", err)
		}
	})
	t.Run("token is single use", func(t *testing.T) {
		if _, err := s.Invite(ctx, "owner", ws.ID, "once@example.com", permissions.RoleMember); err != nil {
			t.Fatal(err)
		}
		tok := m.lastToken(t)
		if _, err := s.AcceptInvite(ctx, "once", tok); err != nil {
			t.Fatal(err)
		}
		if _, err := s.AcceptInvite(ctx, "other", tok); !errors.Is(err, ErrInviteInvalid) {
			t.Fatalf("err = %v, want ErrInviteInvalid", err)
		}
	})
	t.Run("owner accepting a member invite is not demoted", func(t *testing.T) {
		if _, err := s.Invite(ctx, "owner", ws.ID, "dup@example.com", permissions.RoleMember); err != nil {
			t.Fatal(err)
		}
		if _, err := s.AcceptInvite(ctx, "owner", m.lastToken(t)); err != nil {
			t.Fatalf("err = %v, want idempotent success", err)
		}
		if ok, _ := s.Can(ctx, "owner", ws.ID, permissions.ResourceWorkspace, act("delete")); !ok {
			t.Fatal("owner lost ownership by accepting an invitation")
		}
	})
	t.Run("mail failure surfaces", func(t *testing.T) {
		boom := errors.New("smtp down")
		m.err = boom
		defer func() { m.err = nil }()
		if _, err := s.Invite(ctx, "owner", ws.ID, "x@example.com", permissions.RoleMember); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want smtp error", err)
		}
	})
}

func TestAcceptInvite_ExpiredInvitationIsRejected(t *testing.T) {
	ctx := context.Background()
	s, m := newTestService(t, invite.WithInviteExpiry(time.Nanosecond))
	ws, _ := s.Create(ctx, "owner", "Acme", "")
	if _, err := s.Invite(ctx, "owner", ws.ID, "late@example.com", permissions.RoleMember); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AcceptInvite(ctx, "late", m.lastToken(t)); !errors.Is(err, ErrInviteExpired) {
		t.Fatalf("err = %v, want ErrInviteExpired", err)
	}
}

func TestInvite_RevokedInvitationCannotBeAccepted(t *testing.T) {
	ctx := context.Background()
	s, m := newTestService(t)
	ws, _ := s.Create(ctx, "owner", "Acme", "")
	inv, _ := s.Invite(ctx, "owner", ws.ID, "bob@example.com", permissions.RoleMember)

	pending, err := s.ListInvitations(ctx, "owner", ws.ID)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending = %d, %v; want 1", len(pending), err)
	}
	if err := s.RevokeInvitation(ctx, "owner", ws.ID, inv.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AcceptInvite(ctx, "bob", m.lastToken(t)); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("err = %v, want ErrInviteInvalid", err)
	}
}

func TestRoles_PermissionMatrix(t *testing.T) {
	ctx := context.Background()
	s, m := newTestService(t)
	ws, _ := s.Create(ctx, "owner", "Acme", "")
	inviteAndAccept(t, s, m, "owner", ws.ID, "adam", permissions.RoleAdmin)
	inviteAndAccept(t, s, m, "owner", ws.ID, "mia", permissions.RoleMember)

	tests := []struct {
		user, resource, action string
		want                   bool
	}{
		{"owner", permissions.ResourceWorkspace, "delete", true},
		{"adam", permissions.ResourceWorkspace, "update", true},
		{"adam", permissions.ResourceWorkspace, "delete", false}, // owner-only
		{"adam", permissions.ResourceInvite, "create", true},
		{"adam", permissions.ResourceProject, "delete", true},
		{"mia", permissions.ResourceInvite, "create", false},
		{"mia", permissions.ResourceMember, "delete", false},
		{"mia", permissions.ResourceProject, "read", false},
		{"stranger", permissions.ResourceProject, "read", false},
	}
	for _, tt := range tests {
		t.Run(tt.user+"/"+tt.resource+"/"+tt.action, func(t *testing.T) {
			got, err := s.Can(ctx, tt.user, ws.ID, tt.resource, act(tt.action))
			if err != nil || got != tt.want {
				t.Fatalf("Can = %v, %v; want %v", got, err, tt.want)
			}
		})
	}
}

func TestAuthorize_MapsDenials(t *testing.T) {
	ctx := context.Background()
	s, m := newTestService(t)
	ws, _ := s.Create(ctx, "owner", "Acme", "")
	inviteAndAccept(t, s, m, "owner", ws.ID, "mia", permissions.RoleMember)

	if err := s.Authorize(ctx, "mia", ws.ID, permissions.ResourceInvite, act("create")); !errors.Is(err, ErrForbidden) {
		t.Fatalf("member: err = %v, want ErrForbidden", err)
	}
	if err := s.Authorize(ctx, "stranger", ws.ID, permissions.ResourceInvite, act("create")); !errors.Is(err, ErrNotMember) {
		t.Fatalf("stranger: err = %v, want ErrNotMember", err)
	}
	if err := s.Authorize(ctx, "owner", "missing", permissions.ResourceInvite, act("create")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing workspace: err = %v, want ErrNotFound", err)
	}
	if err := s.Authorize(ctx, "owner", ws.ID, permissions.ResourceInvite, act("create")); err != nil {
		t.Fatalf("owner: %v", err)
	}
}

func TestInvite_MemberCannotInvite(t *testing.T) {
	ctx := context.Background()
	s, m := newTestService(t)
	ws, _ := s.Create(ctx, "owner", "Acme", "")
	inviteAndAccept(t, s, m, "owner", ws.ID, "mia", permissions.RoleMember)

	if _, err := s.Invite(ctx, "mia", ws.ID, "x@example.com", permissions.RoleMember); !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}

func TestPrivilegeEscalation_AdminCannotGrantOwner(t *testing.T) {
	ctx := context.Background()
	s, m := newTestService(t)
	ws, _ := s.Create(ctx, "owner", "Acme", "")
	inviteAndAccept(t, s, m, "owner", ws.ID, "adam", permissions.RoleAdmin)
	inviteAndAccept(t, s, m, "owner", ws.ID, "mia", permissions.RoleMember)

	t.Run("by inviting", func(t *testing.T) {
		_, err := s.Invite(ctx, "adam", ws.ID, "evil@example.com", permissions.RoleOwner)
		if !errors.Is(err, ErrPrivilegeEscalation) {
			t.Fatalf("err = %v, want ErrPrivilegeEscalation", err)
		}
	})
	t.Run("by promoting a member", func(t *testing.T) {
		err := s.ChangeMemberRole(ctx, "adam", ws.ID, "mia", permissions.RoleOwner)
		if !errors.Is(err, ErrPrivilegeEscalation) {
			t.Fatalf("err = %v, want ErrPrivilegeEscalation", err)
		}
	})
	t.Run("a plain member cannot promote at all", func(t *testing.T) {
		err := s.ChangeMemberRole(ctx, "mia", ws.ID, "mia", permissions.RoleAdmin)
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
	t.Run("admin can grant up to admin", func(t *testing.T) {
		if err := s.ChangeMemberRole(ctx, "adam", ws.ID, "mia", permissions.RoleAdmin); err != nil {
			t.Fatal(err)
		}
		ok, _ := s.Can(ctx, "mia", ws.ID, permissions.ResourceInvite, act("create"))
		if !ok {
			t.Fatal("promoted member should now be able to invite")
		}
	})
}

func TestOwnership_Rules(t *testing.T) {
	ctx := context.Background()
	s, m := newTestService(t)
	ws, _ := s.Create(ctx, "owner", "Acme", "")
	inviteAndAccept(t, s, m, "owner", ws.ID, "adam", permissions.RoleAdmin)

	if err := s.RemoveMember(ctx, "adam", ws.ID, "owner"); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("remove owner: err = %v, want ErrLastOwner", err)
	}
	if err := s.Leave(ctx, "owner", ws.ID); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("owner leaving: err = %v, want ErrLastOwner", err)
	}
	if err := s.TransferOwnership(ctx, "adam", ws.ID, "adam"); !errors.Is(err, ErrOwnerOnly) {
		t.Fatalf("admin transferring: err = %v, want ErrOwnerOnly", err)
	}
	if err := s.TransferOwnership(ctx, "owner", ws.ID, "adam"); err != nil {
		t.Fatalf("transfer: %v", err)
	}
	if err := s.Leave(ctx, "owner", ws.ID); err != nil {
		t.Fatalf("former owner leaving: %v", err)
	}
	if _, err := s.Get(ctx, "owner", ws.ID); !errors.Is(err, ErrNotMember) {
		t.Fatalf("after leaving: err = %v, want ErrNotMember", err)
	}
	if ok, _ := s.Can(ctx, "adam", ws.ID, permissions.ResourceWorkspace, act("delete")); !ok {
		t.Fatal("new owner should be able to delete the workspace")
	}
}

func TestRemoveMember_RevokesAccess(t *testing.T) {
	ctx := context.Background()
	s, m := newTestService(t)
	ws, _ := s.Create(ctx, "owner", "Acme", "")
	inviteAndAccept(t, s, m, "owner", ws.ID, "mia", permissions.RoleMember)

	if err := s.RemoveMember(ctx, "owner", ws.ID, "mia"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListMembers(ctx, "mia", ws.ID); !errors.Is(err, ErrNotMember) {
		t.Fatalf("err = %v, want ErrNotMember", err)
	}
}

func act(a string) aa.Action { return aa.Action(a) }
