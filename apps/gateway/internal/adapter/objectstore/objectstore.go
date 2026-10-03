// Package objectstore is the outbound adapter that puts go-packages/storage behind the domain's
// ObjectStore port: it picks the provider from configuration (GCS or an S3-compatible service),
// translates the library's types into core's, and nothing else. Core never imports the library.
package objectstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bernardoforcillo/ignition/go-packages/storage"

	"github.com/bernardoforcillo/ignition/apps/gateway/internal/config"
	"github.com/bernardoforcillo/ignition/apps/gateway/internal/core"
)

// Store implements core.ObjectStore over a storage.Store.
type Store struct{ store storage.Store }

var _ core.ObjectStore = (*Store)(nil)

// New builds the provider named in cfg. cfg must not be nil: a disabled provider has no config,
// and the caller then passes no store at all.
func New(cfg config.Storage) (*Store, error) {
	switch cfg.Provider {
	case config.StorageGCS:
		st, err := storage.NewGCS(storage.GCSConfig{
			Bucket: cfg.Bucket, CredentialsJSON: cfg.GCSCredentialsJSON, Endpoint: cfg.GCSEndpoint,
		})
		if err != nil {
			return nil, err
		}
		return &Store{store: st}, nil
	case config.StorageS3:
		st, err := storage.NewS3(storage.S3Config{
			Bucket: cfg.Bucket, Region: cfg.Region, Endpoint: cfg.Endpoint, PathStyle: cfg.PathStyle,
			AccessKeyID: cfg.AccessKeyID, SecretAccessKey: cfg.SecretAccessKey,
		})
		if err != nil {
			return nil, err
		}
		return &Store{store: st}, nil
	}
	return nil, fmt.Errorf("objectstore: unsupported provider %q", cfg.Provider)
}

// Wrap adapts an already built storage.Store (tests build one against storagetest.Server).
func Wrap(s storage.Store) *Store { return &Store{store: s} }

func toSigned(s storage.Signed) core.SignedRequest {
	return core.SignedRequest{URL: s.URL, Method: s.Method, Headers: s.Headers, ExpiresAt: s.ExpiresAt}
}

func (s *Store) SignUpload(ctx context.Context, key, contentType string, size int64, ttl time.Duration) (core.SignedRequest, error) {
	signed, err := s.store.PresignPut(ctx, key, storage.PutOptions{ContentType: contentType, Size: size, Expires: ttl})
	if err != nil {
		return core.SignedRequest{}, err
	}
	return toSigned(signed), nil
}

func (s *Store) SignDownload(ctx context.Context, key, filename string, ttl time.Duration) (core.SignedRequest, error) {
	signed, err := s.store.PresignGet(ctx, key, storage.GetOptions{Expires: ttl, Filename: filename})
	if err != nil {
		return core.SignedRequest{}, err
	}
	return toSigned(signed), nil
}

func (s *Store) Stat(ctx context.Context, key string) (core.ObjectInfo, error) {
	obj, err := s.store.Stat(ctx, key)
	if errors.Is(err, storage.ErrNotFound) {
		return core.ObjectInfo{}, core.ErrObjectNotFound
	}
	if err != nil {
		return core.ObjectInfo{}, err
	}
	return core.ObjectInfo{Size: obj.Size, ContentType: obj.ContentType}, nil
}

func (s *Store) Delete(ctx context.Context, key string) error { return s.store.Delete(ctx, key) }
