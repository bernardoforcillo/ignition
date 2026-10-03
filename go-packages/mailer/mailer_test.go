package mailer_test

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/bernardoforcillo/ignition/go-packages/mailer"
	"github.com/bernardoforcillo/ignition/go-packages/mailer/mailertest"
)

func newMailer(t *testing.T) (*mailer.Mailer, *mailertest.Recorder) {
	t.Helper()
	rec := &mailertest.Recorder{}
	m, err := mailer.New(rec, mailer.Config{
		From:        "Ignition <hello@example.com>",
		ReplyTo:     "support@example.com",
		CompanyName: "Ignition",
		AppURL:      "https://app.example.com/",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return m, rec
}

func TestSendVerification_FillsLinkInHTMLAndText(t *testing.T) {
	m, rec := newMailer(t)
	link := "https://app.example.com/verify?token=abc&next=%2Fhome"

	if err := m.SendVerification(context.Background(), "ada@example.com", link); err != nil {
		t.Fatal(err)
	}

	msgs := rec.Messages()
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	got := msgs[0]
	if got.Subject != "Confirm your email address" {
		t.Errorf("subject = %q", got.Subject)
	}
	if got.From != "Ignition <hello@example.com>" || got.ReplyTo != "support@example.com" {
		t.Errorf("from/reply-to = %q / %q", got.From, got.ReplyTo)
	}
	if !strings.Contains(got.HTML, `href="https://app.example.com/verify?token=abc&amp;next=%2Fhome"`) {
		t.Errorf("html does not carry the escaped link")
	}
	if !strings.Contains(got.Text, link) {
		t.Errorf("text does not carry the raw link")
	}
	if strings.Contains(got.HTML, "{{") || strings.Contains(got.Text, "{{") {
		t.Errorf("unrendered placeholder left in the body")
	}
}

func TestSendPasswordReset_FillsLinkInHTMLAndText(t *testing.T) {
	m, rec := newMailer(t)
	link := "https://app.example.com/reset-password?token=abc&x=1"

	if err := m.SendPasswordReset(context.Background(), "ada@example.com", link); err != nil {
		t.Fatal(err)
	}

	msgs := rec.Messages()
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	got := msgs[0]
	if got.Subject != "Reset your password" || got.To[0] != "ada@example.com" {
		t.Errorf("subject/to = %q / %v", got.Subject, got.To)
	}
	if !strings.Contains(got.HTML, `href="https://app.example.com/reset-password?token=abc&amp;x=1"`) {
		t.Errorf("html does not carry the escaped link")
	}
	if !strings.Contains(got.Text, link) {
		t.Errorf("text does not carry the raw link")
	}
}

func TestSendInvitation_EscapesWorkspaceNameInHTML(t *testing.T) {
	m, rec := newMailer(t)

	err := m.SendInvitation(context.Background(), "ada@example.com", `<script>alert(1)</script>`, "https://app.example.com/invite?token=x")
	if err != nil {
		t.Fatal(err)
	}

	got := rec.Messages()[0]
	if strings.Contains(got.HTML, "<script>") {
		t.Errorf("workspace name was not escaped in the html body")
	}
	if !strings.Contains(got.HTML, "&lt;script&gt;") {
		t.Errorf("escaped workspace name missing from html")
	}
}

func TestSendInvitation_SubjectIsSingleLine(t *testing.T) {
	m, rec := newMailer(t)

	if err := m.SendInvitation(context.Background(), "ada@example.com", "Acme\r\nBcc: evil@example.com", "https://app.example.com/i"); err != nil {
		t.Fatal(err)
	}

	if subject := rec.Messages()[0].Subject; strings.ContainsAny(subject, "\r\n") {
		t.Errorf("subject spans lines: %q", subject)
	}
}

func TestSendAccountExists_LinksToSignIn(t *testing.T) {
	m, rec := newMailer(t)

	if err := m.SendAccountExists(context.Background(), "ada@example.com"); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(rec.Messages()[0].Text, "https://app.example.com/login") {
		t.Errorf("sign-in link missing: %q", rec.Messages()[0].Text)
	}
}

func TestSendTemplate_Errors(t *testing.T) {
	m, rec := newMailer(t)
	ctx := context.Background()
	sendErr := errors.New("provider down")

	tests := []struct {
		name string
		run  func() error
		want error
	}{
		{"unknown template", func() error { return m.SendTemplate(ctx, "nope", "a@b.co", nil) }, mailer.ErrUnknownTemplate},
		{"missing variable", func() error {
			return m.SendTemplate(ctx, "subscription-update", "a@b.co", map[string]string{"Url": "https://x.co/a"})
		}, mailer.ErrMissingVariable},
		{"javascript link", func() error { return m.SendVerification(ctx, "a@b.co", "javascript:alert(1)") }, mailer.ErrInvalidURL},
		{"relative link", func() error { return m.SendVerification(ctx, "a@b.co", "/verify") }, mailer.ErrInvalidURL},
		{"provider failure", func() error {
			rec.Err = sendErr
			defer func() { rec.Err = nil }()
			return m.SendVerification(ctx, "a@b.co", "https://app.example.com/v")
		}, sendErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
	if n := len(rec.Messages()); n != 0 {
		t.Errorf("%d messages sent despite errors", n)
	}
}

func TestSendTemplate_RejectsEmptyRecipient(t *testing.T) {
	m, _ := newMailer(t)

	if err := m.SendVerification(context.Background(), " ", "https://app.example.com/v"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestNew_ValidatesConfig(t *testing.T) {
	rec := &mailertest.Recorder{}
	tests := []struct {
		name   string
		sender mailer.Sender
		cfg    mailer.Config
	}{
		{"nil sender", nil, mailer.Config{From: "a@b.co", CompanyName: "X", AppURL: "https://x.co"}},
		{"empty from", rec, mailer.Config{CompanyName: "X", AppURL: "https://x.co"}},
		{"empty company", rec, mailer.Config{From: "a@b.co", AppURL: "https://x.co"}},
		{"bad app url", rec, mailer.Config{From: "a@b.co", CompanyName: "X", AppURL: "app.example.com"}},
		{"bad asset url", rec, mailer.Config{From: "a@b.co", CompanyName: "X", AppURL: "https://x.co", AssetBaseURL: "cdn"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := mailer.New(tt.sender, tt.cfg, nil); !errors.Is(err, mailer.ErrInvalidConfig) {
				t.Errorf("err = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

// Every template the manifest lists must render with exactly its declared variables, so a
// react.email change that adds a variable without a Go caller fails here rather than in production.
func TestEveryTemplateRendersWithItsDeclaredVariables(t *testing.T) {
	templates, err := mailer.LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if len(templates.Names()) == 0 {
		t.Fatal("no templates embedded")
	}
	for _, name := range templates.Names() {
		t.Run(name, func(t *testing.T) {
			vars, err := templates.Variables(name)
			if err != nil {
				t.Fatal(err)
			}
			data := map[string]string{}
			for _, v := range vars {
				data[v] = "https://app.example.com/" + v
			}
			r, err := templates.Render(name, data)
			if err != nil {
				t.Fatal(err)
			}
			if r.Subject == "" || r.HTML == "" || r.Text == "" {
				t.Errorf("empty part in %+v", r)
			}
		})
	}
}

func TestEmbeddedTemplatesAreFiles(t *testing.T) {
	templates, _ := mailer.LoadTemplates()
	for _, name := range templates.Names() {
		for _, ext := range []string{".html", ".txt"} {
			if _, err := fs.Stat(mailer.TemplateFS(), "templates/"+name+ext); err != nil {
				t.Errorf("%s%s: %v", name, ext, err)
			}
		}
	}
}

// *Mailer must keep satisfying the ports go-packages/identity declares (auth.Mailer and
// workspace.Mailer). They are repeated here because the modules do not import each other.
var (
	_ interface {
		SendVerification(ctx context.Context, to, link string) error
		SendAccountExists(ctx context.Context, to string) error
	} = (*mailer.Mailer)(nil)
	_ interface {
		SendInvitation(ctx context.Context, to, workspaceName, link string) error
	} = (*mailer.Mailer)(nil)
)

func TestSendTrialEnding_FillsWorkspaceDateAndLink(t *testing.T) {
	m, rec := newMailer(t)

	err := m.SendTrialEnding(context.Background(), "ada@example.com", `Acme <b>`, "12 October 2026", "https://app.example.com/app/billing")
	if err != nil {
		t.Fatal(err)
	}

	msgs := rec.Messages()
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	got := msgs[0]
	if got.To[0] != "ada@example.com" || !strings.Contains(got.Subject, "trial is ending soon") {
		t.Errorf("to/subject = %v / %q", got.To, got.Subject)
	}
	if !strings.Contains(got.HTML, "12 October 2026") || !strings.Contains(got.HTML, `href="https://app.example.com/app/billing"`) {
		t.Errorf("html lacks the date or billing link")
	}
	if strings.Contains(got.HTML, "Acme <b>") {
		t.Errorf("workspace name is not escaped in html")
	}
	if !strings.Contains(got.Text, "https://app.example.com/app/billing") {
		t.Errorf("text lacks the billing link")
	}
}
