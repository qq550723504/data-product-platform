package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ProductStatus string

type ProductHealth string

type ReleaseStatus string

type AssetType string

type DatasetRole string

const (
	ProductDraft       ProductStatus = "DRAFT"
	ProductDesigning   ProductStatus = "DESIGNING"
	ProductDeveloping  ProductStatus = "DEVELOPING"
	ProductTesting     ProductStatus = "TESTING"
	ProductReady       ProductStatus = "READY"
	ProductPublished   ProductStatus = "PUBLISHED"
	ProductActive      ProductStatus = "ACTIVE"
	ProductSuspended   ProductStatus = "SUSPENDED"
	ProductDeprecated  ProductStatus = "DEPRECATED"
	ProductRetired     ProductStatus = "RETIRED"

	HealthUnknown   ProductHealth = "UNKNOWN"
	HealthHealthy   ProductHealth = "HEALTHY"
	HealthDegraded  ProductHealth = "DEGRADED"
	HealthUnhealthy ProductHealth = "UNHEALTHY"

	ReleaseDraft      ReleaseStatus = "DRAFT"
	ReleaseValidating ReleaseStatus = "VALIDATING"
	ReleaseReady      ReleaseStatus = "READY"
	ReleasePublished  ReleaseStatus = "PUBLISHED"
	ReleaseSuspended  ReleaseStatus = "SUSPENDED"
	ReleaseWithdrawn  ReleaseStatus = "WITHDRAWN"
	ReleaseFailed     ReleaseStatus = "FAILED"

	AssetDataset          AssetType = "DATASET"
	AssetAPI              AssetType = "API"
	AssetReport           AssetType = "REPORT"
	AssetDashboard        AssetType = "DASHBOARD"
	AssetIndicatorService AssetType = "INDICATOR_SERVICE"
	AssetModelResult      AssetType = "MODEL_RESULT"
	AssetSandbox          AssetType = "SANDBOX"

	DatasetPrimary    DatasetRole = "PRIMARY"
	DatasetInput      DatasetRole = "INPUT"
	DatasetOutput     DatasetRole = "OUTPUT"
	DatasetSupporting DatasetRole = "SUPPORTING"
)

var (
	ErrInvalidProduct        = errors.New("data product is invalid")
	ErrInvalidProductVersion = errors.New("product version is invalid")
	ErrInvalidProductAsset   = errors.New("product asset is invalid")
	ErrInvalidRelease        = errors.New("product release is invalid")
)

type DataProduct struct {
	ID               uuid.UUID
	WorkspaceID      uuid.UUID
	ProjectID        *uuid.UUID
	UseCaseID        *uuid.UUID
	Code             string
	Name             string
	Description      string
	DomainCode       string
	OwnerID          *uuid.UUID
	LifecycleStatus  ProductStatus
	HealthStatus     ProductHealth
	CurrentVersionID *uuid.UUID
	LatestReleaseID  *uuid.UUID
	Metadata         map[string]any
	CreatedAt        time.Time
	CreatedBy        *uuid.UUID
}

type ProductVersion struct {
	ID                  uuid.UUID
	ProductID           uuid.UUID
	MajorVersion        int
	MinorVersion        int
	PatchVersion        int
	WorkflowVersionID   *uuid.UUID
	ContractVersionID   *uuid.UUID
	EntityPolicyRef     string
	IndicatorSetRef     string
	DefinitionSnapshot  map[string]any
	Assets              []ProductAsset
	CreatedAt           time.Time
	CreatedBy           *uuid.UUID
}

type ProductAsset struct {
	ID               uuid.UUID
	ProductVersionID uuid.UUID
	AssetType         AssetType
	Name              string
	DatasetID         *uuid.UUID
	ExternalRef       string
	DeliveryConfig    map[string]any
	SchemaSnapshot    map[string]any
	CreatedAt         time.Time
}

type ReleaseDataset struct {
	DatasetVersionID uuid.UUID
	Role             DatasetRole
}

type ProductRelease struct {
	ID                   uuid.UUID
	ProductID            uuid.UUID
	ProductVersionID     uuid.UUID
	ReleaseNo             string
	Status                ReleaseStatus
	ContractVersionID     *uuid.UUID
	RightsSnapshotID      *uuid.UUID
	QualityResultID       *uuid.UUID
	ComplianceResultID    *uuid.UUID
	EvidenceSnapshotID    *uuid.UUID
	Datasets              []ReleaseDataset
	ReleaseNotes          string
	Metadata              map[string]any
	CreatedAt             time.Time
	CreatedBy             *uuid.UUID
	ReleasedAt            *time.Time
	ReleasedBy            *uuid.UUID
}

type AssetSpec struct {
	AssetType      AssetType
	Name           string
	DatasetID      *uuid.UUID
	ExternalRef    string
	DeliveryConfig map[string]any
	SchemaSnapshot map[string]any
}

