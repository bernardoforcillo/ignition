// Package auth is the template's authentication domain. The credential model,
// session families with refresh rotation, verification tokens and the
// timing/enumeration guards live in github.com/bernardoforcillo/authlayer/auth;
// this package owns what the product decides on top: rate-limit budgets, the
// mail links, the "unverified accounts cannot sign in" rule and the sentinel
// errors a transport layer maps to status codes.
package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	alauth "github.com/bernardoforcillo/authlayer/auth"
	"github.com/bernardoforcillo/authlayer/token"
)

// Per-IP budgets for the unauthenticated endpoints. Login is keyed by IP only,
// never by email, so an attacker cannot lock a victim out by exhausting an
// email bucket.
const (
	loginRateMax     = 20
	loginRateWindow  = 15 * time.Minute
	signupRateMax    = 10
	signupRateWindow = time.Hour

	minSecretLen = 32
	// unknownIP buckets callers whose address could not be resolved; authlayer
	// refuses an empty ip outright.
	unknownIP = "unknown"
)

// Sentinel errors for the transport layer. Match with errors.Is.
var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrEmailNotVerified   = errors.New("email not verified")
	ErrTokenInvalid       = errors.New("token expired or invalid")
	ErrWeakPassword       = errors.New("password does not meet requirements")
	ErrRateLimited        = errors.New("too many attempts; try again later")
	ErrInvalidConfig      = errors.New("invalid auth config")
)

// Mailer delivers the emails authentication needs. Implementations live in the
// product (SMTP, a provider API); this module ships none.
type Mailer interface {
	// SendVerification delivers the link that confirms an address.
	SendVerification(ctx context.Context, to, link string) error
	// SendAccountExists tells the real holder of an address that someone tried
	// to sign up with it, instead of telling the caller (enumeration safety).
	SendAccountExists(ctx context.Context, to string) error
}

// RateLimiter reports whether one more request under key is allowed within
// window, given a per-window maximum. A nil limiter allows everything.
type RateLimiter interface {
	Allow(ctx context.Context, key string, limit int64, window time.Duration) (bool, error)
}

// Config is the deployment-specific part of the service.
type Config struct {
	// Secret signs access tokens (HS256); at least 32 bytes, from a secret
	// manager, never from code.
	Secret     []byte
	AccessTTL  time.Duration
	RefreshTTL time.Duration
	// BaseURL prefixes the links in emails, e.g. "https://app.example.com".
	BaseURL string
}

// User is the account view the rest of the product renders.
type User struct {
	ID         string
	Email      string
	VerifiedAt *time.Time
}

// Tokens is the result of a sign-in or refresh. RefreshToken is the new
// plaintext refresh token; the previous one is dead after a refresh.
type Tokens struct {
	User         User
	AccessToken  string
	RefreshToken string
}

// Claims is the verified access-token payload: Subject is the user id and
// SessionID the refresh session it was minted for.
type Claims = token.Claims

// Service wraps authlayer's auth service with this product's rules.
type Service struct {
	al     *alauth.Service
	mailer Mailer
	rl     RateLimiter
	base   string
}

// NewService builds the service over an authlayer store (memory in tests,
// *dropsstore.AuthStore in production). opts are passed to authlayer last, so
// a caller can override a default (a test injects alauth.WithClock).
func NewService(store alauth.Store, mailer Mailer, rl RateLimiter, cfg Config, opts ...alauth.Option) (*Service, error) {
	if len(cfg.Secret) < minSecretLen {
		return nil, fmt.Errorf("%w: secret must be at least %d bytes", ErrInvalidConfig, minSecretLen)
	}
	if mailer == nil {
		return nil, fmt.Errorf("%w: mailer is required", ErrInvalidConfig)
	}
	base := []alauth.Option{
		alauth.WithJWT([][]byte{cfg.Secret}, cfg.AccessTTL),
		// authlayer's default is permissive; this product refuses unverified
		// accounts.
		alauth.WithRequireVerifiedEmail(true),
	}
	if cfg.RefreshTTL > 0 {
		base = append(base, alauth.WithRefreshTTL(cfg.RefreshTTL))
	}
	return &Service{
		al:     alauth.New(store, append(base, opts...)...),
		mailer: mailer,
		rl:     rl,
		base:   strings.TrimRight(cfg.BaseURL, "/"),
	}, nil
}

// Authlayer exposes the underlying service for capabilities this wrapper does
// not re-export (password reset, MFA, account deletion).
func (s *Service) Authlayer() *alauth.Service { return s.al }

