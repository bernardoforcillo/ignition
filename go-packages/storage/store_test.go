package storage_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bernardoforcillo/ignition/go-packages/storage"
	"github.com/bernardoforcillo/ignition/go-packages/storage/storagetest"
)

var t0 = time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)

// harness runs every behavior against both adapters: they must be interchangeable.
type harness struct {
	name  string
	store storage.Store
	srv   *storagetest.Server
	clock *atomic.Pointer[time.Time]
}

func (h harness) advance(d time.Duration) {
	next := h.clock.Load().Add(d)
	h.clock.Store(&next)
}

func harnesses(t *testing.T) []harness {
	t.Helper()
	var out []harness
	for _, name := range []string{"s3", "gcs"} {
		srv := storagetest.New(t)
		clock := &atomic.Pointer[time.Time]{}
		start := t0
		clock.Store(&start)
		srv.SetNow(func() time.Time { return *clock.Load() })
		var st storage.Store
		if name == "s3" {
			st = srv.S3(t)
		} else {
			st = srv.GCS(t)
		}
		out = append(out, harness{name: name, store: st, srv: srv, clock: clock})
	}
	return out
}

// each runs fn as a subtest per adapter.
func each(t *testing.T, fn func(t *testing.T, h harness)) {
	for _, h := range harnesses(t) {
		t.Run(h.name, func(t *testing.T) { fn(t, h) })
	}
}

func send(t *testing.T, method, rawURL string, headers map[string]string, body []byte) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(method, rawURL, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s: %v", method, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func putOpts(size int) storage.PutOptions {
	return storage.PutOptions{ContentType: "text/plain", Size: int64(size), Expires: 5 * time.Minute}
}

