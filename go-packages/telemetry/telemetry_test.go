package telemetry_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/bernardoforcillo/ignition/go-packages/telemetry"
)

// fakePostHog stands in for the ingestion endpoint and keeps every event it receives.
type fakePostHog struct {
	srv *httptest.Server
	mu  sync.Mutex
	got []map[string]any
}

func newFakePostHog(t *testing.T) *fakePostHog {
	t.Helper()
	f := &fakePostHog{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				t.Errorf("gzip: %v", err)
				return
			}
			body = gz
		}
		raw, _ := io.ReadAll(body)
		var payload struct {
			Batch []map[string]any `json:"batch"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Errorf("decode %q: %v", raw, err)
		}
		f.mu.Lock()
		f.got = append(f.got, payload.Batch...)
		f.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":1}`))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakePostHog) events() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any(nil), f.got...)
}

func newTelemetry(t *testing.T, ph *fakePostHog, key string) (*telemetry.Telemetry, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	host := ""
	if ph != nil {
		host = ph.srv.URL
	}
	tel, err := telemetry.New(telemetry.Config{
		APIKey: key, Host: host, ServiceName: "gateway", Environment: "test", BatchSize: 1,
	}, &logs)
	if err != nil {
		t.Fatal(err)
	}
	return tel, &logs
}

// distinct returns the event's distinct id: top level for captures, inside properties for the
// $exception events the SDK builds.
func distinct(e map[string]any) any {
	if id, ok := e["distinct_id"]; ok {
		return id
	}
	return props(e)["distinct_id"]
}

func props(e map[string]any) map[string]any {
	p, _ := e["properties"].(map[string]any)
	return p
}

func TestCapture_SendsAPseudonymousEventWithServiceContext(t *testing.T) {
	ph := newFakePostHog(t)
	tel, _ := newTelemetry(t, ph, "phc_test")

	tel.Capture(context.Background(), "user-1", "workspace_created", map[string]any{"plan": "free"})
	if err := tel.Close(); err != nil {
		t.Fatal(err)
	}

	evs := ph.events()
	if len(evs) != 1 {
		t.Fatalf("got %d events, want 1: %v", len(evs), evs)
	}
	e := evs[0]
	if e["event"] != "workspace_created" || distinct(e) != "user-1" {
		t.Errorf("event = %v", e)
	}
	if p := props(e); p["plan"] != "free" || p["service"] != "gateway" || p["environment"] != "test" || p["$process_person_profile"] == false {
		t.Errorf("properties = %v", p)
	}
}

func TestCapture_IsANoOpWhenDisabledOrAnonymous(t *testing.T) {
	ph := newFakePostHog(t)
	disabled, _ := newTelemetry(t, ph, "")
	enabled, _ := newTelemetry(t, ph, "phc_test")

	disabled.Capture(context.Background(), "user-1", "e", nil)
	enabled.Capture(context.Background(), "", "e", nil)
	enabled.Capture(context.Background(), "user-1", "", nil)
	_ = disabled.Close()
	_ = enabled.Close()

	if n := len(ph.events()); n != 0 {
		t.Fatalf("%d events sent, want none", n)
	}
	if disabled.Enabled() || !enabled.Enabled() {
		t.Errorf("Enabled() = %v / %v", disabled.Enabled(), enabled.Enabled())
	}
}

func TestErrorLogs_BecomeExceptionsAttributedToTheSignedInUser(t *testing.T) {
	ph := newFakePostHog(t)
	tel, logs := newTelemetry(t, ph, "phc_test")
	ctx := telemetry.WithDistinctID(context.Background(), "user-7")

	tel.Logger().InfoContext(ctx, "boring")
	tel.Logger().ErrorContext(ctx, "checkout failed", "error", errors.New("stripe 500"))
	_ = tel.Close()

	if !strings.Contains(logs.String(), "boring") || !strings.Contains(logs.String(), "checkout failed") {
		t.Errorf("logs must still be written to the writer: %s", logs.String())
	}
	evs := ph.events()
	if len(evs) != 1 || evs[0]["event"] != "$exception" {
		t.Fatalf("want one $exception, got %v", evs)
	}
	if distinct(evs[0]) != "user-7" {
		t.Errorf("distinct id = %v", distinct(evs[0]))
	}
}

func TestErrorLogs_WithoutAUserAreAttributedToTheServiceNotAPerson(t *testing.T) {
	ph := newFakePostHog(t)
	tel, _ := newTelemetry(t, ph, "phc_test")

	tel.Logger().Error("db down")
	_ = tel.Close()

	evs := ph.events()
	if len(evs) != 1 {
		t.Fatalf("got %v", evs)
	}
	p := props(evs[0])
	if distinct(evs[0]) != "service:gateway" || p["$process_person_profile"] != false {
		t.Errorf("distinct id = %v, $process_person_profile = %v", distinct(evs[0]), p["$process_person_profile"])
	}
}

func TestCaptureException_TitlesWithTheTypeNotTheMessage(t *testing.T) {
	ph := newFakePostHog(t)
	tel, _ := newTelemetry(t, ph, "phc_test")

	tel.CaptureException(context.Background(), "user-1", errors.New("secret detail"), map[string]any{"rpc": "Login"})
	_ = tel.Close()

	evs := ph.events()
	if len(evs) != 1 || evs[0]["event"] != "$exception" {
		t.Fatalf("got %v", evs)
	}
	list, _ := props(evs[0])["$exception_list"].([]any)
	first, _ := list[0].(map[string]any)
	if first["type"] != "*errors.errorString" || props(evs[0])["rpc"] != "Login" {
		t.Errorf("exception = %v / %v", first, props(evs[0]))
	}
}

func TestLogging_WorksWithoutAPostHogKey(t *testing.T) {
	tel, logs := newTelemetry(t, nil, "")

	tel.Logger().Error("still logged")
	tel.CaptureException(context.Background(), "u", errors.New("x"), nil)

	if !strings.Contains(logs.String(), "still logged") {
		t.Errorf("logs = %q", logs.String())
	}
	if err := tel.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestNew_ValidatesConfig(t *testing.T) {
	if _, err := telemetry.New(telemetry.Config{}, &bytes.Buffer{}); err == nil {
		t.Error("empty service name must be rejected")
	}
	if _, err := telemetry.New(telemetry.Config{ServiceName: "x"}, nil); err == nil {
		t.Error("nil writer must be rejected")
	}
}
