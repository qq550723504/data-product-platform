package application_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

type fakeStore struct{}

func (fakeStore) Put(_ context.Context, objectName string, reader io.Reader, size int64, _ string) (string, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	if int64(len(content)) != size {
		return "", fmt.Errorf("size mismatch: got %d want %d", len(content), size)
	}
	return "s3://test-bucket/" + objectName, nil
}

func TestDatasetVersionUploadIsSequentialTraceableAndImmutable(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}

	ctx := context.Background()
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer pool.Close()

	txManager := transaction.NewManager(pool)
	repo := infrastructure.NewPostgresRepository(pool)
	createDataset := application.NewCreateDatasetService(txManager, repo)
	upload := application.NewUploadVersionService(txManager, repo, fakeStore{})
	invalidate := application.NewInvalidateVersionService(txManager, repo)

	workspaceID := uuid.New()
	dataset, err := createDataset.Handle(ctx, application.CreateDatasetCommand{
		WorkspaceID: workspaceID,
		Code:        "TEST-RAW-" + uuid.NewString(),
		Name:        "Integration RAW dataset",
		DatasetType: domain.DatasetTypeRaw,
		TraceID:     "test-trace",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}

	csvV1 := []byte("source_id,name\n1,alpha\n2,beta\n")
	v1, err := upload.Handle(ctx, application.UploadVersionCommand{
		DatasetID:   dataset.ID,
		Filename:    "enterprise.csv",
		ContentType: "text/csv",
		Content:     csvV1,
		TraceID:     "test-trace",
	})
	if err != nil {
		t.Fatalf("upload v1: %v", err)
	}
	if v1.VersionNo != 1 || v1.Status != domain.VersionReady {
		t.Fatalf("v1 = version %d status %s, want 1 READY", v1.VersionNo, v1.Status)
	}
	if v1.RowCount == nil || *v1.RowCount != 2 {
		t.Fatalf("v1 row count = %v, want 2", v1.RowCount)
	}
	if v1.ChecksumAlgorithm != "SHA256" || v1.ChecksumValue == "" {
		t.Fatalf("v1 checksum metadata is incomplete")
	}
	if v1.StorageURI == "" {
		t.Fatal("v1 storage URI is empty")
	}

	csvV2 := bytes.ReplaceAll(csvV1, []byte("beta"), []byte("beta-fixed"))
	v2, err := upload.Handle(ctx, application.UploadVersionCommand{
		DatasetID:   dataset.ID,
		Filename:    "enterprise.csv",
		ContentType: "text/csv",
		Content:     csvV2,
		TraceID:     "test-trace",
	})
	if err != nil {
		t.Fatalf("upload v2: %v", err)
	}
	if v2.VersionNo != 2 || v2.Status != domain.VersionReady {
		t.Fatalf("v2 = version %d status %s, want 2 READY", v2.VersionNo, v2.Status)
	}

	storedV1, err := repo.GetVersion(ctx, v1.ID)
	if err != nil {
		t.Fatalf("get v1: %v", err)
	}
	if storedV1.Status != domain.VersionSuperseded {
		t.Fatalf("v1 status = %s, want SUPERSEDED", storedV1.Status)
	}

	if _, err := pool.Exec(ctx, `UPDATE dataset_version SET checksum_value = 'tampered' WHERE id = $1`, v2.ID); err == nil {
		t.Fatal("expected READY dataset_version content update to be rejected")
	}

	invalidated, err := invalidate.Handle(ctx, application.InvalidateVersionCommand{
		VersionID: v2.ID,
		Reason:    "integration test invalidation",
		TraceID:   "test-trace",
	})
	if err != nil {
		t.Fatalf("invalidate v2: %v", err)
	}
	if invalidated.Status != domain.VersionInvalid {
		t.Fatalf("invalidated status = %s, want INVALID", invalidated.Status)
	}

	var eventCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_event WHERE aggregate_id = $1 AND event_type = 'DatasetVersionCreated'`, v2.ID).Scan(&eventCount); err != nil {
		t.Fatalf("count outbox events: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("DatasetVersionCreated events = %d, want 1", eventCount)
	}

	var auditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_event WHERE object_id = $1 AND action = 'DATASET_VERSION_READY'`, v2.ID).Scan(&auditCount); err != nil {
		t.Fatalf("count audit events: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("DATASET_VERSION_READY audit events = %d, want 1", auditCount)
	}
}