func TestStore_UploadStatDownloadDelete(t *testing.T) {
	each(t, func(t *testing.T, h harness) {
		ctx := context.Background()
		data := []byte("hello object storage")
		put, err := h.store.PresignPut(ctx, "workspaces/w1/obj 1", putOpts(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		if put.Method != http.MethodPut || put.Headers["Content-Type"] != "text/plain" {
			t.Fatalf("signed = %+v", put)
		}
		if want := t0.Add(5 * time.Minute); !put.ExpiresAt.Equal(want) {
			t.Errorf("ExpiresAt = %v, want %v", put.ExpiresAt, want)
		}

		if _, err := h.store.Stat(ctx, "workspaces/w1/obj 1"); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("Stat before upload = %v, want ErrNotFound", err)
		}
		if resp, body := send(t, put.Method, put.URL, put.Headers, data); resp.StatusCode != http.StatusOK {
			t.Fatalf("PUT = %d %s", resp.StatusCode, body)
		}
		obj, err := h.store.Stat(ctx, "workspaces/w1/obj 1")
		if err != nil {
			t.Fatal(err)
		}
		if obj.Size != int64(len(data)) || obj.ContentType != "text/plain" || obj.ETag == "" {
			t.Errorf("Stat = %+v", obj)
		}

		get, err := h.store.PresignGet(ctx, "workspaces/w1/obj 1", storage.GetOptions{Filename: `rep"ort.txt`})
		if err != nil {
			t.Fatal(err)
		}
		resp, body := send(t, get.Method, get.URL, nil, nil)
		if resp.StatusCode != http.StatusOK || body != string(data) {
			t.Fatalf("GET = %d %q", resp.StatusCode, body)
		}
		if cd := resp.Header.Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment; filename=") || strings.Contains(cd, `rep"ort`) {
			t.Errorf("Content-Disposition = %q", cd)
		}

		if err := h.store.Delete(ctx, "workspaces/w1/obj 1"); err != nil {
			t.Fatal(err)
		}
		if err := h.store.Delete(ctx, "workspaces/w1/obj 1"); err != nil {
			t.Errorf("deleting a missing object must succeed, got %v", err)
		}
		if _, err := h.store.Stat(ctx, "workspaces/w1/obj 1"); !errors.Is(err, storage.ErrNotFound) {
			t.Errorf("Stat after delete = %v, want ErrNotFound", err)
		}
	})
}

func TestPresignPut_ContentTypeIsBound(t *testing.T) {
	each(t, func(t *testing.T, h harness) {
		put, err := h.store.PresignPut(context.Background(), "k", putOpts(3))
		if err != nil {
			t.Fatal(err)
		}
		resp, _ := send(t, put.Method, put.URL, map[string]string{"Content-Type": "text/html"}, []byte("abc"))
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("a different content type got %d, want 403", resp.StatusCode)
		}
		if _, _, ok := h.srv.Get("k"); ok {
			t.Error("the refused upload was stored")
		}
	})
}

func TestPresignPut_SizeIsBound(t *testing.T) {
	each(t, func(t *testing.T, h harness) {
		put, err := h.store.PresignPut(context.Background(), "k", putOpts(3))
		if err != nil {
			t.Fatal(err)
		}
		for _, body := range []string{"toolong", "x"} {
			resp, _ := send(t, put.Method, put.URL, put.Headers, []byte(body))
			if resp.StatusCode < 400 {
				t.Errorf("a %d-byte body for a 3-byte signature got %d", len(body), resp.StatusCode)
			}
		}
		if _, _, ok := h.srv.Get("k"); ok {
			t.Error("a wrong-sized upload was stored")
		}
	})
}

func TestPresignedURL_ExpiresAndCannotBeRedirected(t *testing.T) {
	each(t, func(t *testing.T, h harness) {
		ctx := context.Background()
		h.srv.Put("a", []byte("A"), "text/plain")
		h.srv.Put("b", []byte("B"), "text/plain")
		get, err := h.store.PresignGet(ctx, "a", storage.GetOptions{Expires: time.Minute})
		if err != nil {
			t.Fatal(err)
		}

		// Another object under the same signature.
		other := strings.Replace(get.URL, "/a?", "/b?", 1)
		if resp, _ := send(t, "GET", other, nil, nil); resp.StatusCode != http.StatusForbidden {
			t.Errorf("a URL re-pointed at another key got %d, want 403", resp.StatusCode)
		}
		// A longer expiry in the query, same signature.
		longer := strings.Replace(get.URL, "Expires=60", "Expires=3600", 1)
		if longer == get.URL {
			t.Fatal("test bug: no expiry to tamper with")
		}
		if resp, _ := send(t, "GET", longer, nil, nil); resp.StatusCode != http.StatusForbidden {
			t.Errorf("a tampered expiry got %d, want 403", resp.StatusCode)
		}
		// A read URL is not a write URL.
		if resp, _ := send(t, "PUT", get.URL, nil, []byte("x")); resp.StatusCode != http.StatusForbidden {
			t.Errorf("PUT with a GET signature got %d, want 403", resp.StatusCode)
		}

		if resp, _ := send(t, "GET", get.URL, nil, nil); resp.StatusCode != http.StatusOK {
			t.Fatalf("fresh URL got %d, want 200", resp.StatusCode)
		}
		h.advance(2 * time.Minute)
		if resp, body := send(t, "GET", get.URL, nil, nil); resp.StatusCode != http.StatusForbidden || !strings.Contains(body, "ExpiredToken") {
			t.Errorf("expired URL got %d %s, want 403 ExpiredToken", resp.StatusCode, body)
		}
	})
}

func TestPresign_RejectsInvalidArguments(t *testing.T) {
	each(t, func(t *testing.T, h harness) {
		ctx := context.Background()
		bad := []struct {
			name string
			do   func() error
		}{
			{"empty key", func() error { _, err := h.store.PresignPut(ctx, "", putOpts(1)); return err }},
			{"dot-dot key", func() error { _, err := h.store.PresignGet(ctx, "a/../b", storage.GetOptions{}); return err }},
			{"no content type", func() error { _, err := h.store.PresignPut(ctx, "k", storage.PutOptions{Size: 1}); return err }},
			{"zero size", func() error {
				_, err := h.store.PresignPut(ctx, "k", storage.PutOptions{ContentType: "a/b"})
				return err
			}},
			{"size over the S3 limit", func() error {
				_, err := h.store.PresignPut(ctx, "k", storage.PutOptions{ContentType: "a/b", Size: storage.MaxObjectSize + 1})
				return err
			}},
			{"expiry over the cap", func() error {
				_, err := h.store.PresignGet(ctx, "k", storage.GetOptions{Expires: storage.MaxExpiry + time.Second})
				return err
			}},
			{"negative expiry", func() error {
				_, err := h.store.PresignGet(ctx, "k", storage.GetOptions{Expires: -time.Second})
				return err
			}},
			{"stat of a bad key", func() error { _, err := h.store.Stat(ctx, "/x"); return err }},
		}
		for _, tc := range bad {
			if err := tc.do(); !errors.Is(err, storage.ErrInvalid) {
				t.Errorf("%s: err = %v, want ErrInvalid", tc.name, err)
			}
		}
	})
}

func TestDefaultExpiryIsShort(t *testing.T) {
	each(t, func(t *testing.T, h harness) {
		put, err := h.store.PresignPut(context.Background(), "k", storage.PutOptions{ContentType: "a/b", Size: 1})
		if err != nil {
			t.Fatal(err)
		}
		if got := put.ExpiresAt.Sub(t0); got != storage.DefaultExpiry || got > 15*time.Minute {
			t.Errorf("default lifetime = %v", got)
		}
	})
}

func TestErrors_NeverCarryTheSignedURL(t *testing.T) {
	each(t, func(t *testing.T, h harness) {
		h.srv.Put("k", []byte("x"), "text/plain")
		// Break the connection: the server is gone but the store still tries it.
		dead := storagetest.New(t)
		var st storage.Store
		if h.name == "s3" {
			st = dead.S3(t)
		} else {
			st = dead.GCS(t)
		}
		dead.Close()
		_, err := st.Stat(context.Background(), "k")
		if err == nil {
			t.Fatal("want a transport error")
		}
		for _, leak := range []string{"Signature", "Credential", "X-Amz", "X-Goog"} {
			if strings.Contains(err.Error(), leak) {
				t.Errorf("error leaks %q: %v", leak, err)
			}
		}
	})
}

func TestConstructors_ValidateConfig(t *testing.T) {
	if _, err := storage.NewS3(storage.S3Config{Bucket: "b"}); err == nil {
		t.Error("s3 without credentials must fail")
	}
	if _, err := storage.NewS3(storage.S3Config{Bucket: "b", AccessKeyID: "i", SecretAccessKey: "s", Endpoint: "minio:9000"}); err == nil {
		t.Error("s3 endpoint without a scheme must fail")
	}
	if _, err := storage.NewGCS(storage.GCSConfig{Bucket: "b"}); err == nil {
		t.Error("gcs without credentials must fail")
	}
	const secret = "TOPSECRETKEYMATERIAL"
	_, err := storage.NewGCS(storage.GCSConfig{Bucket: "b", CredentialsJSON: []byte(`{"client_email":"a@b","private_key":"` + secret + `"}`)})
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Errorf("a bad key must fail without echoing it, got %v", err)
	}
}
