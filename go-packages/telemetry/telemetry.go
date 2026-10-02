// Package telemetry is the service side of observability: structured logs, error tracking and
// product events, shipped to PostHog.
//
//   - Logs always go to the writer you give New, as JSON on stdout. That is the channel for
//     everything else (Cloud Logging on GCP, Loki, ...): FormatGCP shapes the records the way
//     Cloud Logging expects. PostHog is deliberately NOT a log store.
//   - With an API key, only the critical records, slog Error and above by default, are also sent to
//     PostHog error tracking as exceptions (with a stack trace). Info and Warn never leave the
//     process.
//   - Capture and CaptureException send explicit events and handled errors. The browser owns
//     product analytics (see apps/web); the server sends just the few events only it can vouch for.
//
// Without an API key every method is a safe no-op and logging still works, so local development
// and tests need no PostHog project.
//
// Privacy: this package never attaches an email, name or free text of its own. Callers pass a
// pseudonymous distinct id (the account id) and must keep personal data out of event properties.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/posthog/posthog-go"
)

// Format is the JSON shape of the log records.
type Format int

const (
	// FormatJSON is slog's default JSON (time, level, msg).
	FormatJSON Format = iota
	// FormatGCP uses Cloud Logging's special fields: severity, message and timestamp, so the
	// severity filters and alerting work without a parsing rule. Other attributes pass through.
	FormatGCP
)

// DefaultHost is PostHog's EU ingestion endpoint, the safe default for a product with EU users.
const DefaultHost = "https://eu.i.posthog.com"

// Config configures a Telemetry. APIKey is the PostHog project API key (write-only, safe to ship
// to the browser as well; it is not a personal key).
type Config struct {
	APIKey      string
	Host        string
	ServiceName string
	Environment string
	// LogLevel is the minimum level written to the log writer; the zero value is Info.
	LogLevel slog.Level
	// LogFormat shapes the JSON records; the zero value is FormatJSON.
	LogFormat Format
	// CaptureLevel is the minimum slog level mirrored to PostHog error tracking; the zero value is
	// slog.LevelError. Raise it, never lower it: Info and Warn belong in the log pipeline.
	CaptureLevel slog.Level
	// BatchSize overrides the SDK batch size (tests use 1 to flush immediately).
	BatchSize int
}

// Telemetry owns the PostHog client and the logger wired to it.
type Telemetry struct {
	client posthog.Client // nil when disabled
	logger *slog.Logger
	cfg    Config
}

type ctxKey struct{}

// WithDistinctID returns ctx carrying the pseudonymous id (the account id) that errors logged
// through the ctx-aware slog methods are attributed to.
func WithDistinctID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

func distinctIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// New builds a Telemetry that writes JSON logs to out.
func New(cfg Config, out io.Writer) (*Telemetry, error) {
	if out == nil {
		return nil, errors.New("telemetry: nil log writer")
	}
	if strings.TrimSpace(cfg.ServiceName) == "" {
		return nil, errors.New("telemetry: empty service name")
	}
	if cfg.Host == "" {
		cfg.Host = DefaultHost
	}

	base := slog.NewJSONHandler(out, &slog.HandlerOptions{Level: cfg.LogLevel, ReplaceAttr: replacer(cfg.LogFormat)})
	captureLevel := cfg.CaptureLevel
	if captureLevel == 0 {
		captureLevel = slog.LevelError
	}
	t := &Telemetry{cfg: cfg}

	if cfg.APIKey == "" {
		t.logger = slog.New(base)
		return t, nil
	}

	pc := posthog.Config{Endpoint: cfg.Host}
	if cfg.BatchSize > 0 {
		pc.BatchSize = cfg.BatchSize
	}
	client, err := posthog.NewWithConfig(cfg.APIKey, pc)
	if err != nil {
		return nil, fmt.Errorf("telemetry: posthog client: %w", err)
	}
	t.client = client
	t.logger = slog.New(posthog.NewSlogCaptureHandler(base, client,
		posthog.WithMinCaptureLevel(captureLevel),
		posthog.WithDistinctIDFn(func(ctx context.Context, _ slog.Record) string { return t.distinctID(ctx) }),
		posthog.WithPropertiesFn(func(ctx context.Context, _ slog.Record) posthog.Properties {
			return t.baseProperties(distinctIDFrom(ctx) == "")
		}),
	))
	return t, nil
}

