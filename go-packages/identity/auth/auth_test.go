package auth

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	alauth "github.com/bernardoforcillo/authlayer/auth"
	"github.com/bernardoforcillo/authlayer/password"
	"github.com/bernardoforcillo/authlayer/store/memory"
	"golang.org/x/crypto/bcrypt"
)

const (
	testPassword = "Correct-Horse-9!x"
	testEmail    = "ada@example.com"
	testIP       = "203.0.113.7"
)

type fakeMailer struct {
	verifications []mail
	existing      []string
	err           error
}

type mail struct{ to, link string }

func (m *fakeMailer) SendVerification(_ context.Context, to, link string) error {
	m.verifications = append(m.verifications, mail{to, link})
	return m.err
}

func (m *fakeMailer) SendAccountExists(_ context.Context, to string) error {
	m.existing = append(m.existing, to)
	return nil
}

// token pulls the token query parameter out of the last verification link.
func (m *fakeMailer) token(t *testing.T) string {
	t.Helper()
	if len(m.verifications) == 0 {
		t.Fatal("no verification email was sent")
	}
	u, err := url.Parse(m.verifications[len(m.verifications)-1].link)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query().Get("token")
}

type fakeLimiter struct {
	allow bool
	err   error
	keys  []string
}

func (l *fakeLimiter) Allow(_ context.Context, key string, _ int64, _ time.Duration) (bool, error) {
	l.keys = append(l.keys, key)
	return l.allow, l.err
}

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

func newTestService(t *testing.T, rl RateLimiter) (*Service, *fakeMailer, *clock) {
	t.Helper()
	m := &fakeMailer{}
	c := &clock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	s, err := NewService(memory.NewAuthStore(), m, rl, Config{
		Secret:     []byte("0123456789abcdef0123456789abcdef"),
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 24 * time.Hour,
		BaseURL:    "https://app.test/",
	}, alauth.WithClock(c.Now), alauth.WithHasher(password.Bcrypt(bcrypt.MinCost))) // fast hashing for tests
	if err != nil {
		t.Fatal(err)
	}
	return s, m, c
}

// verifiedAccount signs up and verifies testEmail.
func verifiedAccount(t *testing.T, s *Service, m *fakeMailer) {
	t.Helper()
	ctx := context.Background()
	if err := s.SignUp(ctx, testEmail, testPassword, testIP); err != nil {
		t.Fatal(err)
	}
	if err := s.VerifyEmail(ctx, m.token(t)); err != nil {
		t.Fatal(err)
	}
}

func TestNewService_RejectsBadConfig(t *testing.T) {
	good := Config{Secret: make([]byte, 32), AccessTTL: time.Minute}
	short := good
	short.Secret = []byte("short")
	tests := []struct {
		name   string
		cfg    Config
		mailer Mailer
	}{
		{"short secret", short, &fakeMailer{}},
		{"nil mailer", good, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewService(memory.NewAuthStore(), tt.mailer, nil, tt.cfg); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("err = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

func TestLifecycle_SignUpVerifyLoginRefreshLogout(t *testing.T) {
	ctx := context.Background()
	s, m, _ := newTestService(t, nil)

	if err := s.SignUp(ctx, "  Ada@Example.com ", testPassword, testIP); err != nil {
		t.Fatalf("SignUp: %v", err)
	}
	if len(m.verifications) != 1 || m.verifications[0].to != testEmail {
		t.Fatalf("verification mails = %+v, want one to %s", m.verifications, testEmail)
	}
	if got := m.verifications[0].link; len(got) < 30 || got[:30] != "https://app.test/verify-email?" {
		t.Fatalf("link = %q, want base URL joined without a double slash", got)
	}
	if err := s.VerifyEmail(ctx, m.token(t)); err != nil {
		t.Fatalf("VerifyEmail: %v", err)
	}

	first, err := s.Login(ctx, testEmail, testPassword, testIP, "test-agent")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if first.User.Email != testEmail || first.User.VerifiedAt == nil {
		t.Fatalf("user = %+v, want verified %s", first.User, testEmail)
	}
	claims, err := s.VerifyAccessToken(first.AccessToken)
	if err != nil || claims.Subject != first.User.ID {
		t.Fatalf("VerifyAccessToken = %+v, %v; want subject %s", claims, err, first.User.ID)
	}

	second, err := s.Refresh(ctx, first.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if second.RefreshToken == first.RefreshToken {
		t.Fatal("refresh token was not rotated")
	}

	if err := s.Logout(ctx, second.RefreshToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := s.Refresh(ctx, second.RefreshToken); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("Refresh after logout: err = %v, want ErrTokenInvalid", err)
	}
	if err := s.Logout(ctx, second.RefreshToken); err != nil {
		t.Fatalf("second Logout should be a no-op, got %v", err)
	}
}

func TestRefresh_ReplayedTokenRevokesTheFamily(t *testing.T) {
	ctx := context.Background()
	s, m, _ := newTestService(t, nil)
	verifiedAccount(t, s, m)

	first, _ := s.Login(ctx, testEmail, testPassword, testIP, "ua")
	second, err := s.Refresh(ctx, first.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Refresh(ctx, first.RefreshToken); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("replay: err = %v, want ErrTokenInvalid", err)
	}
	if _, err := s.Refresh(ctx, second.RefreshToken); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("successor after replay: err = %v, want revoked", err)
	}
}

func TestRefresh_ExpiredSessionIsRejected(t *testing.T) {
	ctx := context.Background()
	s, m, c := newTestService(t, nil)
	verifiedAccount(t, s, m)
	tok, _ := s.Login(ctx, testEmail, testPassword, testIP, "ua")

	c.now = c.now.Add(25 * time.Hour)
	if _, err := s.Refresh(ctx, tok.RefreshToken); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("err = %v, want ErrTokenInvalid", err)
	}
}

func TestVerifyAccessToken_GarbageAndTamperedAreInvalid(t *testing.T) {
	ctx := context.Background()
	s, m, _ := newTestService(t, nil)
	verifiedAccount(t, s, m)
	tok, _ := s.Login(ctx, testEmail, testPassword, testIP, "ua")

	if _, err := s.VerifyAccessToken("not-a-jwt"); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("garbage: err = %v", err)
	}
	if _, err := s.VerifyAccessToken(tok.AccessToken + "x"); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("tampered: err = %v", err)
	}
}

func TestLogin_Failures(t *testing.T) {
	ctx := context.Background()
	s, m, _ := newTestService(t, nil)
	if err := s.SignUp(ctx, testEmail, testPassword, testIP); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name            string
		email, password string
		want            error
	}{
		{"unverified account", testEmail, testPassword, ErrEmailNotVerified},
		{"unknown email", "nobody@example.com", testPassword, ErrInvalidCredentials},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := s.Login(ctx, tt.email, tt.password, testIP, "ua"); !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}

	if err := s.VerifyEmail(ctx, m.token(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Login(ctx, testEmail, "Wrong-Password-1!", testIP, "ua"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password: err = %v, want ErrInvalidCredentials", err)
	}
}

func TestVerifyEmail_ExpiredOrUnknownTokenIsInvalid(t *testing.T) {
	ctx := context.Background()
	s, m, c := newTestService(t, nil)
	if err := s.SignUp(ctx, testEmail, testPassword, testIP); err != nil {
		t.Fatal(err)
	}
	tok := m.token(t)

	if err := s.VerifyEmail(ctx, "bogus"); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("unknown token: err = %v", err)
	}
	c.now = c.now.Add(48 * time.Hour)
	if err := s.VerifyEmail(ctx, tok); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("expired token: err = %v", err)
	}
}

