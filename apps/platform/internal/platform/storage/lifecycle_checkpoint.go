package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7/pkg/signer"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

// Exact maintenance reads share the OSS signer and single-send custody transport.
// Construction takes a server-owned closed inventory; no public URI/read API.
type LifecycleCheckpointReader struct{ reader *LifecycleReader }

func NewLifecycleCheckpointReader(c LifecycleCopyConfig, copies []domain.Copy) (*LifecycleCheckpointReader, error) {
	r, e := NewLifecycleReader(c, copies)
	if e != nil {
		return nil, e
	}
	return &LifecycleCheckpointReader{r}, nil
}
func (s *LifecycleCheckpointReader) ReadExact(ctx context.Context, c domain.Copy) ([]byte, uuid.UUID, error) {
	if s == nil || s.reader == nil || s.reader.copies[c.ID] != c {
		return nil, uuid.Nil, lc.ErrStorageScope
	}
	b := s.reader.backend
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	call := &custodyCall{copy: c, operation: lc.StorageReadCheckpoint, id: uuid.New()}
	ctx = context.WithValue(ctx, copyCallKey{}, call)
	u := *b.transport.endpoint
	u.Path = "/" + c.Namespace + "/" + c.Key
	u.RawQuery = url.Values{"versionId": {c.ProviderVersion}}.Encode()
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if e != nil {
		return nil, call.id, e
	}
	empty := sha256.Sum256(nil)
	req.Header.Set("X-Amz-Content-Sha256", hex.EncodeToString(empty[:]))
	req = signer.SignV4(*req, b.config.AccessKey, b.config.SecretKey, "", "us-east-1")
	client := &http.Client{Transport: b.transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("checkpoint redirects unsupported") }}
	response, e := client.Do(req)
	if e != nil {
		return nil, call.id, lc.ErrStorageUnknown
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, call.id, lc.ErrStorageDenied
	}
	data, e := io.ReadAll(io.LimitReader(response.Body, c.ByteSize+1))
	if e != nil || int64(len(data)) != c.ByteSize {
		return nil, call.id, lc.ErrStorageContract
	}
	return data, call.id, nil
}
