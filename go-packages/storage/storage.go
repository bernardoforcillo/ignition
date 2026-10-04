// Package storage is the object-storage port of the SaaS template: short-lived signed URLs so that
// bytes travel between the browser and the bucket and never through a Go binary, plus the two
// server-side calls an API needs afterwards (Stat to verify an upload, Delete to remove it).
//
// Two adapters ship, both stdlib-only: GCS (V4 signed URLs, RSA-SHA256, a service-account key) and
// S3 (SigV4 presigned URLs: AWS S3, Cloudflare R2, MinIO, and GCS through its HMAC interoperability
// endpoint). The package reads no environment and owns no transport; the application builds an
// adapter in its composition root. storagetest holds an in-memory server that checks signatures,
// for tests.
//
// Neither adapter needs a vendor SDK on purpose: signing is a page of code, and Stat and Delete are
// plain HTTP requests that are themselves signed URLs, so there is no OAuth flow, no token cache and
// no transitive dependency tree to audit.
package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Sentinel errors. Errors never carry a signed URL: it is a credential for its lifetime.
var (
	// ErrNotFound: the object does not exist.
	ErrNotFound = errors.New("storage: object not found")
	// ErrInvalid: the key or an option is not acceptable (empty, too long, an expiry out of range).
	ErrInvalid = errors.New("storage: invalid argument")
)

// Expiry bounds. A signed URL is a bearer credential, so it is short-lived by construction.
const (
	// DefaultExpiry is used when an option leaves Expires at zero.
	DefaultExpiry = 10 * time.Minute
	// MaxExpiry is the longest a URL may live; asking for more is an error, not a silent clamp.
	MaxExpiry = time.Hour
	// MaxObjectSize is the largest object a single signed PUT may carry (the S3 limit).
	MaxObjectSize int64 = 5 << 30
	maxKeyLen           = 1024
	// internalExpiry is the lifetime of the URLs an adapter signs for its own Stat and Delete calls.
	internalExpiry = time.Minute
)

// PutOptions constrain an upload. The signature binds them: a PUT whose content type or length
// differs from what was signed is refused by the storage service.
type PutOptions struct {
	// ContentType is required; the client must send exactly this Content-Type.
	ContentType string
	// Size is the exact number of bytes the client may upload, > 0 and <= MaxObjectSize.
	Size int64
	// Expires is how long the URL works; zero means DefaultExpiry, the maximum is MaxExpiry.
	Expires time.Duration
}

// GetOptions shape a download URL.
type GetOptions struct {
	// Expires is how long the URL works; zero means DefaultExpiry, the maximum is MaxExpiry.
	Expires time.Duration
	// Filename, when set, makes the response a download ("attachment") under that name instead of
	// letting the browser render the object inline.
	Filename string
}

// Signed is a request the client may perform without credentials of its own.
type Signed struct {
	URL    string
	Method string
	// Headers must be sent exactly as given. The content length is signed too, but the browser
	// derives it from the body, so it is not listed.
	Headers   map[string]string
	ExpiresAt time.Time
}

// Object is what Stat learned about a stored object.
type Object struct {
	Size        int64
	ContentType string
	ETag        string
}

// Signer hands out signed URLs. It is the part a request handler needs.
type Signer interface {
	PresignPut(ctx context.Context, key string, opts PutOptions) (Signed, error)
	PresignGet(ctx context.Context, key string, opts GetOptions) (Signed, error)
}

// Store is a Signer that can also look at and remove objects, which the use cases need to verify an
// upload and to clean up. Delete of a missing object succeeds.
type Store interface {
	Signer
	Stat(ctx context.Context, key string) (Object, error)
	Delete(ctx context.Context, key string) error
}

// validateKey rejects what is never a generated object key: empty, over-long, absolute, with a
// dot segment or a control character. Callers build keys themselves; this is the backstop.
func validateKey(key string) error {
	if key == "" || len(key) > maxKeyLen || strings.HasPrefix(key, "/") {
		return fmt.Errorf("%w: key", ErrInvalid)
	}
	for _, seg := range strings.Split(key, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return fmt.Errorf("%w: key", ErrInvalid)
		}
	}
	for _, r := range key {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("%w: key", ErrInvalid)
		}
	}
	return nil
}

// resolveExpiry applies the default and the bounds.
func resolveExpiry(d time.Duration) (time.Duration, error) {
	switch {
	case d == 0:
		return DefaultExpiry, nil
	case d < time.Second || d > MaxExpiry:
		return 0, fmt.Errorf("%w: expiry must be between 1s and %s", ErrInvalid, MaxExpiry)
	}
	return d, nil
}

func (o PutOptions) validate() (time.Duration, error) {
	if strings.TrimSpace(o.ContentType) == "" || strings.ContainsAny(o.ContentType, "\r\n") {
		return 0, fmt.Errorf("%w: content type", ErrInvalid)
	}
	if o.Size <= 0 || o.Size > MaxObjectSize {
		return 0, fmt.Errorf("%w: size must be between 1 and %d bytes", ErrInvalid, MaxObjectSize)
	}
	return resolveExpiry(o.Expires)
}
