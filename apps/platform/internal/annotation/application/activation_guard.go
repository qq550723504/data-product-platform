package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/tabular"
	rightsdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
)

const maxAnnotationPilotInputBytes int64 = 64 << 20

type ActivationDatasetReader interface {
	GetVersion(ctx context.Context, versionID uuid.UUID) (datasetdomain.DatasetVersion, error)
	GetVersionTx(ctx context.Context, tx pgx.Tx, versionID uuid.UUID) (datasetdomain.DatasetVersion, error)
}

type ActivationContextReader interface {
	DatasetAnnotationContext(ctx context.Context, datasetID uuid.UUID) (workspaceID, sourceResourceID uuid.UUID, err error)
	DatasetAnnotationContextTx(ctx context.Context, tx pgx.Tx, datasetID uuid.UUID) (workspaceID, sourceResourceID uuid.UUID, err error)
	ValidateCurrentInputCertificationTx(
		ctx context.Context,
		tx pgx.Tx,
		workspaceID, certificationID, datasetVersionID uuid.UUID,
	) error
}

type ActivationObjectReader interface {
	Get(ctx context.Context, storageURI string) (io.ReadCloser, error)
}

type ActivationEntitlementChecker interface {
	CheckCurrentEntitlementTx(
		ctx context.Context,
		tx pgx.Tx,
		request rightsdomain.EntitlementRequest,
	) (rightsdomain.EntitlementDecision, error)
}

type CoreActivationGuard struct {
	datasets     ActivationDatasetReader
	contexts     ActivationContextReader
	objects      ActivationObjectReader
	entitlements ActivationEntitlementChecker
}

func NewCoreActivationGuard(
	datasets ActivationDatasetReader,
	contexts ActivationContextReader,
	objects ActivationObjectReader,
	entitlements ActivationEntitlementChecker,
) *CoreActivationGuard {
	return &CoreActivationGuard{
		datasets: datasets, contexts: contexts, objects: objects, entitlements: entitlements,
	}
}

func (g *CoreActivationGuard) Preflight(
	ctx context.Context,
	campaign annotationdomain.Campaign,
	tasks []annotationdomain.Task,
) (ActivationProof, error) {
	if g == nil || g.datasets == nil || g.contexts == nil || g.objects == nil || g.entitlements == nil {
		return ActivationProof{}, ErrActivationGuardRequired
	}
	version, err := g.datasets.GetVersion(ctx, campaign.InputDatasetVersionID)
	if err != nil {
		return ActivationProof{}, err
	}
	if version.Status != datasetdomain.VersionReady ||
		strings.TrimSpace(version.StorageURI) == "" ||
		!strings.EqualFold(strings.TrimSpace(version.ChecksumAlgorithm), "SHA256") ||
		len(strings.TrimSpace(version.ChecksumValue)) != 64 {
		return ActivationProof{}, fmt.Errorf("annotation input DatasetVersion is not a checksum-verifiable READY version")
	}
	workspaceID, sourceResourceID, err := g.contexts.DatasetAnnotationContext(ctx, version.DatasetID)
	if err != nil {
		return ActivationProof{}, err
	}
	if workspaceID != campaign.WorkspaceID || sourceResourceID == uuid.Nil {
		return ActivationProof{}, fmt.Errorf("annotation input dataset context does not match campaign")
	}
	if version.RowCount == nil || *version.RowCount <= 0 || int64(len(tasks)) != *version.RowCount {
		return ActivationProof{}, fmt.Errorf("annotation task count does not cover input row count")
	}
	if version.ByteSize != nil && *version.ByteSize > maxAnnotationPilotInputBytes {
		return ActivationProof{}, fmt.Errorf("annotation Pilot input exceeds %d bytes", maxAnnotationPilotInputBytes)
	}

	reader, err := g.objects.Get(ctx, version.StorageURI)
	if err != nil {
		return ActivationProof{}, err
	}
	defer reader.Close()

	limit := maxAnnotationPilotInputBytes
	if version.ByteSize != nil && *version.ByteSize >= 0 && *version.ByteSize < limit {
		limit = *version.ByteSize
	}
	payload, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return ActivationProof{}, fmt.Errorf("read annotation input object: %w", err)
	}
	if int64(len(payload)) > limit {
		return ActivationProof{}, fmt.Errorf("annotation input object exceeds verified byte limit")
	}
	if version.ByteSize != nil && int64(len(payload)) != *version.ByteSize {
		return ActivationProof{}, fmt.Errorf("annotation input object byte size changed")
	}
	inputChecksum := sha256HexBytes(payload)
	if !strings.EqualFold(inputChecksum, strings.TrimSpace(version.ChecksumValue)) {
		return ActivationProof{}, fmt.Errorf("annotation input object checksum changed")
	}

	if !isCSVContentType(version.ContentType) {
		return ActivationProof{}, fmt.Errorf("annotation Pilot currently requires CSV input")
	}
	table, err := tabular.ReadCSV(bytes.NewReader(payload))
	if err != nil {
		return ActivationProof{}, fmt.Errorf("read annotation Pilot CSV: %w", err)
	}
	if len(table.Rows) != len(tasks) || int64(len(table.Rows)) != *version.RowCount {
		return ActivationProof{}, fmt.Errorf("annotation task membership does not cover exact CSV row set")
	}
	taskByRef := make(map[string]annotationdomain.Task, len(tasks))
	for _, task := range tasks {
		if task.WorkspaceID != campaign.WorkspaceID || task.CampaignID != campaign.ID {
			return ActivationProof{}, fmt.Errorf("annotation task crosses campaign boundary")
		}
		if _, exists := taskByRef[task.SourceItemRef]; exists {
			return ActivationProof{}, fmt.Errorf("annotation task source item ref is duplicated")
		}
		taskByRef[task.SourceItemRef] = task
	}
	for i, row := range table.Rows {
		ref := fmt.Sprintf("row:%d", i+1)
		task, ok := taskByRef[ref]
		if !ok {
			return ActivationProof{}, fmt.Errorf("annotation task manifest is missing %s", ref)
		}
		rowHash, err := normalizedRowHash(row)
		if err != nil {
			return ActivationProof{}, err
		}
		if task.SourceContentSHA256 != rowHash {
			return ActivationProof{}, fmt.Errorf("annotation task %s source hash does not match input row", ref)
		}
	}
	manifestHash, err := taskManifestHash(tasks)
	if err != nil {
		return ActivationProof{}, err
	}
	return ActivationProof{
		TaskManifestHash: manifestHash,
		TaskCount:        len(tasks),
		InputChecksum:    inputChecksum,
		InputStorageURI:  version.StorageURI,
		InputByteSize:    int64(len(payload)),
		InputRowCount:    *version.RowCount,
		InputContentType: strings.TrimSpace(version.ContentType),
		SourceResourceID: sourceResourceID,
	}, nil
}

