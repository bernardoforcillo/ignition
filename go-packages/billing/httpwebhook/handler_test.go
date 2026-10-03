package httpwebhook_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bernardoforcillo/ignition/go-packages/billing"
	"github.com/bernardoforcillo/ignition/go-packages/billing/httpwebhook"
)

type fakeParser struct {
	ev  billing.Event
	err error
}

func (p fakeParser) ParseWebhook(context.Context, []byte, http.Header) (billing.Event, error) {
	return p.ev, p.err
}

type fakeHandler struct {
	err   error
	calls int
}

func (h *fakeHandler) HandleEvent(context.Context, billing.Event) error {
	h.calls++
	return h.err
}

func TestWebhook_MapsErrorsToStatus(t *testing.T) {
	tests := []struct {
		name      string
		parseErr  error
		handleErr error
		want      int
		wantCalls int
	}{
		{"applied", nil, nil, http.StatusOK, 1},
		{"ignored event type is acknowledged", billing.ErrIgnoredEvent, nil, http.StatusOK, 0},
		{"bad signature is 400", billing.ErrInvalidSignature, nil, http.StatusBadRequest, 0},
		{"malformed payload is 400", billing.ErrMalformedEvent, nil, http.StatusBadRequest, 0},
		{"unknown price is 422", nil, billing.ErrUnknownPrice, http.StatusUnprocessableEntity, 1},
		{"missing workspace is 422", nil, billing.ErrMissingWorkspace, http.StatusUnprocessableEntity, 1},
		{"transient sink error is 500 so provider retries", nil, errors.New("sink down"), http.StatusInternalServerError, 1},
		{"unexpected parse error is 500", errors.New("boom"), nil, http.StatusInternalServerError, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &fakeHandler{err: tt.handleErr}
			srv := httpwebhook.New(fakeParser{err: tt.parseErr}, h, 0, slog.New(slog.NewTextHandler(io.Discard, nil)))
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}")))
			if rec.Code != tt.want || h.calls != tt.wantCalls {
				t.Fatalf("status=%d calls=%d, want %d/%d", rec.Code, h.calls, tt.want, tt.wantCalls)
			}
		})
	}
}

func TestWebhook_RejectsOversizedBodyAndWrongMethod(t *testing.T) {
	h := &fakeHandler{}
	srv := httpwebhook.New(fakeParser{}, h, 8, slog.New(slog.NewTextHandler(io.Discard, nil)))

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("x", 9))))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized: status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusMethodNotAllowed || h.calls != 0 {
		t.Errorf("GET: status = %d calls = %d", rec.Code, h.calls)
	}
}
