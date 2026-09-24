package application

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
	annotationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

type EngineSendAuthorizationGuard interface {
	ValidateEngineSendTx(
		ctx context.Context,
		tx pgx.Tx,
		campaign annotationdomain.Campaign,
	) error
}

type EngineService struct {
	tx        *transaction.Manager
	repo      *annotationinfra.Repository
	engine    AnnotationEnginePort
	sendGuard EngineSendAuthorizationGuard
}

func NewEngineService(
	tx *transaction.Manager,
	repo *annotationinfra.Repository,
	engine AnnotationEnginePort,
	sendGuard EngineSendAuthorizationGuard,
) *EngineService {
	return &EngineService{tx: tx, repo: repo, engine: engine, sendGuard: sendGuard}
}

type PrepareEngineCampaignCommand struct {
	WorkspaceID uuid.UUID
	CampaignID  uuid.UUID
	RequestID   string
	Title       string
	ActorID     *uuid.UUID
	TraceID     string
}

type PrepareEngineTasksCommand struct {
	WorkspaceID uuid.UUID
	CampaignID  uuid.UUID
	RequestID   string
	Tasks       []EngineTask
	ActorID     *uuid.UUID
	TraceID     string
}

type engineCampaignManifest struct {
	Kind          string
	WorkspaceID   string
	CampaignID    string
	Title         string
	SchemaContent string
	SchemaSHA256  string
}

type engineTasksManifest struct {
	Kind        string
	WorkspaceID string
	CampaignID  string
	Binding     engineBinding
	Tasks       []EngineTask
}

type engineBinding struct {
	Provider          string
	ProviderInstance  string
	ExternalProjectID string
	RequestID         string
	ConfigSHA256      string
}

type EngineDispatchResult struct {
	Operation annotationdomain.EngineOperation
	AttemptID uuid.UUID
}

type engineResolution struct {
	State           EngineLookupState
	CampaignBinding *EngineCampaignBinding
	ExternalTaskIDs map[uuid.UUID]string
	DiagnosticRef   string
}

func (s *EngineService) configured() error {
	if s == nil || s.tx == nil || s.repo == nil || s.engine == nil {
		return errors.New("annotation engine service is not configured")
	}
	return nil
}

func decodeCampaignManifest(operation annotationdomain.EngineOperation) (engineCampaignManifest, error) {
	var manifest engineCampaignManifest
	err := json.Unmarshal(operation.PayloadManifestHash, &manifest)
	return manifest, err
}

func decodeTasksManifest(operation annotationdomain.EngineOperation) (engineTasksManifest, error) {
	var manifest engineTasksManifest
	err := json.Unmarshal(operation.PayloadManifestHash, &manifest)
	return manifest, err
}

func terminalEngineOperation(status string) bool {
	switch status {
	case annotationdomain.EngineOperationMatched,
		annotationdomain.EngineOperationRejected,
		annotationdomain.EngineOperationConflict,
		annotationdomain.EngineOperationManualResolution:
		return true
	default:
		return false
	}
}

func validLease(lease time.Duration) bool {
	return lease > 0
}

func engineLookupRequest(request EngineSubmitRequest) EngineLookupRequest {
	return EngineLookupRequest{
		WorkspaceID:        request.WorkspaceID,
		CampaignID:         request.CampaignID,
		Binding:            request.Binding,
		RequestID:          request.RequestID,
		RequestFingerprint: request.RequestFingerprint,
		Tasks:              request.Tasks,
	}
}

func engineCampaignRequest(
	operation annotationdomain.EngineOperation,
	manifest engineCampaignManifest,
) EngineCampaignRequest {
	return EngineCampaignRequest{
		WorkspaceID:        operation.WorkspaceID,
		CampaignID:         operation.CampaignID,
		RequestID:          operation.RequestID,
		RequestFingerprint: operation.RequestFingerprint,
		Title:              manifest.Title,
		SchemaContent:      manifest.SchemaContent,
		SchemaSHA256:       manifest.SchemaSHA256,
	}
}

func engineSubmitRequest(
	operation annotationdomain.EngineOperation,
	manifest engineTasksManifest,
) EngineSubmitRequest {
	return EngineSubmitRequest{
		WorkspaceID: operation.WorkspaceID,
		CampaignID:  operation.CampaignID,
		Binding: EngineCampaignBinding{
			Provider:          manifest.Binding.Provider,
			ProviderInstance:  manifest.Binding.ProviderInstance,
			ExternalProjectID: manifest.Binding.ExternalProjectID,
			RequestID:         manifest.Binding.RequestID,
			ConfigSHA256:      manifest.Binding.ConfigSHA256,
		},
		RequestID:          operation.RequestID,
		RequestFingerprint: operation.RequestFingerprint,
		Tasks:              manifest.Tasks,
	}
}

var _ = context.Background