// Logger returns the service logger. Install it with slog.SetDefault in main.
func (t *Telemetry) Logger() *slog.Logger { return t.logger }

// Enabled reports whether events are being sent to PostHog.
func (t *Telemetry) Enabled() bool { return t.client != nil }

// Capture sends a product event for a pseudonymous distinct id. It does nothing when telemetry is
// disabled or id is empty. props must not contain personal data.
func (t *Telemetry) Capture(ctx context.Context, id, event string, props map[string]any) {
	if t.client == nil || id == "" || event == "" {
		return
	}
	p := t.baseProperties(false)
	for k, v := range props {
		p.Set(k, v)
	}
	if err := t.client.Enqueue(posthog.Capture{DistinctId: id, Event: event, Properties: p}); err != nil {
		t.logger.WarnContext(ctx, "telemetry: capture failed", "event", event, "error", err)
	}
}

// CaptureException sends a handled error to PostHog error tracking. id may be empty, in which case
// the exception is attributed to the service rather than a person.
func (t *Telemetry) CaptureException(ctx context.Context, id string, err error, props map[string]any) {
	if t.client == nil || err == nil {
		return
	}
	if id == "" {
		id = distinctIDFrom(ctx)
	}
	if id == "" {
		id = t.serviceID()
	}
	ex := posthog.NewDefaultException(time.Now(), id, errorType(err), err.Error())
	ex.Properties = t.baseProperties(id == t.serviceID())
	for k, v := range props {
		ex.Properties.Set(k, v)
	}
	if e := t.client.Enqueue(ex); e != nil {
		t.logger.WarnContext(ctx, "telemetry: capture exception failed", "error", e)
	}
}

// errorType is the exception title in the PostHog UI: the concrete Go type, which groups the same
// kind of failure together without exposing the (possibly sensitive) message in the title.
func errorType(err error) string { return fmt.Sprintf("%T", err) }

// Close flushes pending events. Call it from main on shutdown.
func (t *Telemetry) Close() error {
	if t.client == nil {
		return nil
	}
	return t.client.Close()
}

func (t *Telemetry) serviceID() string { return "service:" + t.cfg.ServiceName }

func (t *Telemetry) distinctID(ctx context.Context) string {
	if id := distinctIDFrom(ctx); id != "" {
		return id
	}
	return t.serviceID()
}

// baseProperties is the context every event carries. anonymous marks an error attributed to the
// service rather than a person, so it must not create a person profile per service.
func (t *Telemetry) baseProperties(anonymous bool) posthog.Properties {
	p := posthog.NewProperties().
		Set("service", t.cfg.ServiceName).
		Set("environment", t.cfg.Environment)
	if anonymous {
		p.Set("$process_person_profile", false)
	}
	return p
}

// replacer renames the built-in attributes for the chosen format.
func replacer(f Format) func(groups []string, a slog.Attr) slog.Attr {
	if f != FormatGCP {
		return nil
	}
	return func(groups []string, a slog.Attr) slog.Attr {
		if len(groups) > 0 {
			return a
		}
		switch a.Key {
		case slog.TimeKey:
			a.Key = "timestamp"
		case slog.MessageKey:
			a.Key = "message"
		case slog.LevelKey:
			a.Key = "severity"
			if lvl, ok := a.Value.Any().(slog.Level); ok {
				a.Value = slog.StringValue(gcpSeverity(lvl))
			}
		}
		return a
	}
}

// gcpSeverity maps slog levels onto Cloud Logging's LogSeverity names.
func gcpSeverity(l slog.Level) string {
	switch {
	case l >= slog.LevelError+4:
		return "CRITICAL"
	case l >= slog.LevelError:
		return "ERROR"
	case l >= slog.LevelWarn:
		return "WARNING"
	case l >= slog.LevelInfo:
		return "INFO"
	default:
		return "DEBUG"
	}
}
