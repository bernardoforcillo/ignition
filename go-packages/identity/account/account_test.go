package account

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	alauth "github.com/bernardoforcillo/authlayer/auth"
	"github.com/bernardoforcillo/authlayer/org"
	"github.com/bernardoforcillo/authlayer/password"
	"github.com/bernardoforcillo/authlayer/store/memory"
	"golang.org/x/crypto/bcrypt"

	"github.com/bernardoforcillo/ignition/go-packages/identity/auth"
	"github.com/bernardoforcillo/ignition/go-packages/identity/permissions"
	"github.com/bernardoforcillo/ignition/go-packages/identity/workspace"
)

const (
	pw    = "Correct-Horse-9!x"
	email = "ada@example.com"
)

// fakeMail satisfies both identity Mailer ports and keeps the last link of each kind.
type fakeMail struct{ verify, invite string }

func (m *fakeMail) SendVerification(_ context.Context, _, link string) error {
	m.verify = link
	return nil
}
func (m *fakeMail) SendAccountExists(context.Context, string) error         { return nil }
func (m *fakeMail) SendPasswordReset(context.Context, string, string) error { return nil }
func (m *fakeMail) SendInvitation(_ context.Context, _, _, link string) error {
	m.invite = link
	return nil
}

func tokenOf(link string) string { return link[strings.Index(link, "token=")+len("token="):] }

// fakeStore records what the product database would be asked to remove.
type fakeStore struct {
	erased      []string
	invDeleted  []string
	pending     []PendingInvitation
	eraseErr    error
	canErase    map[string]error
	invitesErr  error
	invitesSeen string
}

func (f *fakeStore) CanErase(_ context.Context, id string) error { return f.canErase[id] }

func (f *fakeStore) EraseWorkspace(_ context.Context, id string) error {
	if f.eraseErr != nil {
		return f.eraseErr
	}
	f.erased = append(f.erased, id)
	return nil
}

func (f *fakeStore) PendingInvitations(_ context.Context, e string) ([]PendingInvitation, error) {
	f.invitesSeen = e
	return f.pending, f.invitesErr
}

func (f *fakeStore) DeleteInvitations(_ context.Context, e string) error {
	f.invDeleted = append(f.invDeleted, e)
	return nil
}

type env struct {
	svc   *Service
	auth  *auth.Service
	ws    *workspace.Service
	mail  *fakeMail
	store *fakeStore
}

func newEnv(t *testing.T) *env {
	t.Helper()
	mail := &fakeMail{}
	a, err := auth.NewService(memory.NewAuthStore(), mail, nil, auth.Config{
		Secret: []byte("0123456789abcdef0123456789abcdef"), AccessTTL: time.Minute, RefreshTTL: time.Hour, BaseURL: "https://app.test",
	}, alauth.WithHasher(password.Bcrypt(bcrypt.MinCost)))
	if err != nil {
		t.Fatal(err)
	}
	w := workspace.NewService(permissions.NewAccess(), memory.New[org.Organization, org.Member](), memory.NewInviteStore(), mail, "https://app.test")
	st := &fakeStore{}
	svc := NewService(a, w, st)
	svc.now = func() time.Time { return time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC) }
	return &env{svc: svc, auth: a, ws: w, mail: mail, store: st}
}

// signUp creates a verified account and returns its id.
func (e *env) signUp(t *testing.T, addr string) string {
	t.Helper()
	ctx := context.Background()
	if err := e.auth.SignUp(ctx, addr, pw, "203.0.113.1"); err != nil {
		t.Fatal(err)
	}
	if err := e.auth.VerifyEmail(ctx, tokenOf(e.mail.verify)); err != nil {
		t.Fatal(err)
	}
	tok, err := e.auth.Login(ctx, addr, pw, "203.0.113.1", "ua")
	if err != nil {
		t.Fatal(err)
	}
	return tok.User.ID
}

