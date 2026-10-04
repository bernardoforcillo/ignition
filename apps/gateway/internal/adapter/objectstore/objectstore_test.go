package objectstore

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/bernardoforcillo/ignition/go-packages/storage/storagetest"

	"github.com/bernardoforcillo/ignition/apps/gateway/internal/config"
	"github.com/bernardoforcillo/ignition/apps/gateway/internal/core"
)

func TestStore_RoundTripThroughBothProviders(t *testing.T) {
	srv := storagetest.New(t)
	for name, cfg := range map[string]config.Storage{
		"s3": {Provider: config.StorageS3, Bucket: storagetest.Bucket, Region: storagetest.Region, Endpoint: srv.URL,
			PathStyle: true, AccessKeyID: storagetest.AccessKeyID, SecretAccessKey: storagetest.SecretAccessKey},
		"gcs": {Provider: config.StorageGCS, Bucket: storagetest.Bucket, GCSCredentialsJSON: srv.GCSCredentials, GCSEndpoint: srv.URL},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			store, err := New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			key := "workspaces/w/" + name
			if _, err := store.Stat(ctx, key); !errors.Is(err, core.ErrObjectNotFound) {
				t.Fatalf("Stat of nothing = %v, want core.ErrObjectNotFound", err)
			}
			up, err := store.SignUpload(ctx, key, "text/plain", 5, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			req, _ := http.NewRequest(up.Method, up.URL, bytes.NewReader([]byte("hello")))
			for k, v := range up.Headers {
				req.Header.Set(k, v)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil || resp.StatusCode != http.StatusOK {
				t.Fatalf("PUT = %v, %v", resp, err)
			}
			_ = resp.Body.Close()
			info, err := store.Stat(ctx, key)
			if err != nil || info.Size != 5 || info.ContentType != "text/plain" {
				t.Fatalf("Stat = %+v, %v", info, err)
			}
			if dl, err := store.SignDownload(ctx, key, "a.txt", time.Minute); err != nil || dl.Method != http.MethodGet {
				t.Fatalf("SignDownload = %+v, %v", dl, err)
			}
			if err := store.Delete(ctx, key); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Stat(ctx, key); !errors.Is(err, core.ErrObjectNotFound) {
				t.Errorf("Stat after Delete = %v", err)
			}
		})
	}
}

func TestNew_RejectsAnUnknownProvider(t *testing.T) {
	if _, err := New(config.Storage{Provider: "azure"}); err == nil {
		t.Fatal("want an error")
	}
}
