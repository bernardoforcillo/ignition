package storage

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// gcsHost is the Cloud Storage XML API host the V4 signed URLs target.
const gcsHost = "storage.googleapis.com"

// GCSConfig configures the Google Cloud Storage adapter.
type GCSConfig struct {
	Bucket string
	// CredentialsJSON is a service account key file's content (client_email and private_key). The
	// account needs object read/write/delete on the bucket. Keyless environments (Workload
	// Identity) can use the S3 adapter against storage.googleapis.com with an HMAC key instead.
	CredentialsJSON []byte
	// Endpoint overrides the API origin (tests, emulators); empty means https://storage.googleapis.com.
	Endpoint string
	Client   *http.Client
	Now      func() time.Time
}

// GCS is a Store over Google Cloud Storage, using V4 signed URLs (GOOG4-RSA-SHA256).
type GCS struct {
	*remote
	bucket string
	email  string
	key    *rsa.PrivateKey
	scheme string
	host   string
}

var _ Store = (*GCS)(nil)

// NewGCS validates cfg, parses the service account key and builds the adapter.
func NewGCS(cfg GCSConfig) (*GCS, error) {
	if cfg.Bucket == "" {
		return nil, errors.New("storage: gcs: bucket is required")
	}
	email, key, err := parseServiceAccount(cfg.CredentialsJSON)
	if err != nil {
		return nil, err
	}
	g := &GCS{bucket: cfg.Bucket, email: email, key: key, scheme: "https", host: gcsHost}
	if cfg.Endpoint != "" {
		u, err := url.Parse(cfg.Endpoint)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || (u.Path != "" && u.Path != "/") {
			return nil, errors.New("storage: gcs: endpoint must be an http(s) origin without a path")
		}
		g.scheme, g.host = u.Scheme, u.Host
	}
	g.remote = newRemote(g, cfg.Client, cfg.Now)
	return g, nil
}

// parseServiceAccount extracts the signer identity from a service account key file. Error text
// never includes the key material.
func parseServiceAccount(raw []byte) (string, *rsa.PrivateKey, error) {
	var sa struct {
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
	}
	if err := json.Unmarshal(raw, &sa); err != nil || sa.ClientEmail == "" || sa.PrivateKey == "" {
		return "", nil, errors.New("storage: gcs: credentials must be a service account JSON key with client_email and private_key")
	}
	block, _ := pem.Decode([]byte(sa.PrivateKey))
	if block == nil {
		return "", nil, errors.New("storage: gcs: private_key is not PEM")
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rk, ok := k.(*rsa.PrivateKey); ok {
			return sa.ClientEmail, rk, nil
		}
		return "", nil, errors.New("storage: gcs: private_key is not an RSA key")
	}
	rk, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return "", nil, errors.New("storage: gcs: private_key could not be parsed")
	}
	return sa.ClientEmail, rk, nil
}

// sign produces a V4 signed URL. The upload is bound to its content type and, through the signed
// x-goog-content-length-range extension header (which the client must echo), to its exact size.
func (g *GCS) sign(req request, now time.Time) (Signed, error) {
	now = now.UTC()
	date, stamp := now.Format("20060102"), now.Format("20060102T150405Z")
	scope := date + "/auto/storage/goog4_request"

	headers := make(map[string]string, len(req.headers)+1)
	for k, v := range req.headers {
		headers[k] = v
	}
	var out Signed
	out.Method = req.method
	out.ExpiresAt = now.Add(req.expires)
	if req.size > 0 {
		rng := strconv.FormatInt(req.size, 10) + "," + strconv.FormatInt(req.size, 10)
		headers["x-goog-content-length-range"] = rng
		out.Headers = map[string]string{"Content-Type": req.headers["content-type"], "x-goog-content-length-range": rng}
	}

	path := "/" + g.bucket + "/" + req.key
	canonHeaders, signedNames := canonicalHeaders(g.host, headers)
	query := map[string]string{
		"X-Goog-Algorithm":     "GOOG4-RSA-SHA256",
		"X-Goog-Credential":    g.email + "/" + scope,
		"X-Goog-Date":          stamp,
		"X-Goog-Expires":       strconv.FormatInt(int64(req.expires/time.Second), 10),
		"X-Goog-SignedHeaders": signedNames,
	}
	for k, v := range req.query {
		query[k] = v
	}
	cq := canonicalQuery(query)

	canonical := strings.Join([]string{req.method, uriEncode(path, false), cq, canonHeaders, signedNames, unsignedPayload}, "\n")
	toSign := strings.Join([]string{"GOOG4-RSA-SHA256", stamp, scope, sha256Hex(canonical)}, "\n")
	digest := sha256.Sum256([]byte(toSign))
	sig, err := rsa.SignPKCS1v15(rand.Reader, g.key, crypto.SHA256, digest[:])
	if err != nil {
		return Signed{}, errors.New("storage: gcs: signing failed")
	}
	out.URL = buildURL(g.scheme, g.host, path, cq+"&X-Goog-Signature="+hex.EncodeToString(sig))
	return out, nil
}

// String never prints key material.
func (g *GCS) String() string {
	return "storage.GCS{bucket: " + g.bucket + ", account: " + g.email + "}"
}
