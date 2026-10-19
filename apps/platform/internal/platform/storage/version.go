package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
)

// Non-profile compositions still use the same create-only provider primitive.
// The protected runtime adds the shared inventory/publication journal above.
func (s *Store) PutVersion(ctx context.Context, _ domain.DatasetVersion, key string, r io.Reader, n int64, contentType string) (string, error) {
	data, err := io.ReadAll(io.LimitReader(r, (128<<20)+1))
	if err != nil || int64(len(data)) != n {
		return "", fmt.Errorf("version object size mismatch")
	}
	uri := "s3://" + s.bucket + "/" + key
	_, putErr := s.PutImmutable(ctx, key, bytes.NewReader(data), n, contentType)
	if putErr != nil && !errors.Is(putErr, ErrObjectExists) {
		return "", putErr
	}
	actual, err := s.Get(ctx, uri)
	if err != nil {
		return "", err
	}
	stored, err := io.ReadAll(io.LimitReader(actual, n+1))
	closeErr := actual.Close()
	if err != nil || closeErr != nil {
		return "", errors.Join(err, closeErr)
	}
	if int64(len(stored)) != n || sha256.Sum256(stored) != sha256.Sum256(data) {
		return "", fmt.Errorf("immutable version readback mismatch")
	}
	return uri, nil
}
