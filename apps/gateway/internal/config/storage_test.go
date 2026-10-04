package config

import (
	"slices"
	"strings"
	"testing"
	"time"
)

var storageEnvKeys = []string{
	"STORAGE_PROVIDER", "STORAGE_BUCKET", "STORAGE_ENDPOINT", "STORAGE_REGION", "STORAGE_S3_PATH_STYLE",
	"STORAGE_ACCESS_KEY_ID", "STORAGE_SECRET_ACCESS_KEY", "STORAGE_GCS_CREDENTIALS_JSON", "STORAGE_GCS_ENDPOINT",
	"STORAGE_MAX_FILE_BYTES", "STORAGE_ALLOWED_TYPES", "STORAGE_UPLOAD_URL_TTL", "STORAGE_DOWNLOAD_URL_TTL",
	"STORAGE_PENDING_TTL",
}

func storageEnv(t *testing.T, kv ...string) {
	t.Helper()
	saasEnv(t)
	for _, k := range storageEnvKeys {
		t.Setenv(k, "")
	}
	for i := 0; i < len(kv); i += 2 {
		t.Setenv(kv[i], kv[i+1])
	}
}

func TestLoad_StorageIsDisabledByDefault(t *testing.T) {
	storageEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SaaS.Storage != nil {
		t.Fatalf("Storage = %+v, want nil", cfg.SaaS.Storage)
	}
	storageEnv(t, "STORAGE_PROVIDER", "Disabled", "STORAGE_BUCKET", "ignored")
	if cfg, err = Load(); err != nil || cfg.SaaS.Storage != nil {
		t.Fatalf("explicitly disabled: %+v, %v", cfg.SaaS.Storage, err)
	}
}

func TestLoad_StorageS3(t *testing.T) {
	storageEnv(t, "STORAGE_PROVIDER", "s3", "STORAGE_BUCKET", "files", "STORAGE_ENDPOINT", "http://minio:9000",
		"STORAGE_S3_PATH_STYLE", "true", "STORAGE_ACCESS_KEY_ID", "id", "STORAGE_SECRET_ACCESS_KEY", "secret",
		"STORAGE_MAX_FILE_BYTES", "1048576", "STORAGE_ALLOWED_TYPES", "Image/PNG, application/pdf",
		"STORAGE_UPLOAD_URL_TTL", "2m", "STORAGE_DOWNLOAD_URL_TTL", "30s", "STORAGE_PENDING_TTL", "30m")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	s := cfg.SaaS.Storage
	if s == nil || s.Provider != StorageS3 || s.Bucket != "files" || s.Endpoint != "http://minio:9000" || !s.PathStyle ||
		s.AccessKeyID != "id" || s.SecretAccessKey != "secret" || s.MaxFileBytes != 1<<20 ||
		s.UploadURLTTL != 2*time.Minute || s.DownloadURLTTL != 30*time.Second || s.PendingTTL != 30*time.Minute ||
		!slices.Equal(s.AllowedTypes, []string{"image/png", "application/pdf"}) {
		t.Fatalf("Storage = %+v", s)
	}
}

func TestLoad_StorageDefaults(t *testing.T) {
	storageEnv(t, "STORAGE_PROVIDER", "gcs", "STORAGE_BUCKET", "files", "STORAGE_GCS_CREDENTIALS_JSON", "{}")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	s := cfg.SaaS.Storage
	if s.Provider != StorageGCS || s.MaxFileBytes != 25<<20 || s.UploadURLTTL != 10*time.Minute ||
		s.DownloadURLTTL != 5*time.Minute || s.PendingTTL != time.Hour {
		t.Errorf("defaults = %+v", s)
	}
	for _, mt := range s.AllowedTypes {
		if strings.Contains(mt, "html") || strings.Contains(mt, "svg") || strings.Contains(mt, "javascript") {
			t.Errorf("default allow-list holds an executable type: %s", mt)
		}
	}
}

func TestLoad_StorageRejectsBadConfig(t *testing.T) {
	s3 := []string{"STORAGE_PROVIDER", "s3", "STORAGE_BUCKET", "b", "STORAGE_ACCESS_KEY_ID", "i", "STORAGE_SECRET_ACCESS_KEY", "s"}
	tests := []struct {
		name string
		env  []string
		want string
	}{
		{"unknown provider", []string{"STORAGE_PROVIDER", "azure"}, "invalid STORAGE_PROVIDER"},
		{"s3 without bucket", []string{"STORAGE_PROVIDER", "s3", "STORAGE_ACCESS_KEY_ID", "i", "STORAGE_SECRET_ACCESS_KEY", "s"}, "STORAGE_BUCKET"},
		{"s3 without key", []string{"STORAGE_PROVIDER", "s3", "STORAGE_BUCKET", "b"}, "STORAGE_ACCESS_KEY_ID"},
		{"gcs without credentials", []string{"STORAGE_PROVIDER", "gcs", "STORAGE_BUCKET", "b"}, "STORAGE_GCS_CREDENTIALS_JSON"},
		{"bad max size", append([]string{"STORAGE_MAX_FILE_BYTES", "0"}, s3...), "STORAGE_MAX_FILE_BYTES"},
		{"max size over one PUT", append([]string{"STORAGE_MAX_FILE_BYTES", "6000000000"}, s3...), "STORAGE_MAX_FILE_BYTES"},
		{"wildcard type", append([]string{"STORAGE_ALLOWED_TYPES", "image/*"}, s3...), "STORAGE_ALLOWED_TYPES"},
		{"type with parameters", append([]string{"STORAGE_ALLOWED_TYPES", "text/plain;charset=utf-8"}, s3...), "STORAGE_ALLOWED_TYPES"},
		{"empty type list", append([]string{"STORAGE_ALLOWED_TYPES", " , "}, s3...), "STORAGE_ALLOWED_TYPES"},
		{"signed URLs over an hour", append([]string{"STORAGE_UPLOAD_URL_TTL", "2h"}, s3...), "STORAGE_UPLOAD_URL_TTL"},
		{"not a duration", append([]string{"STORAGE_DOWNLOAD_URL_TTL", "soon"}, s3...), "STORAGE_DOWNLOAD_URL_TTL"},
		{"cleanup before the URL expires", append([]string{"STORAGE_PENDING_TTL", "5m"}, s3...), "must be longer than"},
		{"bad path style", append([]string{"STORAGE_S3_PATH_STYLE", "maybe"}, s3...), "STORAGE_S3_PATH_STYLE"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			storageEnv(t, tc.env...)
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestLoad_StorageIsIgnoredWhileSaaSIsOff(t *testing.T) {
	legacyEnv(t)
	t.Setenv("STORAGE_PROVIDER", "azure")
	if _, err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
}
