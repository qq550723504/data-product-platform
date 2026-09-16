package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/infrastructure"
)

type Service struct {
	tx   *transaction.Manager
	repo *infrastructure.PostgresRepository
}

func NewService(tx *transaction.Manager, repo *infrastructure.PostgresRepository) *Service {
	return &Service{tx: tx, repo: repo}
}

type CreateProductCommand struct {
	WorkspaceID uuid.UUID
	ProjectID   *uuid.UUID
	UseCaseID   *uuid.UUID
	Code        string
	Name        string
	Description string
	DomainCode  string
	OwnerID     *uuid.UUID
	Metadata    map[string]any
	ActorID     *uuid.UUID
	TraceID     string
}

type CreateVersionCommand struct {
	ProductID         uuid.UUID
	MajorVersion      int
	MinorVersion      int
	PatchVersion      int
	WorkflowVersionID *uuid.UUID
	ContractVersionID *uuid.UUID
	EntityPolicyRef   string
	IndicatorSetRef   string
	Definition        map[string]any
	Assets            []domain.AssetSpec
	ActorID           *uuid.UUID
	TraceID           string
}

type CreateReleaseCommand struct {
	ProductID        uuid.UUID
	ProductVersionID uuid.UUID
	ReleaseNo        string
	Datasets         []domain.ReleaseDataset
	ReleaseNotes     string
	Metadata         map[string]any
	ActorID          *uuid.UUID
	TraceID          string
}

func (s *Service) CreateProduct(ctx context.Context, cmd CreateProductCommand) (domain.DataProduct, error) {
	product, err := domain.NewDataProduct(
		cmd.WorkspaceID,
		cmd.Code,
		cmd.Name,
		cmd.Description,
		cmd.DomainCode,
		cmd.ProjectID,
		cmd.UseCaseID,
		cmd.OwnerID,
		cmd.Metadata,
		cmd.ActorID,
	)
	if err != nil {
		return domain.DataProduct{}, err
	}
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.repo.InsertProduct(ctx, tx, product); err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, "DATA_PRODUCT", product.ID, "DataProductCreated", map[string]any{
			"productId": product.ID,
			"code":      product.Code,
			"status":    product.LifecycleStatus,
		}); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &product.WorkspaceID,
			ActorType:   actorType(cmd.ActorID),
			ActorID:     cmd.ActorID,
			Action:      "DATA_PRODUCT_CREATED",
			ObjectType:  "DATA_PRODUCT",
			ObjectID:    product.ID,
			AfterState: map[string]any{
				"code":   product.Code,
				"name":   product.Name,
				"status": product.LifecycleStatus,
			},
			TraceID: cmd.TraceID,
		})
	})
	return product, err
}

func (s *Service) CreateVersion(ctx context.Context, cmd CreateVersionCommand) (domain.ProductVersion, error) {
	product, err := s.repo.GetProduct(ctx, cmd.ProductID)
	if err != nil {
		return domain.ProductVersion{}, err
	}
	version, err := domain.NewProductVersion(
		cmd.ProductID,
		cmd.MajorVersion,
		cmd.MinorVersion,
		cmd.PatchVersion,
		cmd.WorkflowVersionID,
		cmd.ContractVersionID,
		cmd.EntityPolicyRef,
		cmd.IndicatorSetRef,
		cmd.Definition,
		cmd.Assets,
		cmd.ActorID,
	)
	if err != nil {
		return domain.ProductVersion{}, err
	}
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.repo.InsertVersion(ctx, tx, version); err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, "PRODUCT_VERSION", version.ID, "ProductVersionCreated", map[string]any{
			"productId":        version.ProductID,
			"productVersionId": version.ID,
			"version":          version.Semver(),
		}); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &product.WorkspaceID,
			ActorType:   actorType(cmd.ActorID),
			ActorID:     cmd.ActorID,
			Action:      "PRODUCT_VERSION_CREATED",
			ObjectType:  "PRODUCT_VERSION",
			ObjectID:    version.ID,
			AfterState: map[string]any{
				"productId":  product.ID,
				"version":    version.Semver(),
				"assetCount": len(version.Assets),
			},
			TraceID: cmd.TraceID,
		})
	})
	return version, err
}

