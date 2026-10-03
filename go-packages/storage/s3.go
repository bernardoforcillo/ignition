package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// S3Config configures the S3-compatible adapter: AWS S3, Cloudflare R2, MinIO, or Google Cloud
// Storage through its HMAC interoperability endpoint (https://storage.googleapis.com).
type S3Config struct {
	Bucket string
	// Region is the signing region; "us-east-1" when empty ("auto" for R2).
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	// Endpoint is the service origin, e.g. "https://<account>.r2.cloudflarestorage.com" or
	// "http://minio:9000". Empty means AWS S3 (virtual-hosted style, https).
	Endpoint string
	// PathStyle puts the bucket in the path (endpoint/bucket/key) instead of the host name. MinIO
	// needs it; R2 and AWS accept both.
	PathStyle bool
	// Client performs Stat and Delete; nil means a client with a 30s timeout.
	Client *http.Client
	// Now is the clock; nil means time.Now. Tests use it to cross an expiry.
	Now func() time.Time
}

// S3 is a Store over any S3-compatible service.
type S3 struct {
	*remote
	cfg      S3Config
	scheme   string
	endpoint string // host[:port] of Endpoint, or "" for AWS
}

var _ Store = (*S3)(nil)

// NewS3 validates cfg and builds the adapter.
func NewS3(cfg S3Config) (*S3, error) {
	if cfg.Bucket == "" || cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		return nil, errors.New("storage: s3: bucket, access key id and secret access key are required")
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	s := &S3{cfg: cfg, scheme: "https"}
	if cfg.Endpoint != "" {
		u, err := url.Parse(cfg.Endpoint)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || (u.Path != "" && u.Path != "/") {
			return nil, errors.New("storage: s3: endpoint must be an http(s) origin without a path")
		}
		s.scheme, s.endpoint = u.Scheme, u.Host
	}
	s.remote = newRemote(s, cfg.Client, cfg.Now)
	return s, nil
}

// locate returns the scheme, host and path an object lives at.
func (s *S3) locate(key string) (scheme, host, path string) {
	switch {
	case s.endpoint == "":
		return "https", s.cfg.Bucket + ".s3." + s.cfg.Region + ".amazonaws.com", "/" + key
	case s.cfg.PathStyle:
		return s.scheme, s.endpoint, "/" + s.cfg.Bucket + "/" + key
	default:
		return s.scheme, s.cfg.Bucket + "." + s.endpoint, "/" + key
	}
}

// sign produces an AWS Signature Version 4 presigned URL (query-string authentication). The
// payload is unsigned, so the client streams the body; a signed content-length binds its size and
// a signed content-type binds its type.
func (s *S3) sign(req request, now time.Time) (Signed, error) {
	now = now.UTC()
	date, stamp := now.Format("20060102"), now.Format("20060102T150405Z")
	scope := date + "/" + s.cfg.Region + "/s3/aws4_request"

	signedHeaders := make(map[string]string, len(req.headers)+1)
	for k, v := range req.headers {
		signedHeaders[k] = v
	}
	if req.size > 0 {
		signedHeaders["content-length"] = strconv.FormatInt(req.size, 10)
	}
	scheme, host, path := s.locate(req.key)
	canonHeaders, signedNames := canonicalHeaders(host, signedHeaders)

	query := map[string]string{
		"X-Amz-Algorithm":     "AWS4-HMAC-SHA256",
		"X-Amz-Credential":    s.cfg.AccessKeyID + "/" + scope,
		"X-Amz-Date":          stamp,
		"X-Amz-Expires":       strconv.FormatInt(int64(req.expires/time.Second), 10),
		"X-Amz-SignedHeaders": signedNames,
	}
	for k, v := range req.query {
		query[k] = v
	}
	cq := canonicalQuery(query)

	canonical := strings.Join([]string{req.method, uriEncode(path, false), cq, canonHeaders, signedNames, unsignedPayload}, "\n")
	toSign := strings.Join([]string{"AWS4-HMAC-SHA256", stamp, scope, sha256Hex(canonical)}, "\n")
	sig := hex.EncodeToString(hmacSHA256(s.signingKey(date), toSign))

	out := Signed{
		URL:       buildURL(scheme, host, path, cq+"&X-Amz-Signature="+sig),
		Method:    req.method,
		ExpiresAt: now.Add(req.expires),
	}
	if ct := req.headers["content-type"]; ct != "" {
		out.Headers = map[string]string{"Content-Type": ct}
	}
	return out, nil
}

func (s *S3) signingKey(date string) []byte {
	k := hmacSHA256([]byte("AWS4"+s.cfg.SecretAccessKey), date)
	k = hmacSHA256(k, s.cfg.Region)
	k = hmacSHA256(k, "s3")
	return hmacSHA256(k, "aws4_request")
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

// String never prints credentials.
func (s *S3) String() string {
	return fmt.Sprintf("storage.S3{bucket: %s, region: %s}", s.cfg.Bucket, s.cfg.Region)
}
