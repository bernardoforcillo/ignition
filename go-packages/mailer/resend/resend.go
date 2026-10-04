// Package resend is a mailer.Sender backed by the Resend HTTP API (https://resend.com/docs/api-reference/emails/send-email).
package resend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bernardoforcillo/ignition/go-packages/mailer"
)

// DefaultBaseURL is Resend's API origin.
const DefaultBaseURL = "https://api.resend.com"

// Errors a caller can match with errors.Is.
var (
	// ErrTransient marks a failure worth retrying: rate limited, a 5xx, or a network error.
	ErrTransient = errors.New("resend: transient error")
	// ErrRejected marks a permanent refusal (validation, bad key, unverified domain): retrying
	// the same message will not help.
	ErrRejected = errors.New("resend: rejected")
)

// Config configures a Client.
type Config struct {
	APIKey string
	// BaseURL overrides DefaultBaseURL (tests, regional endpoints).
	BaseURL string
	// HTTPClient overrides the default client, which has a 10s timeout.
	HTTPClient *http.Client
}

// Client implements mailer.Sender.
type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

var _ mailer.Sender = (*Client)(nil)

// New builds a Client.
func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("resend: empty api key")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{apiKey: cfg.APIKey, baseURL: strings.TrimRight(cfg.BaseURL, "/"), http: cfg.HTTPClient}, nil
}

type sendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html,omitempty"`
	Text    string   `json:"text,omitempty"`
	ReplyTo string   `json:"reply_to,omitempty"`
}

type errorResponse struct {
	Message string `json:"message"`
	Name    string `json:"name"`
}

// Send implements mailer.Sender.
func (c *Client) Send(ctx context.Context, msg mailer.Message) error {
	body, err := json.Marshal(sendRequest{
		From:    msg.From,
		To:      msg.To,
		Subject: msg.Subject,
		HTML:    msg.HTML,
		Text:    msg.Text,
		ReplyTo: msg.ReplyTo,
	})
	if err != nil {
		return fmt.Errorf("resend: encode: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/emails", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("resend: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	if msg.IdempotencyKey != "" {
		req.Header.Set("Idempotency-Key", msg.IdempotencyKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrTransient, err)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return fmt.Errorf("%w: status %d: %s", ErrTransient, resp.StatusCode, detail(payload))
	default:
		return fmt.Errorf("%w: status %d: %s", ErrRejected, resp.StatusCode, detail(payload))
	}
}

func detail(payload []byte) string {
	var e errorResponse
	if json.Unmarshal(payload, &e) == nil && e.Message != "" {
		return e.Name + ": " + e.Message
	}
	return "no detail"
}
