package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7/pkg/signer"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
)

// Existing OSS signer and bounded single-send transport, without Get, COPY,
// DELETE or overwrite. Exact output slots are frozen by the application journal.
type LifecycleOutputWriter struct{ backend *LifecycleCopier }

func NewLifecycleOutputWriter(c LifecycleCopyConfig) (*LifecycleOutputWriter, error) {
	b, e := NewLifecycleCopier(c)
	if e != nil {
		return nil, e
	}
	return &LifecycleOutputWriter{b}, nil
}
func (s *LifecycleOutputWriter) PutPrepared(ctx context.Context, i lc.ProcessingOutputIntent, content []byte) (string, uuid.UUID, error) {
	sum := sha256.Sum256(content)
	if s == nil || int64(len(content)) != i.ByteSize || hex.EncodeToString(sum[:]) != i.ContentSHA256 {
		return "", uuid.Nil, lc.ErrStorageScope
	}
	call := &custodyCall{copy: i.Source, operation: lc.StorageProcessingPut, id: uuid.New(), outputIntent: &i}
	ctx = context.WithValue(ctx, copyCallKey{}, call)
	u := *s.backend.transport.endpoint
	u.Path = "/" + i.Namespace + "/" + i.Key
	req, e := http.NewRequestWithContext(ctx, "PUT", u.String(), bytes.NewReader(content))
	if e != nil {
		return "", call.id, lc.ErrStorageScope
	}
	req.Header.Set("If-None-Match", "*")
	req.Header.Set("X-Amz-Content-Sha256", i.ContentSHA256)
	req.Header.Set("Content-Type", "text/csv")
	req = signer.SignV4(*req, s.backend.config.AccessKey, s.backend.config.SecretKey, "", "us-east-1")
	client := http.Client{Transport: s.backend.transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("output redirect denied") }}
	response, e := client.Do(req)
	if e != nil {
		if call.result().CallID==uuid.Nil{return "",call.id,e}
		return "", call.id, lc.ErrStorageUnknown
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return "", call.id, lc.ErrStorageDenied
	}
	version := response.Header.Get("X-Amz-Version-Id")
	if version == "" || version == "null" {
		return "", call.id, lc.ErrStorageUnknown
	}
	return version, call.id, nil
}
