// Package mailer sends the product's transactional email.
//
// Templates are written in React with react.email (packages/mailer) and exported to static HTML
// that this package embeds, so Go services render and send email without a Node runtime. Delivery
// goes through a Sender; the Resend implementation lives in the resend subpackage.
//
// *Mailer satisfies the Mailer ports of go-packages/identity (auth and workspace) structurally.
package mailer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

// Message is one email, already rendered.
type Message struct {
	From    string
	To      []string
	ReplyTo string
	Subject string
	HTML    string
	Text    string
	// IdempotencyKey lets a provider drop a retried send of the same message.
	IdempotencyKey string
}

// Sender delivers a rendered message. Implementations: resend.Client, LogSender, and
// mailertest.Recorder in tests.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// ErrInvalidConfig is returned by New for an unusable Config.
var ErrInvalidConfig = errors.New("mailer: invalid config")

// Config is the product-level mailer configuration.
type Config struct {
	// From is the default sender, e.g. `Ignition <hello@example.com>`.
	From    string
	ReplyTo string
	// CompanyName is shown in every template's header and copy.
	CompanyName string
	// AppURL is the public base URL of the web app, used for links the mailer builds itself
	// (the sign-in link in the account-exists email). No trailing slash required.
	AppURL string
	// AssetBaseURL is the origin the email images are served from (`<origin>/static/...`, see
	// packages/mailer/src/emails/static). Defaults to AppURL.
	AssetBaseURL string
}

// Mailer renders templates and sends them through a Sender.
type Mailer struct {
	sender    Sender
	templates *Templates
	cfg       Config
	log       *slog.Logger
}

// New builds a Mailer over the embedded templates.
func New(sender Sender, cfg Config, logger *slog.Logger) (*Mailer, error) {
	if sender == nil {
		return nil, fmt.Errorf("%w: nil sender", ErrInvalidConfig)
	}
	if strings.TrimSpace(cfg.From) == "" {
		return nil, fmt.Errorf("%w: empty from address", ErrInvalidConfig)
	}
	if err := validateURL(cfg.AppURL); err != nil {
		return nil, fmt.Errorf("%w: AppURL must be an absolute http(s) URL", ErrInvalidConfig)
	}
	templates, err := LoadTemplates()
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = slog.Default()
	}
	cfg.AppURL = strings.TrimRight(cfg.AppURL, "/")
	if cfg.AssetBaseURL == "" {
		cfg.AssetBaseURL = cfg.AppURL
	}
	if err := validateURL(cfg.AssetBaseURL); err != nil {
		return nil, fmt.Errorf("%w: AssetBaseURL must be an absolute http(s) URL", ErrInvalidConfig)
	}
	cfg.AssetBaseURL = strings.TrimRight(cfg.AssetBaseURL, "/")
	if strings.TrimSpace(cfg.CompanyName) == "" {
		return nil, fmt.Errorf("%w: empty company name", ErrInvalidConfig)
	}
	return &Mailer{sender: sender, templates: templates, cfg: cfg, log: logger}, nil
}

// SendTemplate renders a template and sends it to one recipient.
func (m *Mailer) SendTemplate(ctx context.Context, name, to string, data map[string]string) error {
	return m.send(ctx, name, to, data, "")
}

// SendTemplateOnce is SendTemplate with an idempotency key, for callers that retry.
func (m *Mailer) SendTemplateOnce(ctx context.Context, name, to string, data map[string]string, key string) error {
	return m.send(ctx, name, to, data, key)
}

func (m *Mailer) send(ctx context.Context, name, to string, data map[string]string, key string) error {
	if strings.TrimSpace(to) == "" {
		return errors.New("mailer: empty recipient")
	}
	// The ambient variables are the mailer's, not the caller's: a caller cannot override them.
	merged := make(map[string]string, len(data)+2)
	for k, v := range data {
		merged[k] = v
	}
	merged["CompanyName"] = m.cfg.CompanyName
	merged["AssetBaseUrl"] = m.cfg.AssetBaseURL
	r, err := m.templates.Render(name, merged)
	if err != nil {
		return err
	}
	msg := Message{
		From:           m.cfg.From,
		To:             []string{to},
		ReplyTo:        m.cfg.ReplyTo,
		Subject:        r.Subject,
		HTML:           r.HTML,
		Text:           r.Text,
		IdempotencyKey: key,
	}
	if err := m.sender.Send(ctx, msg); err != nil {
		return fmt.Errorf("mailer: send %s: %w", name, err)
	}
	m.log.DebugContext(ctx, "email sent", "template", name)
	return nil
}

// SendVerification delivers the link that confirms an address.
func (m *Mailer) SendVerification(ctx context.Context, to, link string) error {
	return m.SendTemplate(ctx, "activation", to, map[string]string{"Url": link})
}

// SendAccountExists tells the real holder of an address that someone tried to sign up with it.
func (m *Mailer) SendAccountExists(ctx context.Context, to string) error {
	return m.SendTemplate(ctx, "account-exists", to, map[string]string{"Url": m.cfg.AppURL + "/login"})
}

// SendInvitation delivers the accept link for an invitation to a workspace.
func (m *Mailer) SendInvitation(ctx context.Context, to, workspaceName, link string) error {
	return m.SendTemplate(ctx, "workspace-invitation", to, map[string]string{
		"WorkspaceName": workspaceName,
		"Url":           link,
	})
}

// LogSender writes each message to the log instead of sending it: the default for local
// development, where no provider key is configured. It never logs the body, which carries
// single-use links.
type LogSender struct{ log *slog.Logger }

// NewLogSender builds a LogSender.
func NewLogSender(logger *slog.Logger) *LogSender {
	if logger == nil {
		logger = slog.Default()
	}
	return &LogSender{log: logger}
}

// Send implements Sender.
func (s *LogSender) Send(ctx context.Context, msg Message) error {
	s.log.InfoContext(ctx, "email (not sent)", "to", msg.To, "subject", msg.Subject)
	return nil
}
