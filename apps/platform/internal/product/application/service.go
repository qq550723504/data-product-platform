package application

import (
	"context"
	"fmt"
	"sort"
	"time"

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

type ValidateReleaseCommand struct {
	ReleaseID          uuid.UUID
	ContractVersionID  uuid.UUID
	RightsSnapshotID   uuid.UUID
	QualityResultID    uuid.UUID
	ComplianceResultID uuid.UUID
	ActorID            *uuid.UUID
	TraceID            string
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
	Details   map[string]any         `json:"details,omitempty"`
}

func (s *Service) ValidateRelease(ctx context.Context, cmd ValidateReleaseCommand) (ReadinessResult, error) {
	release, err := s.repo.GetRelease(ctx, cmd.ReleaseID)
	if err != nil {
		return ReadinessResult{}, err
	}
	product, err := s.repo.GetProduct(ctx, release.ProductID)
	if err != nil {
		return ReadinessResult{}, err
	}
	before := release.Status
	if err := release.BeginValidation(cmd.ContractVersionID, cmd.RightsSnapshotID, cmd.QualityResultID, cmd.ComplianceResultID); err != nil {
		return ReadinessResult{}, err
	}

	if err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.repo.SaveReleaseValidation(ctx, tx, release); err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, "PRODUCT_RELEASE", release.ID, "ProductReleaseValidationStarted", map[string]any{
			"releaseId":          release.ID,
			"contractVersionId":  release.ContractVersionID,
			"rightsSnapshotId":   release.RightsSnapshotID,
			"qualityResultId":    release.QualityResultID,
			"complianceResultId": release.ComplianceResultID,
		}); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &product.WorkspaceID,
			ActorType:   actorType(cmd.ActorID),
			ActorID:     cmd.ActorID,
			Action:      "PRODUCT_RELEASE_VALIDATION_STARTED",
			ObjectType:  "PRODUCT_RELEASE",
			ObjectID:    release.ID,
			BeforeState: map[string]any{"status": before},
			AfterState: map[string]any{
				"status":             release.Status,
				"contractVersionId":  release.ContractVersionID,
				"rightsSnapshotId":   release.RightsSnapshotID,
				"qualityResultId":    release.QualityResultID,
				"complianceResultId": release.ComplianceResultID,
			},
			TraceID: cmd.TraceID,
		})
	}); err != nil {
		return ReadinessResult{}, err
	}

	readiness, err := s.Readiness(ctx, release.ID)
	if err != nil {
		return ReadinessResult{}, err
	}
	if readiness.Overall != "READY" {
		return readiness, nil
	}
	if err := release.MarkReady(); err != nil {
		return ReadinessResult{}, err
	}
	if err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.repo.SaveReleaseStatus(ctx, tx, release.ID, domain.ReleaseValidating, domain.ReleaseReady); err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, "PRODUCT_RELEASE", release.ID, "ProductReleaseReady", map[string]any{
			"releaseId": release.ID,
			"status":    domain.ReleaseReady,
		}); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &product.WorkspaceID,
			ActorType:   actorType(cmd.ActorID),
			ActorID:     cmd.ActorID,
			Action:      "PRODUCT_RELEASE_READY",
			ObjectType:  "PRODUCT_RELEASE",
			ObjectID:    release.ID,
			BeforeState: map[string]any{"status": domain.ReleaseValidating},
			AfterState:  map[string]any{"status": domain.ReleaseReady},
			TraceID:     cmd.TraceID,
		})
	}); err != nil {
		return ReadinessResult{}, err
	}
	return readiness, nil
}

func (s *Service) Readiness(ctx context.Context, releaseID uuid.UUID) (ReadinessResult, error) {
	release, err := s.repo.GetRelease(ctx, releaseID)
	if err != nil {
		return ReadinessResult{}, err
	}
	if release.Status == domain.ReleaseDraft {
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
		return ReadinessResult{
			ReleaseID: release.ID,
			Overall:   "NOT_READY",
			Checks:    checks,
			Blockers:  []string{"rights", "quality", "compliance", "contract", "evidence", "delivery"},
		}, nil
	}
	product, err := s.repo.GetProduct(ctx, release.ProductID)
	if err != nil {
		return ReadinessResult{}, err
	}
	version, err := s.repo.GetVersion(ctx, release.ProductVersionID)
	if err != nil {
		return ReadinessResult{}, err
	}
	facts, err := s.repo.ReadinessFacts(ctx, release, product, version, time.Now().UTC())
	if err != nil {
		return ReadinessResult{}, err
	}
	return readinessResultFromFacts(release.ID, facts), nil
}

