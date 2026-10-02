package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

const newPassword = "Brand-New-Horse-7!z"

// keyLimiter denies every key that starts with deny and records what it was asked.
type keyLimiter struct {
	deny string
	keys []string
}

func (l *keyLimiter) Allow(_ context.Context, key string, _ int64, _ time.Duration) (bool, error) {
	l.keys = append(l.keys, key)
	return l.deny == "" || !strings.HasPrefix(key, l.deny), nil
}

func TestRequestPasswordReset_KnownAddressIsMailedAResetLink(t *testing.T) {
	ctx := context.Background()
	s, m, _ := newTestService(t, nil)
	verifiedAccount(t, s, m)

	if err := s.RequestPasswordReset(ctx, "  Ada@Example.com ", testIP); err != nil {
		t.Fatal(err)
	}
	if len(m.resets) != 1 || m.resets[0].to != testEmail {
		t.Fatalf("reset mails = %+v, want one to %s", m.resets, testEmail)
	}
	if !strings.HasPrefix(m.resets[0].link, "https://app.test/reset-password?token=") {
		t.Fatalf("link = %q", m.resets[0].link)
	}
}

func TestRequestPasswordReset_UnknownAddressAnswersTheSameAndSendsNothing(t *testing.T) {
	s, m, _ := newTestService(t, nil)
	verifiedAccount(t, s, m)

	if err := s.RequestPasswordReset(context.Background(), "nobody@example.com", testIP); err != nil {
		t.Fatalf("unknown address must not error, got %v", err)
	}
	if len(m.resets) != 0 {
		t.Fatalf("mails = %v, want none", m.resets)
	}
}

func TestRequestPasswordReset_MailerFailureIsNotReportedToTheCaller(t *testing.T) {
	s, m, _ := newTestService(t, nil)
	verifiedAccount(t, s, m)
	m.err = errors.New("resend is down")

	if err := s.RequestPasswordReset(context.Background(), testEmail, testIP); err != nil {
		t.Fatalf("err = %v, want nil: a send failure would reveal the address exists", err)
	}
}

func TestRequestPasswordReset_IPBudgetIsReportedAndAddressBudgetIsSilent(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name      string
		deny      string
		wantErr   error
		wantMails int
	}{
		{"per-IP budget spent", "reset:ip:", ErrRateLimited, 0},
		{"per-address budget spent", "reset:email:", nil, 0},
		{"no budget spent", "", nil, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, m, _ := newTestService(t, &keyLimiter{deny: tt.deny})
			verifiedAccount(t, s, m)
			err := s.RequestPasswordReset(ctx, testEmail, testIP)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if len(m.resets) != tt.wantMails {
				t.Fatalf("mails = %d, want %d", len(m.resets), tt.wantMails)
			}
		})
	}
}

func TestRequestPasswordReset_BucketsAreKeyedByIPAndNormalizedAddress(t *testing.T) {
	l := &keyLimiter{}
	s, _, _ := newTestService(t, l)

	if err := s.RequestPasswordReset(context.Background(), " Ada@Example.com", ""); err != nil {
		t.Fatal(err)
	}
	want := []string{"reset:ip:unknown", "reset:email:ada@example.com"}
	if len(l.keys) < 2 || l.keys[0] != want[0] || l.keys[1] != want[1] {
		t.Fatalf("keys = %v, want %v", l.keys, want)
	}
}

func TestResetPassword_NewPasswordWorksOldDoesNot(t *testing.T) {
	ctx := context.Background()
	s, m, _ := newTestService(t, nil)
	verifiedAccount(t, s, m)
	_ = s.RequestPasswordReset(ctx, testEmail, testIP)

	if err := s.ResetPassword(ctx, m.resetToken(t), newPassword); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Login(ctx, testEmail, testPassword, testIP, "ua"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password: err = %v, want ErrInvalidCredentials", err)
	}
	if _, err := s.Login(ctx, testEmail, newPassword, testIP, "ua"); err != nil {
		t.Fatalf("new password: %v", err)
	}
}

func TestResetPassword_TokenIsSingleUse(t *testing.T) {
	ctx := context.Background()
	s, m, _ := newTestService(t, nil)
	verifiedAccount(t, s, m)
	_ = s.RequestPasswordReset(ctx, testEmail, testIP)
	tok := m.resetToken(t)

	if err := s.ResetPassword(ctx, tok, newPassword); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetPassword(ctx, tok, "Another-Horse-Staple-3!"); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("second use: err = %v, want ErrTokenInvalid", err)
	}
}

func TestResetPassword_ExpiredOrUnknownTokenIsInvalid(t *testing.T) {
	ctx := context.Background()
	s, m, c := newTestService(t, nil)
	verifiedAccount(t, s, m)
	_ = s.RequestPasswordReset(ctx, testEmail, testIP)
	tok := m.resetToken(t)

	if err := s.ResetPassword(ctx, "bogus", newPassword); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("unknown token: err = %v", err)
	}
	c.now = c.now.Add(25 * time.Hour)
	if err := s.ResetPassword(ctx, tok, newPassword); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("expired token: err = %v", err)
	}
}

