package mailer

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"sort"
	"strings"
	"text/template"
)

// templateFS holds the static HTML and plain-text bodies exported from the react.email templates
// in packages/mailer (`pnpm --filter @ignition/mailer export`). They are committed so a Go build
// needs no Node toolchain.
//
//go:embed templates
var templateFS embed.FS

// Sentinel errors a caller can match with errors.Is.
var (
	// ErrUnknownTemplate is returned for a template name the manifest does not list.
	ErrUnknownTemplate = errors.New("mailer: unknown template")
	// ErrMissingVariable is returned when the data lacks a variable the template declares.
	ErrMissingVariable = errors.New("mailer: missing template variable")
	// ErrInvalidURL is returned for a Link/Url variable that is not an absolute http(s) URL.
	ErrInvalidURL = errors.New("mailer: invalid url variable")
)

type manifestEntry struct {
	Subject   string   `json:"subject"`
	Variables []string `json:"variables"`
}

type compiled struct {
	subject   *template.Template
	html      *template.Template
	text      *template.Template
	variables []string
}

// Rendered is a template filled with data, ready to hand to a Sender.
type Rendered struct {
	Subject string
	HTML    string
	Text    string
}

// Templates renders the embedded email templates.
//
// The HTML uses text/template with values escaped by hand rather than html/template: html/template
// strips HTML comments, and the Outlook conditional comments react.email emits live in comments.
// URL variables are validated as absolute http(s) URLs first, so escaping alone is enough.
type Templates struct {
	byName map[string]compiled
}

// LoadTemplates parses every template listed in the embedded manifest.
func LoadTemplates() (*Templates, error) {
	raw, err := templateFS.ReadFile("templates/manifest.json")
	if err != nil {
		return nil, fmt.Errorf("mailer: read manifest: %w", err)
	}
	var manifest map[string]manifestEntry
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("mailer: parse manifest: %w", err)
	}

	t := &Templates{byName: make(map[string]compiled, len(manifest))}
	for name, entry := range manifest {
		c, err := compile(name, entry)
		if err != nil {
			return nil, err
		}
		t.byName[name] = c
	}
	return t, nil
}

func compile(name string, entry manifestEntry) (compiled, error) {
	parse := func(label, body string) (*template.Template, error) {
		tpl, err := template.New(name + "." + label).Option("missingkey=error").Parse(body)
		if err != nil {
			return nil, fmt.Errorf("mailer: parse %s.%s: %w", name, label, err)
		}
		return tpl, nil
	}
	htmlBody, err := templateFS.ReadFile("templates/" + name + ".html")
	if err != nil {
		return compiled{}, fmt.Errorf("mailer: read %s.html: %w", name, err)
	}
	textBody, err := templateFS.ReadFile("templates/" + name + ".txt")
	if err != nil {
		return compiled{}, fmt.Errorf("mailer: read %s.txt: %w", name, err)
	}

	c := compiled{variables: entry.Variables}
	if c.subject, err = parse("subject", entry.Subject); err != nil {
		return compiled{}, err
	}
	if c.html, err = parse("html", string(htmlBody)); err != nil {
		return compiled{}, err
	}
	if c.text, err = parse("text", string(textBody)); err != nil {
		return compiled{}, err
	}
	return c, nil
}

// Names lists the available templates, sorted.
func (t *Templates) Names() []string {
	names := make([]string, 0, len(t.byName))
	for name := range t.byName {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Variables lists the variables a template requires.
func (t *Templates) Variables(name string) ([]string, error) {
	c, ok := t.byName[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownTemplate, name)
	}
	return append([]string(nil), c.variables...), nil
}

// Render fills a template. Every declared variable must be present in data.
func (t *Templates) Render(name string, data map[string]string) (Rendered, error) {
	c, ok := t.byName[name]
	if !ok {
		return Rendered{}, fmt.Errorf("%w: %q", ErrUnknownTemplate, name)
	}

	plain := make(map[string]string, len(c.variables))
	escaped := make(map[string]string, len(c.variables))
	for _, key := range c.variables {
		value, ok := data[key]
		if !ok {
			return Rendered{}, fmt.Errorf("%w: %s.%s", ErrMissingVariable, name, key)
		}
		if isURLVariable(key) {
			if err := validateURL(value); err != nil {
				return Rendered{}, fmt.Errorf("%w: %s.%s", ErrInvalidURL, name, key)
			}
		}
		plain[key] = value
		escaped[key] = html.EscapeString(value)
	}

	subject, err := execute(c.subject, plain)
	if err != nil {
		return Rendered{}, err
	}
	body, err := execute(c.html, escaped)
	if err != nil {
		return Rendered{}, err
	}
	text, err := execute(c.text, plain)
	if err != nil {
		return Rendered{}, err
	}
	// A subject is one line: a stray newline in user-controlled data (a workspace name) would
	// otherwise be a header-injection vector on providers that pass it through verbatim.
	subject = strings.Join(strings.Fields(subject), " ")
	return Rendered{Subject: subject, HTML: body, Text: text}, nil
}

func execute(t *template.Template, data map[string]string) (string, error) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("mailer: render %s: %w", t.Name(), err)
	}
	return buf.String(), nil
}

func isURLVariable(key string) bool {
	return strings.HasSuffix(key, "Link") || strings.HasSuffix(key, "Url")
}

func validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("not an absolute http(s) url")
	}
	return nil
}