func readinessResultFromFacts(releaseID uuid.UUID, facts infrastructure.ReadinessFacts) ReadinessResult {
	checks := map[string]CheckStatus{
		"production": CheckFail,
		"dataset":    CheckFail,
		"rights":     CheckFail,
		"quality":    CheckFail,
		"compliance": CheckFail,
		"contract":   CheckFail,
		"evidence":   CheckFail,
		"delivery":   CheckFail,
	}
	blockers := make([]string, 0)
	details := map[string]any{}

	if facts.TargetDatasetVersionID != nil {
		if facts.ProductionDependencyBindingRequired && !facts.ProductionDependencyBindingComplete {
			blockers = append(blockers, "PRODUCTION_DEPENDENCY_BINDING_INCOMPLETE")
		} else {
			checks["production"] = CheckPass
		}
	} else {
		blockers = append(blockers, "PRODUCTION_DATASET_MISSING")
	}
	if facts.AllDatasetsUsable && facts.TargetDatasetVersionID != nil {
		checks["dataset"] = CheckPass
	} else {
		blockers = append(blockers, "DATASET_NOT_USABLE")
	}

	rightsStatus, rightsBlockers, rightsDetails := evaluateRightsReadiness(facts)
	checks["rights"] = rightsStatus
	blockers = append(blockers, rightsBlockers...)
	details["rights"] = rightsDetails

	if !facts.ContractExists {
		blockers = append(blockers, "CONTRACT_VERSION_MISSING")
	} else if !facts.ContractMatchesProduct {
		blockers = append(blockers, "CONTRACT_VERSION_MISMATCH")
	} else if !facts.ContractPublished {
		blockers = append(blockers, "CONTRACT_NOT_PUBLISHED")
	} else {
		checks["contract"] = CheckPass
	}
	if !facts.QualityResultExists {
		blockers = append(blockers, "QUALITY_RESULT_MISSING")
	} else if !facts.QualityDatasetMatches {
		blockers = append(blockers, "QUALITY_DATASET_MISMATCH")
	} else if facts.QualityDecision != "PASS" && facts.QualityDecision != "PASS_WITH_WARNING" {
		blockers = append(blockers, "QUALITY_GATE_BLOCKING")
	} else {
		checks["quality"] = CheckPass
	}
	if !facts.ComplianceResultExists {
		blockers = append(blockers, "COMPLIANCE_RESULT_MISSING")
	} else if !facts.ComplianceDatasetMatches {
		blockers = append(blockers, "COMPLIANCE_DATASET_MISMATCH")
	} else if facts.ComplianceDecision != "PASS" {
		blockers = append(blockers, "COMPLIANCE_GATE_BLOCKING")
	} else {
		checks["compliance"] = CheckPass
	}
	if facts.EvidenceCount >= 2 {
		checks["evidence"] = CheckPass
	} else {
		blockers = append(blockers, "EVIDENCE_INCOMPLETE")
	}
	if facts.DeliveryAvailable {
		checks["delivery"] = CheckPass
	} else {
		blockers = append(blockers, "DELIVERY_ASSET_MISSING")
	}
	details["productionDependencyBinding"] = map[string]any{
		"required": facts.ProductionDependencyBindingRequired,
		"complete": facts.ProductionDependencyBindingComplete,
	}

	sort.Strings(blockers)
	overall := "NOT_READY"
	allPass := true
	for _, status := range checks {
		if status != CheckPass {
			allPass = false
			break
		}
	}
	if allPass {
		overall = "READY"
	}
	return ReadinessResult{ReleaseID: releaseID, Overall: overall, Checks: checks, Blockers: blockers, Details: details}
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
