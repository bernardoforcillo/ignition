// Package httpwebhook exposes billing webhooks over net/http. It is transport
// only: read, parse (provider verifies the signature), delegate, map errors.
package httpwebhook

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/bernardoforcillo/ignition/go-packages/billing"
)

// DefaultMaxBody caps webhook bodies; provider events are a few KiB.
const DefaultMaxBody int64 = 1 << 20

// Parser verifies and decodes a webhook (satisfied by billing.Provider).
type Parser interface {
	ParseWebhook(ctx context.Context, payload []byte, headers http.Header) (billing.Event, error)
}

// Handler applies an event (satisfied by *billing.Service).
type Handler interface {
	HandleEvent(ctx context.Context, ev billing.Event) error
}

type webhook struct {
	parser  Parser
	handler Handler
	maxBody int64
	log     *slog.Logger
}

// New returns the webhook http.Handler. maxBody <= 0 uses DefaultMaxBody.
//
// Status mapping: 200 applied, replayed or ignored type; 400 bad signature or
// payload; 405 non-POST; 413 oversized body; 422 event valid but unusable
// (unknown price, no workspace) which a retry cannot fix; 500 anything else
// (e.g. sink down) so the provider retries.
func New(parser Parser, handler Handler, maxBody int64, log *slog.Logger) http.Handler {
	if maxBody <= 0 {
		maxBody = DefaultMaxBody
	}
	return &webhook{parser: parser, handler: handler, maxBody: maxBody, log: log}
}

func (h *webhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, h.maxBody))
	if err != nil {
		if _, tooBig := errors.AsType[*http.MaxBytesError](err); tooBig {
			http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "cannot read body", http.StatusBadRequest)
		return
	}

	ev, err := h.parser.ParseWebhook(r.Context(), body, r.Header)
	if err == nil {
		err = h.handler.HandleEvent(r.Context(), ev)
	}
	status := statusFor(err)
	if status >= http.StatusInternalServerError {
		h.log.ErrorContext(r.Context(), "billing webhook failed", "error", err)
	} else if err != nil {
		h.log.WarnContext(r.Context(), "billing webhook rejected", "error", err, "status", status)
	}
	w.WriteHeader(status)
}

func statusFor(err error) int {
	switch {
	case err == nil, errors.Is(err, billing.ErrIgnoredEvent):
		return http.StatusOK
	case errors.Is(err, billing.ErrInvalidSignature), errors.Is(err, billing.ErrMalformedEvent):
		return http.StatusBadRequest
	case errors.Is(err, billing.ErrUnknownPrice), errors.Is(err, billing.ErrAmbiguousPlan),
		errors.Is(err, billing.ErrMissingWorkspace):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}
