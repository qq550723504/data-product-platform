package application

import (
	"context"
	"encoding/json"
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

func pilotSchemaSpec(labels ...string) annotationdomain.FrozenSpec {
	content, _ := json.Marshal(pilotSchemaContract{Kind: pilotSchemaKind, Labels: labels})
	return annotationdomain.FrozenSpec{
		Ref: "activity-record-review", Version: "1.0.0",
		ContentSHA256: sha256HexBytes(content), ContentSnapshot: string(content),
	}
}

func pilotRendererSpec(fields ...string) annotationdomain.FrozenSpec {
	content, _ := json.Marshal(pilotRendererContract{Kind: pilotRendererKind, Fields: fields})
	return annotationdomain.FrozenSpec{
		Ref: "activity-record-renderer", Version: "1.0.0",
		ContentSHA256: sha256HexBytes(content), ContentSnapshot: string(content),
	}
}

type fakeActivationEntitlementChecker struct{}

func (fakeActivationEntitlementChecker) CheckCurrentEntitlementTx(
	context.Context,
	pgx.Tx,
	rightsdomain.EntitlementRequest,
) (rightsdomain.EntitlementDecision, error) {
	return rightsdomain.EntitlementDecision{Decision: rightsdomain.DecisionAllowed}, nil
}

type capturingActivationEntitlementChecker struct {
	requests []rightsdomain.EntitlementRequest
}

func (f *capturingActivationEntitlementChecker) CheckCurrentEntitlementTx(
	_ context.Context,
	_ pgx.Tx,
	request rightsdomain.EntitlementRequest,
) (rightsdomain.EntitlementDecision, error) {
	f.requests = append(f.requests, request)
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
	renderer := pilotRendererSpec("name", "value")
	campaign := annotationdomain.Campaign{
		ID: campaignID, WorkspaceID: workspaceID, InputDatasetVersionID: versionID,
		Schema: pilotSchemaSpec("A", "B"), Renderer: renderer,
	}
	task1Hash, err := rendererTaskTextSHA256(renderer, map[string]string{"name": "alpha", "value": "1"})
	if err != nil {
		t.Fatalf("task 1 render hash: %v", err)
	}
	task2Hash, err := rendererTaskTextSHA256(renderer, map[string]string{"name": "beta", "value": "2"})
	if err != nil {
		t.Fatalf("task 2 render hash: %v", err)
	}
	tasks := []annotationdomain.Task{
		{
			ID: uuid.New(), WorkspaceID: workspaceID, CampaignID: campaignID,
			SourceItemRef: "row:1", SourceContentSHA256: row1,
			TaskTextSHA256: task1Hash, PrimaryAnnotatorRef: "annotator",
		},
		{
			ID: uuid.New(), WorkspaceID: workspaceID, CampaignID: campaignID,
			SourceItemRef: "row:2", SourceContentSHA256: row2,
			TaskTextSHA256: task2Hash, PrimaryAnnotatorRef: "annotator",
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
	renderer := pilotRendererSpec("name")
	taskTextHash, hashErr := rendererTaskTextSHA256(renderer, map[string]string{"name": "alpha"})
	if hashErr != nil {
		t.Fatalf("render task hash: %v", hashErr)
	}
	_, err := guard.Preflight(t.Context(), annotationdomain.Campaign{
		ID: campaignID, WorkspaceID: workspaceID, InputDatasetVersionID: versionID,
		Schema: pilotSchemaSpec("A"), Renderer: renderer,
	}, []annotationdomain.Task{{
		ID: uuid.New(), WorkspaceID: workspaceID, CampaignID: campaignID,
		SourceItemRef: "row:1", SourceContentSHA256: strings.Repeat("0", 64),
		TaskTextSHA256: taskTextHash, PrimaryAnnotatorRef: "annotator",
	}})
	if err == nil || !strings.Contains(err.Error(), "source hash does not match") {
		t.Fatalf("Preflight error = %v, want source hash mismatch", err)
	}
}

func TestCoreActivationGuardPreflightRejectsTaskTextMismatch(t *testing.T) {
	workspaceID := uuid.New()
	datasetID := uuid.New()
	versionID := uuid.New()
	resourceID := uuid.New()
	campaignID := uuid.New()
	csv := "name\nalpha\n"
	rowCount := int64(1)
	byteSize := int64(len(csv))
	renderer := pilotRendererSpec("name")
	rowHash, err := normalizedRowHash(map[string]string{"name": "alpha"})
	if err != nil {
		t.Fatalf("row hash: %v", err)
	}
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
	_, err = guard.Preflight(t.Context(), annotationdomain.Campaign{
		ID: campaignID, WorkspaceID: workspaceID, InputDatasetVersionID: versionID,
		Schema: pilotSchemaSpec("A"), Renderer: renderer,
	}, []annotationdomain.Task{{
		ID: uuid.New(), WorkspaceID: workspaceID, CampaignID: campaignID,
		SourceItemRef: "row:1", SourceContentSHA256: rowHash,
		TaskTextSHA256: strings.Repeat("f", 64), PrimaryAnnotatorRef: "annotator",
	}})
	if err == nil || !strings.Contains(err.Error(), "text hash does not match frozen renderer") {
		t.Fatalf("Preflight error = %v, want renderer task-text mismatch", err)
	}
}

func TestCoreActivationGuardAlwaysRequiresProcessEntitlement(t *testing.T) {
	checker := &capturingActivationEntitlementChecker{}
	guard := &CoreActivationGuard{entitlements: checker}
	resourceID := uuid.New()
	campaign := annotationdomain.Campaign{
		WorkspaceID: uuid.New(),
		Purpose:     "gold-pilot",
		Action:      "READ",
		ConsumerRef: "consumer",
	}

	if err := guard.checkCurrentResourceEntitlement(t.Context(), nil, campaign, resourceID); err != nil {
		t.Fatalf("checkCurrentResourceEntitlement: %v", err)
	}
	if len(checker.requests) != 1 {
		t.Fatalf("entitlement request count = %d, want 1", len(checker.requests))
	}
	if checker.requests[0].Action != "PROCESS" {
		t.Fatalf("entitlement action = %q, want PROCESS", checker.requests[0].Action)
	}
}

