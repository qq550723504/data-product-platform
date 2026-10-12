package storage

import (
	"context"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"time"
)

// This wiring exposes no native mutation method. Deployment uses a separate
// read-only IAM principal and the recovery journal additionally rejects COPY.
type LifecycleCopyRecovery struct {
	copier    *LifecycleCopier
	inspector *LifecycleInspector
}

var _ lc.CopyRecoveryPort = (*LifecycleCopyRecovery)(nil)

func NewLifecycleCopyRecovery(c LifecycleCopyConfig) (*LifecycleCopyRecovery, error) {
	s, e := NewLifecycleCopier(c)
	if e != nil {
		return nil, e
	}
	i, e := NewLifecycleInspector(LifecycleInspectionConfig{Endpoint: c.Endpoint, Secure: c.Secure, AccessKey: c.AccessKey, SecretKey: c.SecretKey, WorkspaceID: c.WorkspaceID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespaces: c.Namespaces, Journal: c.Journal})
	if e != nil {
		return nil, e
	}
	return &LifecycleCopyRecovery{s, i}, nil
}
func (s *LifecycleCopyRecovery) InspectRetentionAndHold(ctx context.Context, c domain.Copy) (lc.CopyProtection, error) {
	return s.inspector.InspectRetentionAndHold(ctx, c)
}
func (s *LifecycleCopyRecovery) VerifyCopyBytes(ctx context.Context, o domain.CopyOperation, c domain.Copy) (lc.CopyBytesProof, error) {
	return s.copier.VerifyCopyBytes(ctx, o, c)
}
func (s *LifecycleCopyRecovery) DiscoverCopyTarget(ctx context.Context, o domain.CopyOperation) (lc.CopyDiscoveryProof, error) {
	var p lc.CopyDiscoveryProof
	if !s.copier.allowed(o) {
		return p, lc.ErrStorageScope
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	call := &custodyCall{op: o, copy: o.Source, operation: lc.StorageRecoverCopyVersions, id: uuid.New()}
	listctx := context.WithValue(ctx, copyCallKey{}, call)
	count := 0
	version := ""
	valid := true
	var listErr error
	for info := range s.copier.client.ListObjects(listctx, o.TargetNamespace, minio.ListObjectsOptions{WithVersions: true, Recursive: true, MaxKeys: 1000}) {
		if info.Err != nil {
			listErr = inspectionError(info.Err)
			continue
		}
		if info.Key == o.TargetKey {
			count++
			version = info.VersionID
			valid = valid && !info.IsDeleteMarker && info.Size == o.Source.ByteSize && version != "" && version != "null"
		}
	}
	if listErr != nil {
		return p, listErr
	}
	obs := call.result()
	if obs.HTTPStatus != 200 || obs.Failure != "" || obs.ResponseSHA256 == "" {
		return p, lc.ErrStorageUnknown
	}
	p.InventoryCallID = call.id
	p.ObservedAt = obs.ObservedAt
	if count != 1 || !valid {
		return p, lc.ErrCopyTargetUnknown
	}
	target := o.Materialized(version)
	head := &custodyCall{op: o, copy: target, operation: lc.StorageRecoverCopyExact, id: uuid.New()}
	info, e := s.copier.client.StatObject(context.WithValue(ctx, copyCallKey{}, head), o.TargetNamespace, o.TargetKey, minio.StatObjectOptions{VersionID: version})
	if e != nil {
		return p, inspectionError(e)
	}
	obs = head.result()
	if info.VersionID != version || info.IsDeleteMarker || info.Size != target.ByteSize || obs.CopyOperationTag != o.ID || obs.CopyResultVersion != version || obs.CopyResultBytes == nil || *obs.CopyResultBytes != target.ByteSize {
		return p, lc.ErrCopyTargetUnknown
	}
	p.Copy = target
	p.StatCallID = head.id
	p.OperationTag = obs.CopyOperationTag
	p.ObservedAt = obs.ObservedAt
	return p, nil
}