func TestSignUp_WeakPasswordIsRejected(t *testing.T) {
	s, m, _ := newTestService(t, nil)
	err := s.SignUp(context.Background(), testEmail, "short", testIP)
	if !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("err = %v, want ErrWeakPassword", err)
	}
	if len(m.verifications) != 0 {
		t.Fatal("no mail should be sent for a rejected sign-up")
	}
}

func TestSignUp_DuplicateAddressLooksIdenticalToTheCaller(t *testing.T) {
	ctx := context.Background()
	s, m, _ := newTestService(t, nil)
	verifiedAccount(t, s, m)

	if err := s.SignUp(ctx, testEmail, testPassword, testIP); err != nil {
		t.Fatalf("duplicate sign-up must not error, got %v", err)
	}
	if len(m.existing) != 1 || m.existing[0] != testEmail {
		t.Fatalf("account-exists mails = %v, want one to %s", m.existing, testEmail)
	}
	if len(m.verifications) != 1 {
		t.Fatalf("verification mails = %d, want still 1", len(m.verifications))
	}
}

func TestRateLimit(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		limiter *fakeLimiter
		wantErr error
	}{
		{"denied", &fakeLimiter{allow: false}, ErrRateLimited},
		{"limiter outage fails open", &fakeLimiter{err: errors.New("redis down")}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _, _ := newTestService(t, tt.limiter)
			err := s.SignUp(ctx, testEmail, testPassword, "")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("SignUp err = %v, want %v", err, tt.wantErr)
			}
			if tt.limiter.keys[0] != "signup:ip:unknown" {
				t.Fatalf("key = %q, want the unknown-IP bucket", tt.limiter.keys[0])
			}
		})
	}

	s, _, _ := newTestService(t, &fakeLimiter{allow: false})
	if _, err := s.Login(ctx, testEmail, testPassword, testIP, "ua"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("Login err = %v, want ErrRateLimited", err)
	}
}

func TestLogoutAll_RevokesEverySession(t *testing.T) {
	ctx := context.Background()
	s, m, _ := newTestService(t, nil)
	verifiedAccount(t, s, m)
	a, _ := s.Login(ctx, testEmail, testPassword, testIP, "phone")
	b, _ := s.Login(ctx, testEmail, testPassword, testIP, "laptop")

	if err := s.LogoutAll(ctx, a.User.ID); err != nil {
		t.Fatal(err)
	}
	for _, tok := range []string{a.RefreshToken, b.RefreshToken} {
		if _, err := s.Refresh(ctx, tok); !errors.Is(err, ErrTokenInvalid) {
			t.Fatalf("err = %v, want ErrTokenInvalid", err)
		}
	}
}
