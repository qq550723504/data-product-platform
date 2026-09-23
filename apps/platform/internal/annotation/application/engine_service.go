package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
	annotationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

type EngineService struct {
	tx     *transaction.Manager
	repo   *annotationinfra.Repository
	engine AnnotationEnginePort
}

func NewEngineService(tx *transaction.Manager, repo *annotationinfra.Repository, engine AnnotationEnginePort) *EngineService {
	return &EngineService{tx: tx, repo: repo, engine: engine}
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
	Kind          string `json:"kind"`
	WorkspaceID   string `json:"workspaceId"`
	CampaignID    string `json:"campaignId"`
	Title         string `json:"title"`
	SchemaContent string `json:"schemaContent"`
	SchemaSHA256  string `json:"schemaSha256"`
}

type engineTasksManifest struct {
	Kind        string        `json:"kind"`
	WorkspaceID string        `json:"workspaceId"`
	CampaignID  string        `json:"campaignId"`
	Binding     engineBinding `json:"binding"`
	Tasks       []EngineTask  `json:"tasks"`
}

type engineBinding struct {
	Provider          string `json:"provider"`
	ProviderInstance  string `json:"providerInstance"`
	ExternalProjectID string `json:"externalProjectId"`
	RequestID         string `json:"requestId"`
	ConfigSHA256      string `json:"configSha256"`
}

func (s *EngineService) PrepareCampaign(ctx context.Context, cmd PrepareEngineCampaignCommand) (annotationdomain.EngineOperation, error) {
	if s == nil || s.tx == nil || s.repo == nil || s.engine == nil {
		return annotationdomain.EngineOperation{}, errors.New("annotation engine service is not configured")
	}
	cmd.RequestID = strings.TrimSpace(cmd.RequestID)
	cmd.Title = strings.TrimSpace(cmd.Title)
	if cmd.WorkspaceID == uuid.Nil || cmd.CampaignID == uuid.Nil || cmd.RequestID == "" || cmd.Title == "" {
		return annotationdomain.EngineOperation{}, annotationdomain.ErrInvalidEngineOperation
	}

 campaign, err := s.repo.GetCampaign(ctx, cmd.CampaignID)
	if err != nil {
		return annotationdomain.EngineOperation{}, err
	}
	if campaign.WorkspaceID != cmd.WorkspaceID || campaign.Status != annotationdomain.CampaignActive {
		return annotationdomain.EngineOperation{}, annotationdomain.ErrInvalidCampaign
	}
	manifest := engineCampaignManifest{
		Kind:          annotationdomain.EngineOperationEnsureCampaign,
		WorkspaceID:   cmd.WorkspaceID.String(),
		CampaignID:    cmd.CampaignID.String(),
		Title:         cmd.Title,
		SchemaContent: campaign.Schema.ContentSnapshot,
		SchemaSHA256:  campaign.Schema.ContentSHA256,
	}
	return s.prepareOperation(ctx, campaign, cmd.RequestID, manifest, cmd.ActorID, cmd.TraceID)
}

func (s *EngineService) PrepareTasks(ctx context.Context, cmd PrepareEngineTasksCommand) (annotationdomain.EngineOperation, error) {
	if s == nil || s.tx == nil || s.repo == nil || s.engine == nil {