// join admits userID to wsID as a member through the real invitation flow.
func (e *env) join(t *testing.T, ownerID, wsID, userID string) {
	t.Helper()
	ctx := context.Background()
	if _, err := e.ws.Invite(ctx, ownerID, wsID, userID+"@example.com", "member"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.ws.AcceptInvite(ctx, userID, tokenOf(e.mail.invite)); err != nil {
		t.Fatal(err)
	}
}

func TestExport_ContainsAccountMembershipsAndInvitationsButNoSecrets(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	ada := e.signUp(t, email)
	ws, _ := e.ws.Create(ctx, ada, "Acme", "")
	e.store.pending = []PendingInvitation{{WorkspaceName: "Zed Co", RoleKey: "admin", ExpiresAt: time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)}}

	data, name, err := e.svc.Export(ctx, ada)
	if err != nil {
		t.Fatal(err)
	}
	if name != "ignition-export-2026-10-02.json" {
		t.Errorf("filename = %q", name)
	}
	if e.store.invitesSeen != email {
		t.Errorf("invitations looked up for %q, want %q", e.store.invitesSeen, email)
	}
	var doc struct {
		Account     map[string]any   `json:"account"`
		Memberships []map[string]any `json:"memberships"`
		Invitations []map[string]any `json:"pending_invitations"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("export is not JSON: %v", err)
	}
	if doc.Account["email"] != email || doc.Account["id"] != ada {
		t.Errorf("account = %v", doc.Account)
	}
	if len(doc.Memberships) != 1 || doc.Memberships[0]["workspace_id"] != ws.ID || doc.Memberships[0]["role"] != "owner" || doc.Memberships[0]["owner"] != true {
		t.Errorf("memberships = %v", doc.Memberships)
	}
	if len(doc.Invitations) != 1 || doc.Invitations[0]["workspace_name"] != "Zed Co" {
		t.Errorf("invitations = %v", doc.Invitations)
	}
	for _, secret := range []string{"hash", "password", "token", "secret", pw} {
		if strings.Contains(strings.ToLower(string(data)), secret) {
			t.Errorf("export mentions %q:\n%s", secret, data)
		}
	}
}

func TestExport_UserWithNothingGetsEmptyListsNotNull(t *testing.T) {
	e := newEnv(t)
	ada := e.signUp(t, email)
	data, _, err := e.svc.Export(context.Background(), ada)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"memberships": []`) || !strings.Contains(string(data), `"pending_invitations": []`) {
		t.Errorf("want empty arrays:\n%s", data)
	}
}

func TestDelete_WrongPasswordChangesNothing(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	ada := e.signUp(t, email)
	ws, _ := e.ws.Create(ctx, ada, "Acme", "")

	err := e.svc.Delete(ctx, ada, "Wrong-Password-1!")
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
	if len(e.store.erased) != 0 {
		t.Fatalf("erased %v before the password was verified", e.store.erased)
	}
	if _, err := e.ws.Get(ctx, ada, ws.ID); err != nil {
		t.Fatalf("workspace should be untouched: %v", err)
	}
}

func TestDelete_RefusesWhileSoleOwnerOfAWorkspaceWithOtherMembers(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	ada := e.signUp(t, email)
	bob := e.signUp(t, "bob@example.com")
	shared, _ := e.ws.Create(ctx, ada, "Shared", "")
	alone, _ := e.ws.Create(ctx, ada, "Alone", "")
	e.join(t, ada, shared.ID, bob)

	err := e.svc.Delete(ctx, ada, pw)
	if !errors.Is(err, ErrOwnsSharedWorkspace) {
		t.Fatalf("err = %v, want ErrOwnsSharedWorkspace", err)
	}
	if len(e.store.erased) != 0 || len(e.store.invDeleted) != 0 {
		t.Fatalf("a refusal must not remove anything: erased %v", e.store.erased)
	}
	if _, err := e.auth.User(ctx, ada); err != nil {
		t.Fatalf("account should still exist: %v", err)
	}
	if _, err := e.ws.Get(ctx, ada, alone.ID); err != nil {
		t.Fatalf("the unshared workspace should survive a refusal: %v", err)
	}
}