// SignUp registers an account and mails a verification link. The outcome is
// the same for the caller whether or not the address was already registered;
// only the email differs.
func (s *Service) SignUp(ctx context.Context, email, password, clientIP string) error {
	if !s.allow(ctx, "signup:ip:"+orUnknown(clientIP), signupRateMax, signupRateWindow) {
		return ErrRateLimited
	}
	email = alauth.NormalizeEmail(email)
	res, err := s.al.SignUp(ctx, email, password)
	if err != nil {
		return mapError(err)
	}
	if !res.Created {
		return s.mailer.SendAccountExists(ctx, email)
	}
	return s.mailer.SendVerification(ctx, email, s.link("/verify-email", res.VerifyToken))
}

// VerifyEmail redeems the token from a verification email.
func (s *Service) VerifyEmail(ctx context.Context, token string) error {
	_, err := s.al.VerifyEmail(ctx, token)
	return mapError(err)
}

// Login checks the password and opens a session.
func (s *Service) Login(ctx context.Context, email, password, clientIP, userAgent string) (Tokens, error) {
	ip := orUnknown(clientIP)
	if !s.allow(ctx, "login:ip:"+ip, loginRateMax, loginRateWindow) {
		return Tokens{}, ErrRateLimited
	}
	res, err := s.al.Login(ctx, alauth.NormalizeEmail(email), password, ip, userAgent)
	if err != nil {
		return Tokens{}, mapError(err)
	}
	return tokensOf(res), nil
}

// Refresh rotates the refresh token. Presenting an already-rotated token is
// treated as theft: the whole session family is revoked and ErrTokenInvalid
// returned.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (Tokens, error) {
	res, err := s.al.Refresh(ctx, refreshToken)
	if err != nil {
		return Tokens{}, mapError(err)
	}
	return tokensOf(res), nil
}

// Logout revokes the session behind refreshToken. An unknown token is not an
// error: a double click or a stale tab is the ordinary cause.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	err := s.al.Logout(ctx, refreshToken)
	if errors.Is(err, alauth.ErrTokenInvalid) {
		return nil
	}
	return mapError(err)
}

// LogoutAll revokes every session of the user.
func (s *Service) LogoutAll(ctx context.Context, userID string) error {
	return mapError(s.al.LogoutAll(ctx, userID))
}

// VerifyAccessToken checks a bearer token's signature and expiry without
// touching the store; a revoked session's access token stays valid until it
// expires.
func (s *Service) VerifyAccessToken(raw string) (Claims, error) {
	claims, err := s.al.VerifyAccessToken(raw)
	if err != nil {
		return Claims{}, ErrTokenInvalid
	}
	return claims, nil
}

// allow fails open: a limiter outage degrades brute-force protection rather
// than locking every user out.
func (s *Service) allow(ctx context.Context, key string, limit int64, window time.Duration) bool {
	if s.rl == nil {
		return true
	}
	ok, err := s.rl.Allow(ctx, key, limit, window)
	if err != nil {
		slog.WarnContext(ctx, "auth rate limiter unavailable; allowing request", "error", err)
		return true
	}
	return ok
}

func (s *Service) link(path, token string) string {
	return s.base + path + "?token=" + url.QueryEscape(token)
}

func orUnknown(ip string) string {
	if ip == "" {
		return unknownIP
	}
	return ip
}

func tokensOf(res alauth.LoginResult) Tokens {
	return Tokens{
		User:         User{ID: res.User.ID, Email: res.User.Email, VerifiedAt: res.User.EmailVerifiedAt},
		AccessToken:  res.AccessToken,
		RefreshToken: res.RefreshToken,
	}
}

// mapError translates authlayer sentinels into this package's. Anything
// unrecognised (a store failure) passes through and should surface as internal.
func mapError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, alauth.ErrWeakPassword):
		return fmt.Errorf("%w: %s", ErrWeakPassword, strings.TrimPrefix(err.Error(), alauth.ErrWeakPassword.Error()+": "))
	case errors.Is(err, alauth.ErrInvalidCredentials):
		return ErrInvalidCredentials
	case errors.Is(err, alauth.ErrEmailNotVerified):
		return ErrEmailNotVerified
	case errors.Is(err, alauth.ErrRateLimited), errors.Is(err, alauth.ErrMissingIP):
		return ErrRateLimited
	case errors.Is(err, alauth.ErrTokenInvalid),
		errors.Is(err, alauth.ErrTokenReuse),
		errors.Is(err, alauth.ErrSessionRevoked),
		errors.Is(err, alauth.ErrSessionNotFound),
		errors.Is(err, alauth.ErrVerificationExpired),
		errors.Is(err, alauth.ErrVerificationPurpose),
		errors.Is(err, alauth.ErrVerificationNotFound):
		return ErrTokenInvalid
	default:
		return err
	}
}
