package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Store struct {
	client *minio.Client
	bucket string
}

// ErrObjectExists is a failed create-only write, never permission to overwrite.
var ErrObjectExists = errors.New("object already exists")

// PutImmutable uses the provider's atomic create-only primitive. A caller
// recovering a lost response must read and verify the existing bytes itself.
// The ingestion deployment additionally enforces this header and denies delete
// and copy at its sole storage gateway; engine credentials cannot access Core.
func (s *Store) PutImmutable(ctx context.Context, objectName string, reader io.Reader, size int64, contentType string) (string, error) {
	if size <= 0 || size > 128<<20 {
		return "", fmt.Errorf("immutable object size must be in (0, 128 MiB]")
	}
	opts := minio.PutObjectOptions{ContentType: contentType, DisableMultipart: true}
	opts.SetMatchETagExcept("*")
	_, err := s.client.PutObject(ctx, s.bucket, objectName, reader, size, opts)
	if err != nil {
		if response := minio.ToErrorResponse(err); response.Code == "PreconditionFailed" || response.Code == "ConditionalRequestConflict" {
			return "", ErrObjectExists
		}
		return "", fmt.Errorf("create immutable object: %w", err)
	}
	return fmt.Sprintf("s3://%s/%s", s.bucket, objectName), nil
}

func New(endpoint, accessKey, secretKey, bucket string, useSSL bool) (*Store, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("create object storage client: %w", err)
	}

	return &Store{client: client, bucket: bucket}, nil
}

func (s *Store) EnsureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("check bucket %q: %w", s.bucket, err)
	}
	if exists {
		return nil
	}
	if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{}); err != nil {
		return fmt.Errorf("create bucket %q: %w", s.bucket, err)
	}
	return nil
}

func (s *Store) Put(ctx context.Context, objectName string, reader io.Reader, size int64, contentType string) (string, error) {
	_, err := s.client.PutObject(ctx, s.bucket, objectName, reader, size, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return "", fmt.Errorf("put object %q: %w", objectName, err)
	}
	return fmt.Sprintf("s3://%s/%s", s.bucket, objectName), nil
}

func (s *Store) Get(ctx context.Context, storageURI string) (io.ReadCloser, error) {
	bucket, objectName, err := parseStorageURI(storageURI)
	if err != nil {
		return nil, err
	}
	if bucket != s.bucket {
		return nil, fmt.Errorf("storage URI bucket %q does not match configured bucket %q", bucket, s.bucket)
	}

	object, err := s.client.GetObject(ctx, bucket, objectName, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object %q: %w", objectName, err)
	}
	if _, err := object.Stat(); err != nil {
		_ = object.Close()
		return nil, fmt.Errorf("stat object %q: %w", objectName, err)
	}
	return object, nil
}

func parseStorageURI(storageURI string) (string, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(storageURI))
	if err != nil {
		return "", "", fmt.Errorf("parse storage URI: %w", err)
	}
	if parsed.Scheme != "s3" || parsed.Host == "" {
		return "", "", fmt.Errorf("unsupported storage URI %q", storageURI)
	}
	objectName := strings.TrimPrefix(parsed.Path, "/")
	if objectName == "" {
		return "", "", fmt.Errorf("storage URI %q does not contain an object key", storageURI)
	}
	return parsed.Host, objectName, nil
}

func (s *Store) Bucket() string {
	return s.bucket
}
