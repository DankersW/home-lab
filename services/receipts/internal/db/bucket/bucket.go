// Package bucket stores receipt files in an S3-compatible object store (MinIO).
package bucket

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// ErrNotFound is returned when an object key does not exist.
var ErrNotFound = errors.New("bucket: object not found")

// Config describes how to reach the object store.
type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

// Store wraps a MinIO client bound to a single bucket.
type Store struct {
	client *minio.Client
	bucket string
}

// ObjectInfo is the metadata returned alongside a fetched object. We expose a
// concrete struct rather than leaking minio.ObjectInfo.
type ObjectInfo struct {
	Size         int64
	ContentType  string
	ETag         string
	LastModified time.Time
}

// ObjectMeta is a lightweight listing entry used by the backup command.
type ObjectMeta struct {
	Key  string
	Size int64
	ETag string
}

// New constructs a Store, verifies connectivity, and ensures the bucket exists.
// It is the single explicit initialization point (no init()). The app uses the
// MinIO root credentials directly: it is the only tenant of this server, so a
// separate scoped user would add provisioning with no real benefit here.
func New(ctx context.Context, cfg Config) (*Store, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("bucket: new client: %w", err)
	}
	s := &Store{client: client, bucket: cfg.Bucket}
	if err := s.ensureBucket(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) ensureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("bucket: check %q: %w", s.bucket, err)
	}
	if exists {
		return nil
	}
	if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{}); err != nil {
		// Tolerate a concurrent startup that created it first.
		if ex, exErr := s.client.BucketExists(ctx, s.bucket); exErr == nil && ex {
			return nil
		}
		return fmt.Errorf("bucket: make %q: %w", s.bucket, err)
	}
	return nil
}

// Key builds the canonical object key for an attachment. Keeping every
// attachment under its receipt's prefix makes "delete a receipt" a prefix sweep
// and keeps keys collision-free (both IDs are UUIDs).
func Key(receiptID, attachmentID string) string {
	return receiptID + "/" + attachmentID
}

// Put streams an object of known size into the bucket.
func (s *Store) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	if _, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{ContentType: contentType}); err != nil {
		return fmt.Errorf("bucket: put %q: %w", key, err)
	}
	return nil
}

// Get returns a streaming reader plus metadata. The caller must Close rc. A
// missing key maps to ErrNotFound (surfaced here via Stat, not on first Read).
func (s *Store) Get(ctx context.Context, key string) (rc io.ReadCloser, info ObjectInfo, err error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, ObjectInfo{}, fmt.Errorf("bucket: get %q: %w", key, err)
	}
	stat, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		if isNoSuchKey(err) {
			return nil, ObjectInfo{}, fmt.Errorf("bucket: get %q: %w", key, ErrNotFound)
		}
		return nil, ObjectInfo{}, fmt.Errorf("bucket: stat %q: %w", key, err)
	}
	return obj, ObjectInfo{
		Size:         stat.Size,
		ContentType:  stat.ContentType,
		ETag:         stat.ETag,
		LastModified: stat.LastModified,
	}, nil
}

// Remove deletes a single object. Removing a missing key is treated as success.
func (s *Store) Remove(ctx context.Context, key string) error {
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		if isNoSuchKey(err) {
			return nil
		}
		return fmt.Errorf("bucket: remove %q: %w", key, err)
	}
	return nil
}

// List returns metadata for every object in the bucket (used by backup).
func (s *Store) List(ctx context.Context) ([]ObjectMeta, error) {
	var out []ObjectMeta
	for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{Recursive: true}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("bucket: list: %w", obj.Err)
		}
		out = append(out, ObjectMeta{Key: obj.Key, Size: obj.Size, ETag: obj.ETag})
	}
	return out, nil
}

// HealthCheck verifies the bucket is reachable.
func (s *Store) HealthCheck(ctx context.Context) error {
	if _, err := s.client.BucketExists(ctx, s.bucket); err != nil {
		return fmt.Errorf("bucket: health: %w", err)
	}
	return nil
}

func isNoSuchKey(err error) bool {
	resp := minio.ToErrorResponse(err)
	return resp.Code == "NoSuchKey" || resp.StatusCode == 404
}
