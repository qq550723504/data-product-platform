package application

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	rightsdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
)

type fakeActivationDatasetReader struct {
	version datasetdomain.DatasetVersion
}

func (f fakeActivationDatasetReader) GetVersion(context.Context, uuid.UUID) (datasetdomain.DatasetVersion, error) {
	return f.version, nil
}

func (f fakeActivationDatasetReader) GetVersionTx(context.Context, pgx.Tx, uuid.UUID) (datasetdomain.DatasetVersion, error) {
	return f.version, nil
}

type fakeActivationContextReader struct {
	workspaceID      uuid.UUID
	sourceResourceID uuid.UUID
}

func (f fakeActivationContextReader) DatasetAnnotationContext(context.Context, uuid.UUID) (uuid.UUID, uuid.UUID, error) {
	return f.workspaceID, f.sourceResourceID, nil
}

func (f fakeActivationContextReader) DatasetAnnotationContextTx(context.Context, pgx.Tx, uuid.UUID) (uuid.UUID, uuid.UUID, error) {
	return f.workspaceID, f.sourceResourceID, nil
}

func (f fakeActivationContextReader) ValidateCurrentInputCertificationTx(context.Context, pgx.Tx, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return nil
}

type fakeActivationObjectReader struct {
	content string
}

func (f fakeActivationObjectReader) Get(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(f.content)), nil
}

type fakeActivationEntitlementChecker struct{}

func (fakeActivationEntitlementChecker) CheckCurrentEntitlementTx(
	context.Context,
	pgx.Tx,
	rightsdomain.EntitlementRequest,
) (rightsdomain.EntitlementDecision, error) {
	return rightsdomain.EntitlementDecision{Decision: rightsdomain.DecisionAllowed}, nil
}

func TestCoreActivationGuardPreflightProvesExactCSVTaskMembership(t *testing.T) {
	workspaceID := uuid.New()
	datasetID := uuid.New()
	versionID := uuid.New()
	resourceID := uuid.New()
	campaignID := uuid.New()
	csv := "name,value\nalpha,1\nbeta,2\n"
	row1, err := normalizedRowHash(map[string]string{"name": "alpha", "value": "1"})
	if err != nil {
		t.Fatalf("row 1 hash: %v", err)
	}
	row2, err := normalizedRowHash(map[string]string{"name": "beta", "value": "2"})
	if err != nil {
		t.Fatalf("row 2 hash: %v", err)
	}
	rowCount := int64(2)
	byteSize := int64(len(csv))
	guard := NewCoreActivationGuard(
		fakeActivationDatasetReader{version: datasetdomain.DatasetVersion{
			ID: versionID, DatasetID: datasetID, Status: datasetdomain.VersionReady,
			StorageURI: "s3://fixture/input.csv", ContentType: "text/csv",
			RowCount: &rowCount, ByteSize: &byteSize,
			ChecksumAlgorithm: "SHA256", ChecksumValue: sha256HexBytes([]byte(csv)),
		}},
		fakeActivationContextReader{workspaceID: workspaceID, sourceResourceID: resourceID},
		fakeActivationObjectReader{content: csv},
		fakeActivationEntitlementChecker{},
	)
	campaign := annotationdomain.Campaign{
		ID: campaignID, WorkspaceID: workspaceID, InputDatasetVersionID: versionID,
	}
	tasks := []annotationdomain.Task{
		{
			ID: uuid.New(), WorkspaceID: workspaceID, CampaignID: campaignID,
			SourceItemRef: "row:1", SourceContentSHA256: row1,
			TaskTextSHA256: strings.Repeat("a", 64),
		},
		{
			ID: uuid.New(), WorkspaceID: workspaceID, CampaignID: campaignID,
			SourceItemRef: "row:2", SourceContentSHA256: row2,
			TaskTextSHA256: strings.Repeat("b", 64),
		},
	}
	proof, err := guard.Preflight(t.Context(), campaign, tasks)
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	if proof.TaskCount != 2 || proof.SourceResourceID != resourceID ||
		proof.InputChecksum != sha256HexBytes([]byte(csv)) ||
		proof.InputStorageURI != "s3://fixture/input.csv" ||
		proof.InputByteSize != int64(len(csv)) ||
		proof.InputRowCount != 2 ||
		proof.InputContentType != "text/csv" ||
		proof.TaskManifestHash == "" {
		t.Fatalf("unexpected activation proof: %#v", proof)
	}
}

func TestCoreActivationGuardPreflightRejectsRowHashMismatch(t *testing.T) {
	workspaceID := uuid.New()
	datasetID := uuid.New()
	versionID := uuid.New()
	resourceID := uuid.New()
	campaignID := uuid.New()
	csv := "name\nalpha\n"
	rowCount := int64(1)
	byteSize := int64(len(csv))
	guard := NewCoreActivationGuard(
		fakeActivationDatasetReader{version: datasetdomain.DatasetVersion{
			ID: versionID, DatasetID: datasetID, Status: datasetdomain.VersionReady,
			StorageURI: "s3://fixture/input.csv", ContentType: "text/csv",
			RowCount: &rowCount, ByteSize: &byteSize,
			ChecksumAlgorithm: "SHA256", ChecksumValue: sha256HexBytes([]byte(csv)),
		}},
		fakeActivationContextReader{workspaceID: workspaceID, sourceResourceID: resourceID},
		fakeActivationObjectReader{content: csv},
		fakeActivationEntitlementChecker{},
	)
	_, err := guard.Preflight(t.Context(), annotationdomain.Campaign{
		ID: campaignID, WorkspaceID: workspaceID, InputDatasetVersionID: versionID,
	}, []annotationdomain.Task{{
		ID: uuid.New(), WorkspaceID: workspaceID, CampaignID: campaignID,
		SourceItemRef: "row:1", SourceContentSHA256: strings.Repeat("0", 64),
		TaskTextSHA256: strings.Repeat("a", 64),
	}})
	if err == nil || !strings.Contains(err.Error(), "source hash does not match") {
		t.Fatalf("Preflight error = %v, want source hash mismatch", err)
	}
}
