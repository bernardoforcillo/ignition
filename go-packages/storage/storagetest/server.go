// Package storagetest is an in-memory object store behind a real HTTP server, for tests.
//
// It accepts only requests whose signature checks out, and it checks them from the received
// request, not from what the signer meant: the verifier below is written independently of the
// adapters in package storage, so a signing bug (a wrong canonical form, a header that is signed
// but not sent, an expired URL, a body longer than the signed length) is refused here the way the
// real services would refuse it. It speaks both schemes at once: AWS SigV4 query-string
// authentication (S3, R2, MinIO) and Google V4 (GOOG4-RSA-SHA256).
//
// It also answers CORS preflights, so a browser on another origin can PUT to it: the end-to-end
// harness runs an equivalent server written in TypeScript.
package storagetest

import (
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bernardoforcillo/ignition/go-packages/storage"
)

// Fixed identities the server accepts.
const (
	Bucket          = "test-bucket"
	AccessKeyID     = "AKIATESTACCESSKEY"
	SecretAccessKey = "test-secret-access-key-0123456789"
	Region          = "test-region"
	GCSEmail        = "signer@test-project.iam.gserviceaccount.com"

	maxBody = 64 << 20
)

type object struct {
	data        []byte
	contentType string
}

// Server is the fake. Build it with New; it is shut down with the test.
type Server struct {
	// URL is the server origin (http://127.0.0.1:port).
	URL string
	// GCSCredentials is a service account JSON key whose public half the server trusts.
	GCSCredentials []byte

	ts     *httptest.Server
	gcsPub *rsa.PublicKey

	mu      sync.Mutex
	now     func() time.Time
	objects map[string]object
}

// New starts a server and registers its cleanup.
func New(tb testing.TB) *Server {
	tb.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		tb.Fatalf("generating key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		tb.Fatalf("encoding key: %v", err)
	}
	creds, err := json.Marshal(map[string]string{
		"type":         "service_account",
		"client_email": GCSEmail,
		"private_key":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
	})
	if err != nil {
		tb.Fatalf("encoding credentials: %v", err)
	}
	s := &Server{GCSCredentials: creds, gcsPub: &key.PublicKey, now: time.Now, objects: map[string]object{}}
	s.ts = httptest.NewServer(http.HandlerFunc(s.serve))
	s.URL = s.ts.URL
	tb.Cleanup(s.ts.Close)
	return s
}

// Close stops the server early (a test of the failure path); it is also closed with the test.
func (s *Server) Close() { s.ts.Close() }

// SetNow replaces the clock the server judges expiry with (the stores built by S3 and GCS share it).
func (s *Server) SetNow(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = now
}

// Now is the server's clock.
func (s *Server) Now() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.now()
}

// S3 returns an S3 adapter pointed at the server (path style, like MinIO).
func (s *Server) S3(tb testing.TB) *storage.S3 {
	tb.Helper()
	st, err := storage.NewS3(storage.S3Config{
		Bucket: Bucket, Region: Region, AccessKeyID: AccessKeyID, SecretAccessKey: SecretAccessKey,
		Endpoint: s.URL, PathStyle: true, Now: s.Now,
	})
	if err != nil {
		tb.Fatalf("building s3 store: %v", err)
	}
	return st
}

// GCS returns a GCS adapter pointed at the server.
func (s *Server) GCS(tb testing.TB) *storage.GCS {
	tb.Helper()
	st, err := storage.NewGCS(storage.GCSConfig{Bucket: Bucket, CredentialsJSON: s.GCSCredentials, Endpoint: s.URL, Now: s.Now})
	if err != nil {
		tb.Fatalf("building gcs store: %v", err)
	}
	return st
}

// Put stores an object directly, bypassing signatures (seeding a test).
func (s *Server) Put(key string, data []byte, contentType string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = object{data: append([]byte(nil), data...), contentType: contentType}
}

// Get returns an object's bytes and content type.
func (s *Server) Get(key string) (data []byte, contentType string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.objects[key]
	return o.data, o.contentType, ok
}

