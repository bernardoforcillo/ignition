package resend_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bernardoforcillo/ignition/go-packages/mailer"
	"github.com/bernardoforcillo/ignition/go-packages/mailer/resend"
)

func TestSend_PostsTheMessageWithAuthAndIdempotencyKey(t *testing.T) {
	var gotAuth, gotKey, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotKey, gotPath = r.Header.Get("Authorization"), r.Header.Get("Idempotency-Key"), r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"em_123"}`))
	}))
	defer srv.Close()
	c, err := resend.New(resend.Config{APIKey: "re_test", BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}

	err = c.Send(context.Background(), mailer.Message{
		From: "a@example.com", To: []string{"b@example.com"}, Subject: "Hi",
		HTML: "<p>x</p>", Text: "x", ReplyTo: "r@example.com", IdempotencyKey: "k-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	if gotPath != "/emails" || gotAuth != "Bearer re_test" || gotKey != "k-1" {
		t.Errorf("path/auth/key = %q / %q / %q", gotPath, gotAuth, gotKey)
	}
	if gotBody["from"] != "a@example.com" || gotBody["subject"] != "Hi" || gotBody["reply_to"] != "r@example.com" {
		t.Errorf("body = %v", gotBody)
	}
}

func TestSend_MapsStatusToRetryability(t *testing.T) {
	tests := []struct {
		status int
		want   error
	}{
		{http.StatusUnprocessableEntity, resend.ErrRejected},
		{http.StatusUnauthorized, resend.ErrRejected},
		{http.StatusTooManyRequests, resend.ErrTransient},
		{http.StatusBadGateway, resend.ErrTransient},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"name":"validation_error","message":"nope"}`))
			}))
			defer srv.Close()
			c, _ := resend.New(resend.Config{APIKey: "re_test", BaseURL: srv.URL})

			err := c.Send(context.Background(), mailer.Message{From: "a@b.co", To: []string{"c@d.co"}})
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestSend_NetworkErrorIsTransient(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	c, _ := resend.New(resend.Config{APIKey: "re_test", BaseURL: url})

	err := c.Send(context.Background(), mailer.Message{From: "a@b.co", To: []string{"c@d.co"}})
	if !errors.Is(err, resend.ErrTransient) {
		t.Errorf("err = %v, want ErrTransient", err)
	}
}

func TestNew_RejectsEmptyAPIKey(t *testing.T) {
	if _, err := resend.New(resend.Config{}); err == nil {
		t.Fatal("expected an error")
	}
}