func NewDataProduct(workspaceID uuid.UUID, code, name, description, domainCode string, projectID, useCaseID, ownerID *uuid.UUID, metadata map[string]any, actorID *uuid.UUID) (DataProduct, error) {
	code = strings.TrimSpace(code)
	name = strings.TrimSpace(name)
	if workspaceID == uuid.Nil || code == "" || name == "" {
		return DataProduct{}, ErrInvalidProduct
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	return DataProduct{
		ID:              uuid.New(),
		WorkspaceID:     workspaceID,
		ProjectID:       projectID,
		UseCaseID:       useCaseID,
		Code:            code,
		Name:            name,
		Description:     strings.TrimSpace(description),
		DomainCode:      strings.TrimSpace(domainCode),
		OwnerID:         ownerID,
		LifecycleStatus: ProductDraft,
		HealthStatus:    HealthUnknown,
		Metadata:        metadata,
		CreatedAt:       time.Now().UTC(),
		CreatedBy:       actorID,
	}, nil
}

func NewProductVersion(productID uuid.UUID, major, minor, patch int, workflowVersionID, contractVersionID *uuid.UUID, entityPolicyRef, indicatorSetRef string, definition map[string]any, assets []AssetSpec, actorID *uuid.UUID) (ProductVersion, error) {
	if productID == uuid.Nil || major < 0 || minor < 0 || patch < 0 {
		return ProductVersion{}, ErrInvalidProductVersion
	}
	if definition == nil {
		definition = map[string]any{}
	}
	version := ProductVersion{
		ID:                 uuid.New(),
		ProductID:          productID,
		MajorVersion:       major,
		MinorVersion:       minor,
		PatchVersion:       patch,
		WorkflowVersionID:  workflowVersionID,
		ContractVersionID:  contractVersionID,
		EntityPolicyRef:    strings.TrimSpace(entityPolicyRef),
		IndicatorSetRef:    strings.TrimSpace(indicatorSetRef),
		DefinitionSnapshot: definition,
		CreatedAt:          time.Now().UTC(),
		CreatedBy:          actorID,
	}
	for _, spec := range assets {
		asset, err := newAsset(version.ID, spec)
		if err != nil {
			return ProductVersion{}, err
		}
		version.Assets = append(version.Assets, asset)
	}
	return version, nil
}

func newAsset(productVersionID uuid.UUID, spec AssetSpec) (ProductAsset, error) {
	name := strings.TrimSpace(spec.Name)
	if productVersionID == uuid.Nil || name == "" || !validAssetType(spec.AssetType) {
		return ProductAsset{}, ErrInvalidProductAsset
	}
	if spec.AssetType == AssetDataset && spec.DatasetID == nil {
		return ProductAsset{}, fmt.Errorf("%w: DATASET asset requires dataset id", ErrInvalidProductAsset)
	}
	if spec.DeliveryConfig == nil {
		spec.DeliveryConfig = map[string]any{}
	}
	if spec.SchemaSnapshot == nil {
		spec.SchemaSnapshot = map[string]any{}
	}
	return ProductAsset{
		ID:               uuid.New(),
		ProductVersionID: productVersionID,
		AssetType:        spec.AssetType,
		Name:             name,
		DatasetID:        spec.DatasetID,
		ExternalRef:      strings.TrimSpace(spec.ExternalRef),
		DeliveryConfig:   spec.DeliveryConfig,
		SchemaSnapshot:   spec.SchemaSnapshot,
		CreatedAt:        time.Now().UTC(),
	}, nil
}

func NewProductRelease(productID, productVersionID uuid.UUID, releaseNo string, datasets []ReleaseDataset, releaseNotes string, metadata map[string]any, actorID *uuid.UUID) (ProductRelease, error) {
	releaseNo = strings.TrimSpace(releaseNo)
	if productID == uuid.Nil || productVersionID == uuid.Nil || releaseNo == "" || len(datasets) == 0 {
		return ProductRelease{}, ErrInvalidRelease
	}
	seen := map[string]struct{}{}
	for _, binding := range datasets {
		if binding.DatasetVersionID == uuid.Nil || !validDatasetRole(binding.Role) {
			return ProductRelease{}, ErrInvalidRelease
		}
		key := binding.DatasetVersionID.String() + ":" + string(binding.Role)
		if _, exists := seen[key]; exists {
			return ProductRelease{}, fmt.Errorf("%w: duplicate dataset binding", ErrInvalidRelease)
		}
		seen[key] = struct{}{}
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	return ProductRelease{
		ID:               uuid.New(),
		ProductID:        productID,
		ProductVersionID: productVersionID,
		ReleaseNo:        releaseNo,
		Status:           ReleaseDraft,
		Datasets:         append([]ReleaseDataset(nil), datasets...),
		ReleaseNotes:     strings.TrimSpace(releaseNotes),
		Metadata:         metadata,
		CreatedAt:        time.Now().UTC(),
		CreatedBy:        actorID,
	}, nil
}

func (v ProductVersion) Semver() string {
	return fmt.Sprintf("%d.%d.%d", v.MajorVersion, v.MinorVersion, v.PatchVersion)
}

func validAssetType(assetType AssetType) bool {
	switch assetType {
	case AssetDataset, AssetAPI, AssetReport, AssetDashboard, AssetIndicatorService, AssetModelResult, AssetSandbox:
		return true
	default:
		return false
	}
}

func validDatasetRole(role DatasetRole) bool {
	switch role {
	case DatasetPrimary, DatasetInput, DatasetOutput, DatasetSupporting:
		return true
	default:
		return false
	}
}
