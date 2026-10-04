package storage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// signer turns a neutral request into a signed one; each adapter implements it for its scheme.
type signer interface {
	sign(req request, now time.Time) (Signed, error)
}

// remote implements Store over a signer: PresignPut and PresignGet are the signer's output, and
// Stat and Delete perform HEAD and DELETE requests through URLs it signs for itself. A signed URL
// is the credential, so there is no token exchange and nothing to refresh.
type remote struct {
	signer signer
	client *http.Client
	now    func() time.Time
}

var _ Store = (*remote)(nil)

func newRemote(s signer, client *http.Client, now func() time.Time) *remote {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if now == nil {
		now = time.Now
	}
	return &remote{signer: s, client: client, now: now}
}

func (r *remote) PresignPut(_ context.Context, key string, o PutOptions) (Signed, error) {
	if err := validateKey(key); err != nil {
		return Signed{}, err
	}
	expires, err := o.validate()
	if err != nil {
		return Signed{}, err
	}
	return r.signer.sign(request{
		method: http.MethodPut, key: key, expires: expires,
		headers: map[string]string{"content-type": o.ContentType},
		// The signer adds what binds the length (a signed header or a range extension header).
		size: o.Size,
	}, r.now())
}

func (r *remote) PresignGet(_ context.Context, key string, o GetOptions) (Signed, error) {
	if err := validateKey(key); err != nil {
		return Signed{}, err
	}
	expires, err := resolveExpiry(o.Expires)
	if err != nil {
		return Signed{}, err
	}
	req := request{method: http.MethodGet, key: key, expires: expires}
	if o.Filename != "" {
		req.query = map[string]string{"response-content-disposition": contentDisposition(o.Filename)}
	}
	return r.signer.sign(req, r.now())
}

func (r *remote) Stat(ctx context.Context, key string) (Object, error) {
	resp, err := r.do(ctx, http.MethodHead, key)
	if err != nil {
		return Object{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return Object{}, ErrNotFound
	case resp.StatusCode != http.StatusOK:
		return Object{}, fmt.Errorf("storage: stat: unexpected status %d", resp.StatusCode)
	}
	size, err := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	if err != nil {
		return Object{}, errors.New("storage: stat: response has no valid content length")
	}
	return Object{Size: size, ContentType: resp.Header.Get("Content-Type"), ETag: resp.Header.Get("ETag")}, nil
}

func (r *remote) Delete(ctx context.Context, key string) error {
	resp, err := r.do(ctx, http.MethodDelete, key)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent, http.StatusNotFound:
		return nil
	}
	return fmt.Errorf("storage: delete: unexpected status %d", resp.StatusCode)
}

// do signs a short-lived request for the object and performs it. Transport errors are unwrapped
// from *url.Error, whose text embeds the full signed URL.
func (r *remote) do(ctx context.Context, method, key string) (*http.Response, error) {
	if err := validateKey(key); err != nil {
		return nil, err
	}
	signed, err := r.signer.sign(request{method: method, key: key, expires: internalExpiry}, r.now())
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, signed.URL, nil)
	if err != nil {
		return nil, errors.New("storage: building request failed")
	}
	resp, err := r.client.Do(req)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err
		}
		return nil, fmt.Errorf("storage: %s: %w", method, err)
	}
	return resp, nil
}