func TestDelete_ErasesOwnedWorkspacesLeavesJoinedOnesAndRemovesTheAccount(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	ada := e.signUp(t, email)
	bob := e.signUp(t, "bob@example.com")
	alone, _ := e.ws.Create(ctx, ada, "Alone", "")
	bobs, _ := e.ws.Create(ctx, bob, "Bobs", "")
	e.join(t, bob, bobs.ID, ada)
	session, _ := e.auth.Login(ctx, email, pw, "203.0.113.1", "ua")

	if err := e.svc.Delete(ctx, ada, pw); err != nil {
		t.Fatal(err)
	}

	if len(e.store.erased) != 1 || e.store.erased[0] != alone.ID {
		t.Errorf("erased = %v, want only %s", e.store.erased, alone.ID)
	}
	if len(e.store.invDeleted) != 1 || e.store.invDeleted[0] != email {
		t.Errorf("invitations deleted for %v, want %s", e.store.invDeleted, email)
	}
	members, err := e.ws.ListMembers(ctx, bob, bobs.ID)
	if err != nil || len(members) != 1 || members[0].UserID != bob {
		t.Errorf("Bob's workspace members = %v, %v; want only Bob", members, err)
	}
	if _, err := e.auth.User(ctx, ada); !errors.Is(err, auth.ErrAccountNotFound) {
		t.Errorf("User after delete: %v, want ErrAccountNotFound", err)
	}
	if _, err := e.auth.Refresh(ctx, session.RefreshToken); !errors.Is(err, auth.ErrTokenInvalid) {
		t.Errorf("session survived the deletion: %v", err)
	}
}

func TestDelete_FailedErasureKeepsTheAccountSoItCanBeRetried(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	ada := e.signUp(t, email)
	_, _ = e.ws.Create(ctx, ada, "Alone", "")
	e.store.eraseErr = errors.New("db down")

	if err := e.svc.Delete(ctx, ada, pw); err == nil {
		t.Fatal("want the erase failure")
	}
	if _, err := e.auth.User(ctx, ada); err != nil {
		t.Fatalf("account must survive a failed erasure: %v", err)
	}
	e.store.eraseErr = nil
	if err := e.svc.Delete(ctx, ada, pw); err != nil {
		t.Fatalf("retry: %v", err)
	}
}

func TestMe_ReturnsTheAccountAndNotFoundOnceDeleted(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	ada := e.signUp(t, email)
	u, err := e.svc.Me(ctx, ada)
	if err != nil || u.Email != email {
		t.Fatalf("Me = %+v, %v", u, err)
	}
	if err := e.svc.Delete(ctx, ada, pw); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Me(ctx, ada); !errors.Is(err, auth.ErrAccountNotFound) {
		t.Fatalf("Me after delete: %v", err)
	}
}

func TestDelete_RefusesWhileAnOwnedWorkspaceIsStillBilledAndRemovesNothing(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	ada := e.signUp(t, email)
	free, _ := e.ws.Create(ctx, ada, "Free", "")
	paid, _ := e.ws.Create(ctx, ada, "Paid", "")
	e.store.canErase = map[string]error{paid.ID: ErrActiveSubscription}

	if err := e.svc.Delete(ctx, ada, pw); !errors.Is(err, ErrActiveSubscription) {
		t.Fatalf("err = %v, want ErrActiveSubscription", err)
	}
	if len(e.store.erased) != 0 {
		t.Fatalf("erased %v although the deletion was refused", e.store.erased)
	}
	if _, err := e.ws.Get(ctx, ada, free.ID); err != nil {
		t.Fatalf("free workspace should survive a refusal: %v", err)
	}
}
