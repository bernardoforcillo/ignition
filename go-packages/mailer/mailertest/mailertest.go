// Package mailertest provides a recording mailer.Sender for tests.
package mailertest

import (
	"context"
	"sync"

	"github.com/bernardoforcillo/ignition/go-packages/mailer"
)

// Recorder keeps every message it is asked to send. Set Err to make Send fail.
type Recorder struct {
	mu       sync.Mutex
	messages []mailer.Message
	Err      error
}

var _ mailer.Sender = (*Recorder)(nil)

// Send implements mailer.Sender.
func (r *Recorder) Send(_ context.Context, msg mailer.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.messages = append(r.messages, msg)
	return nil
}

// Messages returns a copy of what was sent.
func (r *Recorder) Messages() []mailer.Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]mailer.Message(nil), r.messages...)
}