func (s *Service) CreateRelease(ctx context.Context, cmd CreateReleaseCommand) (domain.ProductRelease, error) {
	product, err := s.repo.GetProduct(ctx, cmd.ProductID)
	if err != nil {
		return domain.ProductRelease{}, err
	}
	version, err := s.repo.GetVersion(ctx, cmd.ProductVersionID)
	if err != nil {
		return domain.ProductRelease{}, err
	}
	release, err := domain.NewProductRelease(
		cmd.ProductID,
		cmd.ProductVersionID,
		cmd.ReleaseNo,
		cmd.Datasets,
		cmd.ReleaseNotes,
		cmd.Metadata,
		cmd.ActorID,
	)
	if err != nil {
		return domain.ProductRelease{}, err
	}
	release.ContractVersionID = version.ContractVersionID
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.repo.ValidateReleaseReferences(ctx, tx, release); err != nil {
			return err
		}
		if err := s.repo.InsertRelease(ctx, tx, release); err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, "PRODUCT_RELEASE", release.ID, "ProductReleaseDraftCreated", map[string]any{
			"productId":        release.ProductID,
			"productVersionId": release.ProductVersionID,
			"releaseId":        release.ID,
			"releaseNo":        release.ReleaseNo,
			"status":           release.Status,
		}); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &product.WorkspaceID,
			ActorType:   actorType(cmd.ActorID),
			ActorID:     cmd.ActorID,
			Action:      "PRODUCT_RELEASE_DRAFT_CREATED",
			ObjectType:  "PRODUCT_RELEASE",
			ObjectID:    release.ID,
			AfterState: map[string]any{
				"productId":        release.ProductID,
				"productVersionId": release.ProductVersionID,
				"releaseNo":        release.ReleaseNo,
				"status":           release.Status,
				"datasetCount":     len(release.Datasets),
			},
			TraceID: cmd.TraceID,
		})
	})
	return release, err
}

type CheckStatus string

const (
	CheckPass    CheckStatus = "PASS"
	CheckFail    CheckStatus = "FAIL"
	CheckPending CheckStatus = "PENDING"
)

type ReadinessResult struct {
	ReleaseID uuid.UUID              `json:"releaseId"`
	Overall   string                 `json:"overall"`
	Checks    map[string]CheckStatus `json:"checks"`
	Blockers  []string               `json:"blockers"`
}

func (s *Service) Readiness(ctx context.Context, releaseID uuid.UUID) (ReadinessResult, error) {
	release, err := s.repo.GetRelease(ctx, releaseID)
	if err != nil {
		return ReadinessResult{}, err
	}
	checks := map[string]CheckStatus{
		"production": CheckPass,
		"dataset":    CheckPass,
		"rights":     CheckPending,
		"quality":    CheckPending,
		"compliance": CheckPending,
		"contract":   CheckPending,
		"evidence":   CheckPending,
		"delivery":   CheckPending,
	}
	if release.RightsSnapshotID != nil {
		checks["rights"] = CheckPass
	}
	if release.QualityResultID != nil {
		checks["quality"] = CheckPass
	}
	if release.ComplianceResultID != nil {
		checks["compliance"] = CheckPass
	}
	if release.ContractVersionID != nil {
		checks["contract"] = CheckPass
	}
	if release.EvidenceSnapshotID != nil {
		checks["evidence"] = CheckPass
	}
	blockers := make([]string, 0)
	for name, status := range checks {
		if status != CheckPass {
			blockers = append(blockers, name)
		}
	}
	return ReadinessResult{
		ReleaseID: release.ID,
		Overall:   "NOT_READY",
		Checks:    checks,
		Blockers:  blockers,
	}, nil
}

func appendEvent(ctx context.Context, tx pgx.Tx, aggregateType string, aggregateID uuid.UUID, eventType string, payload map[string]any) error {
	event, err := outbox.NewEvent(aggregateType, aggregateID, eventType, payload)
	if err != nil {
		return fmt.Errorf("create %s event: %w", eventType, err)
	}
	return outbox.Append(ctx, tx, event)
}

func actorType(actorID *uuid.UUID) string {
	if actorID == nil {
		return "SYSTEM"
	}
	return "USER"
}