// Keys lists the stored keys, sorted.
func (s *Server) Keys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]string, 0, len(s.objects))
	for k := range s.objects {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Methods", "PUT, GET, HEAD, DELETE")
	h.Set("Access-Control-Allow-Headers", "content-type, x-goog-content-length-range")
	h.Set("Access-Control-Expose-Headers", "ETag")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	bucket, key, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if bucket != Bucket || key == "" {
		fail(w, http.StatusNotFound, "NoSuchBucket")
		return
	}
	var minLen, maxLen int64 = 0, -1
	var code string
	var status int
	q := r.URL.Query()
	switch {
	case q.Get("X-Amz-Algorithm") != "":
		status, code = s.verifyS3(r)
	case q.Get("X-Goog-Algorithm") != "":
		status, code, minLen, maxLen = s.verifyGCS(r)
	default:
		status, code = http.StatusForbidden, "AccessDenied"
	}
	if code != "" {
		fail(w, status, code)
		return
	}

	switch r.Method {
	case http.MethodPut:
		body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
		if err != nil || len(body) > maxBody {
			fail(w, http.StatusBadRequest, "IncompleteBody")
			return
		}
		if maxLen >= 0 && int64(len(body)) > maxLen {
			fail(w, http.StatusBadRequest, "EntityTooLarge")
			return
		}
		if int64(len(body)) < minLen {
			fail(w, http.StatusBadRequest, "EntityTooSmall")
			return
		}
		s.Put(key, body, r.Header.Get("Content-Type"))
		h.Set("ETag", `"`+hex.EncodeToString(sum(body))+`"`)
		w.WriteHeader(http.StatusOK)
	case http.MethodGet, http.MethodHead:
		data, ct, ok := s.Get(key)
		if !ok {
			fail(w, http.StatusNotFound, "NoSuchKey")
			return
		}
		h.Set("Content-Type", ct)
		h.Set("Content-Length", strconv.Itoa(len(data)))
		h.Set("ETag", `"`+hex.EncodeToString(sum(data))+`"`)
		if d := q.Get("response-content-disposition"); d != "" {
			h.Set("Content-Disposition", d)
		}
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			_, _ = w.Write(data)
		}
	case http.MethodDelete:
		s.mu.Lock()
		delete(s.objects, key)
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		fail(w, http.StatusMethodNotAllowed, "MethodNotAllowed")
	}
}

func fail(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, "<Error><Code>"+code+"</Code></Error>")
}

func sum(b []byte) []byte { s := sha256.Sum256(b); return s[:] }

// canonical rebuilds the canonical request from the request as received. signedHeaders is the
// client-declared list; each value is read from the request itself, so a header that was signed
// but not sent (or sent differently) changes the result.
func canonical(r *http.Request, q url.Values, sigParam, signedHeaders string) string {
	pairs := make([]string, 0, len(q))
	for k, vs := range q {
		if k == sigParam {
			continue
		}
		pairs = append(pairs, enc(k)+"="+enc(vs[0]))
	}
	sort.Strings(pairs)

	names := strings.Split(signedHeaders, ";")
	var hdrs strings.Builder
	for _, n := range names {
		var v string
		switch n {
		case "host":
			v = r.Host
		case "content-length":
			if r.ContentLength >= 0 {
				v = strconv.FormatInt(r.ContentLength, 10)
			}
		default:
			v = strings.TrimSpace(r.Header.Get(n))
		}
		hdrs.WriteString(n + ":" + v + "\n")
	}
	return strings.Join([]string{
		r.Method, encPath(r.URL.Path), strings.Join(pairs, "&"), hdrs.String(), signedHeaders, "UNSIGNED-PAYLOAD",
	}, "\n")
}

func enc(s string) string { return strings.ReplaceAll(url.QueryEscape(s), "+", "%20") }

func encPath(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		segs[i] = strings.ReplaceAll(enc(s), "%7E", "~")
	}
	return strings.Join(segs, "/")
}

// valid reports whether now lies inside [stamp, stamp+expires].
func (s *Server) valid(stamp, expires string) bool {
	t, err := time.Parse("20060102T150405Z", stamp)
	exp, err2 := strconv.ParseInt(expires, 10, 64)
	if err != nil || err2 != nil || exp <= 0 || exp > 7*24*3600 {
		return false
	}
	now := s.Now()
	return !now.Before(t.Add(-time.Minute)) && now.Before(t.Add(time.Duration(exp)*time.Second))
}