func (g *CoreActivationGuard) ValidateActivationTx(
	ctx context.Context,
	tx pgx.Tx,
	campaign annotationdomain.Campaign,
	proof ActivationProof,
) error {
	if g == nil || g.datasets == nil || g.contexts == nil || g.entitlements == nil {
		return ErrActivationGuardRequired
	}
	if _, err := deliveryfence.Lock(ctx, tx, campaign.WorkspaceID); err != nil {
		return err
	}
	version, err := g.datasets.GetVersionTx(ctx, tx, campaign.InputDatasetVersionID)
	if err != nil {
		return err
	}
	if version.Status != datasetdomain.VersionReady ||
		!strings.EqualFold(strings.TrimSpace(version.ChecksumAlgorithm), "SHA256") ||
		!strings.EqualFold(strings.TrimSpace(version.ChecksumValue), proof.InputChecksum) ||
		strings.TrimSpace(version.StorageURI) != proof.InputStorageURI ||
		strings.TrimSpace(version.ContentType) != proof.InputContentType ||
		version.ByteSize == nil || *version.ByteSize != proof.InputByteSize ||
		version.RowCount == nil || *version.RowCount != proof.InputRowCount ||
		proof.InputRowCount != int64(proof.TaskCount) {
		return fmt.Errorf("annotation input DatasetVersion identity changed after preflight")
	}
	workspaceID, sourceResourceID, err := g.contexts.DatasetAnnotationContextTx(ctx, tx, version.DatasetID)
	if err != nil {
		return err
	}
	if workspaceID != campaign.WorkspaceID || sourceResourceID != proof.SourceResourceID {
		return fmt.Errorf("annotation input dataset context changed after preflight")
	}
	if err := g.contexts.ValidateCurrentInputCertificationTx(
		ctx, tx, campaign.WorkspaceID, campaign.InputCertificationID, campaign.InputDatasetVersionID,
	); err != nil {
		return err
	}
	for _, resourceID := range []uuid.UUID{sourceResourceID, campaign.AnnotationContributionID} {
		if err := g.checkCurrentResourceEntitlement(ctx, tx, campaign, resourceID); err != nil {
			return err
		}
	}
	return nil
}

func (g *CoreActivationGuard) checkCurrentResourceEntitlement(
	ctx context.Context,
	tx pgx.Tx,
	campaign annotationdomain.Campaign,
	resourceID uuid.UUID,
) error {
	if resourceID == uuid.Nil {
		return rightsdomain.ErrEntitlementBlocked
	}
	scope, err := rightsdomain.NewNormalizedScope("ALL_RESOURCE", resourceID.String())
	if err != nil {
		return err
	}
	request := rightsdomain.EntitlementRequest{
		WorkspaceID:    campaign.WorkspaceID,
		DataResourceID: resourceID,
		ConsumerRef:    strings.TrimSpace(campaign.ConsumerRef),
		Purpose:        strings.TrimSpace(campaign.Purpose),
		Action:         strings.TrimSpace(campaign.Action),
		Scope:          scope,
		Path:           rightsdomain.EntitlementDirectUse,
	}
	decision, err := g.entitlements.CheckCurrentEntitlementTx(ctx, tx, request)
	if err != nil {
		return err
	}
	if decision.Decision == rightsdomain.DecisionAllowed {
		return nil
	}
	request.Path = rightsdomain.EntitlementDownstream
	decision, err = g.entitlements.CheckCurrentEntitlementTx(ctx, tx, request)
	if err != nil {
		return err
	}
	if decision.Decision != rightsdomain.DecisionAllowed {
		return rightsdomain.ErrEntitlementBlocked
	}
	return nil
}

func normalizedRowHash(row map[string]string) (string, error) {
	encoded, err := json.Marshal(row)
	if err != nil {
		return "", fmt.Errorf("marshal annotation source row: %w", err)
	}
	return sha256HexBytes(encoded), nil
}

func sha256HexBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func isCSVContentType(value string) bool {
	switch strings.ToLower(strings.TrimSpace(strings.Split(value, ";")[0])) {
	case "text/csv", "application/csv", "application/vnd.ms-excel":
		return true
	default:
		return false
	}
}
