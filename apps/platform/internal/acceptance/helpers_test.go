package acceptance_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/google/uuid"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	resourceapp "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/application"
	resourcedomain "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/domain"
)

type memoryStore struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func newMemoryStore() *memoryStore {
	return &memoryStore{objects: map[string][]byte{}}
}

func (s *memoryStore) Put(_ context.Context, objectName string, reader io.Reader, _ int64, _ string) (string, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	uri := "s3://acceptance-bucket/" + objectName
	s.mu.Lock()
	s.objects[uri] = append([]byte(nil), content...)
	s.mu.Unlock()
	return uri, nil
}

func (s *memoryStore) Get(_ context.Context, storageURI string) (io.ReadCloser, error) {
	s.mu.Lock()
	content := append([]byte(nil), s.objects[storageURI]...)
	s.mu.Unlock()
	return io.NopCloser(bytes.NewReader(content)), nil
}

func (s *memoryStore) bytes(storageURI string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.objects[storageURI]...)
}

func mustCreateResource(t *testing.T, ctx context.Context, service *resourceapp.CreateService, workspaceID uuid.UUID, code, name string, actorID *uuid.UUID, traceID string) resourcedomain.DataResource {
	t.Helper()
	resource, err := service.Handle(ctx, resourceapp.CreateDataResourceCommand{
		WorkspaceID:      workspaceID,
		Code:             code + "-" + uuid.NewString(),
		Name:             name,
		DomainCode:       "PARK_ENTERPRISE_ACTIVITY",
		ResourceType:     resourcedomain.ResourceTypeTableLike,
		SensitivityLevel: "INTERNAL",
		ActorID:          actorID,
		TraceID:          traceID,
	})
	if err != nil {
		t.Fatalf("create DataResource %s: %v", code, err)
	}
	return resource
}

func mustCreateDataset(t *testing.T, ctx context.Context, service *datasetapp.CreateDatasetService, workspaceID uuid.UUID, code, name string, datasetType datasetdomain.DatasetType, sourceResourceID *uuid.UUID, actorID *uuid.UUID, traceID string) datasetdomain.Dataset {
	t.Helper()
	dataset, err := service.Handle(ctx, datasetapp.CreateDatasetCommand{
		WorkspaceID:      workspaceID,
		Code:             code + "-" + uuid.NewString(),
		Name:             name,
		DatasetType:      datasetType,
		SourceResourceID: sourceResourceID,
		ActorID:          actorID,
		TraceID:          traceID,
	})
	if err != nil {
		t.Fatalf("create Dataset %s: %v", code, err)
	}
	return dataset
}

func mustUploadFixture(t *testing.T, ctx context.Context, service *datasetapp.UploadVersionService, datasetID uuid.UUID, fixtureName, storedName string, actorID *uuid.UUID, traceID string) datasetdomain.DatasetVersion {
	t.Helper()
	return mustUploadCSV(t, ctx, service, datasetID, storedName, readRepoFile(t, "examples", "enterprise-activity", "data", fixtureName), nil, nil, actorID, traceID)
}

func mustUploadCSV(t *testing.T, ctx context.Context, service *datasetapp.UploadVersionService, datasetID uuid.UUID, filename string, content []byte, metadata map[string]any, executionID *uuid.UUID, actorID *uuid.UUID, traceID string) datasetdomain.DatasetVersion {
	t.Helper()
	version, err := service.Handle(ctx, datasetapp.UploadVersionCommand{
		DatasetID:              datasetID,
		Filename:               filename,
		ContentType:            "text/csv; charset=utf-8",
		Content:                content,
		ActorID:                actorID,
		TraceID:                traceID,
		GeneratedByExecutionID: executionID,
		Metadata:               metadata,
	})
	if err != nil {
		t.Fatalf("upload DatasetVersion %s: %v", filename, err)
	}
	return version
}

func readRepoFile(t *testing.T, parts ...string) []byte {
	t.Helper()
	content, err := os.ReadFile(repoPath(t, parts...))
	if err != nil {
		t.Fatalf("read repository file %v: %v", parts, err)
	}
	return content
}

func repoPath(t *testing.T, parts ...string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test source path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../../"))
	return filepath.Join(append([]string{root}, parts...)...)
}