func (s *Server) verifyS3(r *http.Request) (int, string) {
	q := r.URL.Query()
	if q.Get("X-Amz-Algorithm") != "AWS4-HMAC-SHA256" {
		return http.StatusForbidden, "AccessDenied"
	}
	cred := strings.Split(q.Get("X-Amz-Credential"), "/")
	if len(cred) != 5 || cred[0] != AccessKeyID || cred[2] != Region || cred[3] != "s3" || cred[4] != "aws4_request" {
		return http.StatusForbidden, "InvalidAccessKeyId"
	}
	stamp := q.Get("X-Amz-Date")
	if !s.valid(stamp, q.Get("X-Amz-Expires")) {
		return http.StatusForbidden, "ExpiredToken"
	}
	if !strings.HasPrefix(stamp, cred[1]) {
		return http.StatusForbidden, "AccessDenied"
	}
	signed := q.Get("X-Amz-SignedHeaders")
	if !strings.Contains(";"+signed+";", ";host;") {
		return http.StatusForbidden, "AccessDenied"
	}
	scope := strings.Join(cred[1:], "/")
	toSign := "AWS4-HMAC-SHA256\n" + stamp + "\n" + scope + "\n" + hex.EncodeToString(sum([]byte(canonical(r, q, "X-Amz-Signature", signed))))
	k := mac([]byte("AWS4"+SecretAccessKey), cred[1])
	k = mac(k, cred[2])
	k = mac(k, cred[3])
	k = mac(k, cred[4])
	want := hex.EncodeToString(mac(k, toSign))
	if !hmac.Equal([]byte(want), []byte(q.Get("X-Amz-Signature"))) {
		return http.StatusForbidden, "SignatureDoesNotMatch"
	}
	return 0, ""
}

func mac(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

// verifyGCS also returns the body length range the signed range header allows (max -1: unbounded).
func (s *Server) verifyGCS(r *http.Request) (status int, code string, minLen, maxLen int64) {
	q := r.URL.Query()
	if q.Get("X-Goog-Algorithm") != "GOOG4-RSA-SHA256" {
		return http.StatusForbidden, "AccessDenied", 0, -1
	}
	cred := strings.Split(q.Get("X-Goog-Credential"), "/")
	if len(cred) != 5 || cred[0] != GCSEmail || cred[2] != "auto" || cred[3] != "storage" || cred[4] != "goog4_request" {
		return http.StatusForbidden, "InvalidAccessKeyId", 0, -1
	}
	stamp := q.Get("X-Goog-Date")
	if !s.valid(stamp, q.Get("X-Goog-Expires")) {
		return http.StatusForbidden, "ExpiredToken", 0, -1
	}
	signed := q.Get("X-Goog-SignedHeaders")
	if !strings.Contains(";"+signed+";", ";host;") {
		return http.StatusForbidden, "AccessDenied", 0, -1
	}
	scope := strings.Join(cred[1:], "/")
	toSign := "GOOG4-RSA-SHA256\n" + stamp + "\n" + scope + "\n" + hex.EncodeToString(sum([]byte(canonical(r, q, "X-Goog-Signature", signed))))
	sig, err := hex.DecodeString(q.Get("X-Goog-Signature"))
	if err != nil {
		return http.StatusForbidden, "SignatureDoesNotMatch", 0, -1
	}
	if rsa.VerifyPKCS1v15(s.gcsPub, crypto.SHA256, sum([]byte(toSign)), sig) != nil {
		return http.StatusForbidden, "SignatureDoesNotMatch", 0, -1
	}
	maxLen = -1
	if rng := r.Header.Get("x-goog-content-length-range"); rng != "" {
		lo, hi, ok := strings.Cut(rng, ",")
		min, err1 := strconv.ParseInt(lo, 10, 64)
		max, err2 := strconv.ParseInt(hi, 10, 64)
		if !ok || err1 != nil || err2 != nil {
			return http.StatusBadRequest, "InvalidArgument", 0, -1
		}
		minLen, maxLen = min, max
	}
	return 0, "", minLen, maxLen
}
