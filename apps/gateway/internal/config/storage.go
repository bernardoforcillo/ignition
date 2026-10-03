package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Storage providers STORAGE_PROVIDER accepts.
const (
	StorageDisabled = "disabled"
	StorageGCS      = "gcs"
	StorageS3       = "s3"
)

// Storage holds the object-storage settings of the files feature. SaaS.Storage is nil when
// STORAGE_PROVIDER is "disabled" (the default): FileService then answers Unimplemented.
type Storage struct {
	// Provider is StorageGCS or StorageS3.
	Provider string
	Bucket   string

	// S3 and S3-compatible services (AWS, R2, MinIO, GCS with an HMAC key).
	Endpoint        string
	Region          string
	PathStyle       bool
	AccessKeyID     string
	SecretAccessKey string

	// GCSCredentialsJSON is a service account key file's content.
	GCSCredentialsJSON []byte
	// GCSEndpoint overrides the GCS API origin. Test-only, like SaaS.ResendBaseURL.
	GCSEndpoint string

	// Upload rules.
	MaxFileBytes int64
	AllowedTypes []string
	// UploadURLTTL and DownloadURLTTL are the lifetimes of the signed URLs.
	UploadURLTTL   time.Duration
	DownloadURLTTL time.Duration
	// PendingTTL is how long an upload may stay unconfirmed before the cleanup job removes it.
	PendingTTL time.Duration
}

const (
	defaultMaxFileBytes   int64 = 25 << 20
	maxFileBytesCeiling   int64 = 5 << 30 // one signed PUT; the S3 single-request limit
	defaultUploadURLTTL         = 10 * time.Minute
	defaultDownloadURLTTL       = 5 * time.Minute
	maxURLTTL                   = time.Hour
	defaultPendingTTL           = time.Hour
)

// defaultAllowedTypes is deliberately short and holds nothing a browser executes (no HTML, no SVG).
var defaultAllowedTypes = []string{
	"image/png", "image/jpeg", "image/gif", "image/webp",
	"application/pdf", "text/plain", "text/csv", "application/json", "application/zip",
}

// loadStorage reads the STORAGE_* variables; it returns nil when the provider is disabled.
func loadStorage() (*Storage, error) {
	provider := strings.ToLower(getEnv("STORAGE_PROVIDER", StorageDisabled))
	if provider == StorageDisabled {
		return nil, nil
	}
	s := &Storage{
		Provider:           provider,
		Bucket:             os.Getenv("STORAGE_BUCKET"),
		Endpoint:           os.Getenv("STORAGE_ENDPOINT"),
		Region:             os.Getenv("STORAGE_REGION"),
		AccessKeyID:        os.Getenv("STORAGE_ACCESS_KEY_ID"),
		SecretAccessKey:    os.Getenv("STORAGE_SECRET_ACCESS_KEY"),
		GCSCredentialsJSON: []byte(os.Getenv("STORAGE_GCS_CREDENTIALS_JSON")),
		GCSEndpoint:        os.Getenv("STORAGE_GCS_ENDPOINT"),
		MaxFileBytes:       defaultMaxFileBytes,
		AllowedTypes:       defaultAllowedTypes,
		UploadURLTTL:       defaultUploadURLTTL,
		DownloadURLTTL:     defaultDownloadURLTTL,
		PendingTTL:         defaultPendingTTL,
	}
	switch provider {
	case StorageGCS:
		if len(s.GCSCredentialsJSON) == 0 {
			return nil, fmt.Errorf("config: STORAGE_GCS_CREDENTIALS_JSON is required when STORAGE_PROVIDER=gcs")
		}
	case StorageS3:
		for _, r := range []struct{ name, value string }{
			{"STORAGE_ACCESS_KEY_ID", s.AccessKeyID},
			{"STORAGE_SECRET_ACCESS_KEY", s.SecretAccessKey},
		} {
			if r.value == "" {
				return nil, fmt.Errorf("config: %s is required when STORAGE_PROVIDER=s3", r.name)
			}
		}
	default:
		return nil, fmt.Errorf("config: invalid STORAGE_PROVIDER %q: want gcs, s3 or disabled", provider)
	}
	if s.Bucket == "" {
		return nil, fmt.Errorf("config: STORAGE_BUCKET is required when STORAGE_PROVIDER=%s", provider)
	}

	if v := os.Getenv("STORAGE_S3_PATH_STYLE"); v != "" {
		pathStyle, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("config: invalid STORAGE_S3_PATH_STYLE %q: want true or false", v)
		}
		s.PathStyle = pathStyle
	}
	if v := os.Getenv("STORAGE_MAX_FILE_BYTES"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 || n > maxFileBytesCeiling {
			return nil, fmt.Errorf("config: invalid STORAGE_MAX_FILE_BYTES %q: want 1 to %d", v, maxFileBytesCeiling)
		}
		s.MaxFileBytes = n
	}
	if v := os.Getenv("STORAGE_ALLOWED_TYPES"); v != "" {
		types, err := parseMediaTypes(v)
		if err != nil {
			return nil, err
		}
		s.AllowedTypes = types
	}
	for _, d := range []struct {
		name string
		dst  *time.Duration
		max  time.Duration
	}{
		{"STORAGE_UPLOAD_URL_TTL", &s.UploadURLTTL, maxURLTTL},
		{"STORAGE_DOWNLOAD_URL_TTL", &s.DownloadURLTTL, maxURLTTL},
		{"STORAGE_PENDING_TTL", &s.PendingTTL, 30 * 24 * time.Hour},
	} {
		v := os.Getenv(d.name)
		if v == "" {
			continue
		}
		parsed, err := time.ParseDuration(v)
		if err != nil || parsed < time.Second || parsed > d.max {
			return nil, fmt.Errorf("config: invalid %s %q: want a duration between 1s and %s", d.name, v, d.max)
		}
		*d.dst = parsed
	}
	// An upload must not be purged while its signed URL still works.
	if s.PendingTTL <= s.UploadURLTTL {
		return nil, fmt.Errorf("config: STORAGE_PENDING_TTL (%s) must be longer than STORAGE_UPLOAD_URL_TTL (%s)", s.PendingTTL, s.UploadURLTTL)
	}
	return s, nil
}

// parseMediaTypes reads "image/png, application/pdf": exact lower-case media types, no wildcards.
func parseMediaTypes(raw string) ([]string, error) {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" {
			continue
		}
		typ, sub, ok := strings.Cut(part, "/")
		if !ok || typ == "" || sub == "" || strings.ContainsAny(part, "*; \t") || strings.Count(part, "/") != 1 {
			return nil, fmt.Errorf("config: invalid STORAGE_ALLOWED_TYPES entry %q: want an exact type/subtype", part)
		}
		out = append(out, part)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("config: STORAGE_ALLOWED_TYPES lists no type")
	}
	return out, nil
}
