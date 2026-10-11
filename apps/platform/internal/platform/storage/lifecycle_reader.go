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

// The read-only wrapper reuses the existing bounded, single-send transport and
// MinIO OSS signer. No arbitrary URI, mutation, presigned link or retry API.
type LifecycleReader struct {
	backend *LifecycleCopier
	copies  map[uuid.UUID]domain.Copy
}

var _ lc.PinnedReaderPort = (*LifecycleReader)(nil)

func NewLifecycleReader(c LifecycleCopyConfig, copies []domain.Copy) (*LifecycleReader, error) {
	if len(copies) == 0 || len(copies) > 16 {
		return nil, lc.ErrStorageScope
	}
	s := &LifecycleReader{copies: map[uuid.UUID]domain.Copy{}}
	for _, copy := range copies {
		allowed := false
		for _, ns := range c.Namespaces {
			allowed = allowed || copy.Namespace == ns
		}
		if copy.Validate() != nil || copy.WorkspaceID != c.WorkspaceID || copy.ProviderInstanceID != c.ProviderInstanceID || copy.IncarnationID != c.IncarnationID || copy.ByteSize > 128<<10 || !allowed || s.copies[copy.ID].ID != uuid.Nil {
			return nil, lc.ErrStorageScope
		}
		s.copies[copy.ID] = copy
	}
	b, e := NewLifecycleCopier(c)
	if e != nil {
		return nil, e
	}
	s.backend = b
	return s, nil
}

func (s *LifecycleReader) ReadExactPinned(ctx context.Context, c domain.Copy, pin domain.ReaderPin) (lc.PinnedReadResult, error) {
	result := lc.PinnedReadResult{Copy: c, PinID: pin.ID}
	if s == nil || c.Validate() != nil || pin.Validate() != nil || pin.CopyID != c.ID || pin.WorkspaceID != c.WorkspaceID || s.copies[c.ID] != c {
		result.StoppedAt = time.Now().UTC()
		return result, lc.ErrStorageScope
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	call := &custodyCall{copy: c, operation: lc.StorageReadPinned, id: uuid.New(), readerPin: &pin}
	result.CallID = call.id
	ctx = context.WithValue(ctx, copyCallKey{}, call)
	u := *s.backend.transport.endpoint
	u.Path = "/" + c.Namespace + "/" + c.Key
	u.RawQuery = url.Values{"versionId": []string{c.ProviderVersion}}.Encode()
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if e != nil {
		result.StoppedAt = time.Now().UTC()
		return result, lc.ErrStorageScope
	}
	empty := sha256.Sum256(nil)
	req.Header.Set("X-Amz-Content-Sha256", hex.EncodeToString(empty[:]))
	req = signer.SignV4(*req, s.backend.config.AccessKey, s.backend.config.SecretKey, "", "us-east-1")
	client := &http.Client{Transport: s.backend.transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("reader redirects unsupported") }}
	resp, e := client.Do(req)
	call.mu.Lock()
	result.StoppedAt = call.stoppedAt
	call.mu.Unlock()
	if e != nil {
		return result, lc.ErrStorageUnknown
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return result, lc.ErrStorageDenied
	}
	result.Bytes, e = io.ReadAll(io.LimitReader(resp.Body, c.ByteSize+1))
	if e != nil || int64(len(result.Bytes)) != c.ByteSize {
		return result, lc.ErrStorageContract
	}
	return result, nil
}