func TestResetPassword_VerificationTokenCannotResetAPassword(t *testing.T) {
	ctx := context.Background()
	s, m, _ := newTestService(t, nil)
	if err := s.SignUp(ctx, testEmail, testPassword, testIP); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetPassword(ctx, m.token(t), newPassword); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("err = %v, want ErrTokenInvalid", err)
	}
}

func TestResetPassword_WeakPasswordIsRefusedWithoutSpendingTheToken(t *testing.T) {
	ctx := context.Background()
	s, m, _ := newTestService(t, nil)
	verifiedAccount(t, s, m)
	_ = s.RequestPasswordReset(ctx, testEmail, testIP)
	tok := m.resetToken(t)

	if err := s.ResetPassword(ctx, tok, "short"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("err = %v, want ErrWeakPassword", err)
	}
	if err := s.ResetPassword(ctx, tok, newPassword); err != nil {
		t.Fatalf("retry with a strong password: %v", err)
	}
}

func TestResetPassword_RevokesEverySession(t *testing.T) {
	ctx := context.Background()
	s, m, _ := newTestService(t, nil)
	verifiedAccount(t, s, m)
	phone, _ := s.Login(ctx, testEmail, testPassword, testIP, "phone")
	laptop, _ := s.Login(ctx, testEmail, testPassword, testIP, "laptop")
	_ = s.RequestPasswordReset(ctx, testEmail, testIP)

	if err := s.ResetPassword(ctx, m.resetToken(t), newPassword); err != nil {
		t.Fatal(err)
	}
	for _, tok := range []string{phone.RefreshToken, laptop.RefreshToken} {
		if _, err := s.Refresh(ctx, tok); !errors.Is(err, ErrTokenInvalid) {
			t.Fatalf("refresh after reset: err = %v, want ErrTokenInvalid", err)
		}
	}
}

func TestRequestPasswordReset_SecondRequestInvalidatesTheFirstLink(t *testing.T) {
	ctx := context.Background()
	s, m, _ := newTestService(t, nil)
	verifiedAccount(t, s, m)
	_ = s.RequestPasswordReset(ctx, testEmail, testIP)
	first := m.resetToken(t)
	_ = s.RequestPasswordReset(ctx, testEmail, testIP)

	if err := s.ResetPassword(ctx, first, newPassword); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("first link after a second request: err = %v, want ErrTokenInvalid", err)
	}
	if err := s.ResetPassword(ctx, m.resetToken(t), newPassword); err != nil {
		t.Fatalf("latest link: %v", err)
	}
}

func TestDeleteAccount(t *testing.T) {
	ctx := context.Background()

	t.Run("wrong password runs no cleanup and keeps the account", func(t *testing.T) {
		s, m, _ := newTestService(t, nil)
		verifiedAccount(t, s, m)
		tok, _ := s.Login(ctx, testEmail, testPassword, testIP, "ua")
		ran := false
		err := s.DeleteAccount(ctx, tok.User.ID, "Wrong-Password-1!", func(context.Context, string) error { ran = true; return nil })
		if !errors.Is(err, ErrInvalidCredentials) || ran {
			t.Fatalf("err = %v, cleanup ran = %v; want ErrInvalidCredentials and no cleanup", err, ran)
		}
		if _, err := s.User(ctx, tok.User.ID); err != nil {
			t.Fatalf("account should still exist: %v", err)
		}
	})

	t.Run("failing cleanup aborts and keeps the account and its sessions", func(t *testing.T) {
		s, m, _ := newTestService(t, nil)
		verifiedAccount(t, s, m)
		tok, _ := s.Login(ctx, testEmail, testPassword, testIP, "ua")
		boom := errors.New("boom")
		if err := s.DeleteAccount(ctx, tok.User.ID, testPassword, func(context.Context, string) error { return boom }); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want the cleanup error", err)
		}
		if _, err := s.Refresh(ctx, tok.RefreshToken); err != nil {
			t.Fatalf("session should survive an aborted deletion: %v", err)
		}
	})

	t.Run("success removes the account, frees the address and revokes sessions", func(t *testing.T) {
		s, m, _ := newTestService(t, nil)
		verifiedAccount(t, s, m)
		tok, _ := s.Login(ctx, testEmail, testPassword, testIP, "ua")
		var cleaned string
		if err := s.DeleteAccount(ctx, tok.User.ID, testPassword, func(_ context.Context, id string) error { cleaned = id; return nil }); err != nil {
			t.Fatal(err)
		}
		if cleaned != tok.User.ID {
			t.Fatalf("cleanup got %q, want %q", cleaned, tok.User.ID)
		}
		if _, err := s.User(ctx, tok.User.ID); !errors.Is(err, ErrAccountNotFound) {
			t.Fatalf("User after delete: err = %v, want ErrAccountNotFound", err)
		}
		if _, err := s.Refresh(ctx, tok.RefreshToken); !errors.Is(err, ErrTokenInvalid) {
			t.Fatalf("Refresh after delete: err = %v, want ErrTokenInvalid", err)
		}
		if _, err := s.Login(ctx, testEmail, testPassword, testIP, "ua"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("Login after delete: err = %v, want ErrInvalidCredentials", err)
		}
	})
}
