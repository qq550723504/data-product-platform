package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ResourceType string

type LifecycleStatus string

const (
	ResourceTypeTableLike       ResourceType = "TABLE_LIKE"
	ResourceTypeStream          ResourceType = "STREAM"
	ResourceTypeFileCollection  ResourceType = "FILE_COLLECTION"
	ResourceTypeAPI             ResourceType = "API"
	ResourceTypeDocument        ResourceType = "DOCUMENT"
	ResourceTypeImageCollection ResourceType = "IMAGE_COLLECTION"
	ResourceTypeOther           ResourceType = "OTHER"

	LifecycleDiscovered     LifecycleStatus = "DISCOVERED"
	LifecycleCataloged      LifecycleStatus = "CATALOGED"
	LifecycleClassified     LifecycleStatus = "CLASSIFIED"
	LifecycleRightsReviewed LifecycleStatus = "RIGHTS_REVIEWED"
	LifecycleReady          LifecycleStatus = "READY"
	LifecycleBlocked        LifecycleStatus = "BLOCKED"
	LifecycleArchived       LifecycleStatus = "ARCHIVED"
)

var (
	ErrInvalidWorkspace = errors.New("workspace id is required")
	ErrInvalidCode      = errors.New("resource code is required")
	ErrInvalidName      = errors.New("resource name is required")
	ErrInvalidType      = errors.New("invalid resource type")
)

type DataResource struct {
	ID               uuid.UUID
	WorkspaceID      uuid.UUID
	ProjectID        *uuid.UUID
	Code             string
	Name             string
	Description      string
	DomainCode       string
	ResourceType     ResourceType
	OwnerID          *uuid.UUID
	SensitivityLevel string
	RightsStatus     string
	QualityStatus    string
	LifecycleStatus  LifecycleStatus
	BusinessMetadata map[string]any
	Extension        map[string]any
	Revision         int64
	CreatedAt        time.Time
	CreatedBy        *uuid.UUID
}

func NewDataResource(id, workspaceID uuid.UUID, code, name string, resourceType ResourceType, createdBy *uuid.UUID) (DataResource, error) {
	if id == uuid.Nil {
		id = uuid.New()
	}
	if workspaceID == uuid.Nil {
		return DataResource{}, ErrInvalidWorkspace
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return DataResource{}, ErrInvalidCode
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return DataResource{}, ErrInvalidName
	}
	if !resourceType.Valid() {
		return DataResource{}, ErrInvalidType
	}

	return DataResource{
		ID:               id,
		WorkspaceID:      workspaceID,
		Code:             code,
		Name:             name,
		ResourceType:     resourceType,
		RightsStatus:     "UNKNOWN",
		QualityStatus:    "UNKNOWN",
		LifecycleStatus:  LifecycleDiscovered,
		BusinessMetadata: map[string]any{},
		Extension:        map[string]any{},
		Revision:         1,
		CreatedAt:        time.Now().UTC(),
		CreatedBy:        createdBy,
	}, nil
}

func (t ResourceType) Valid() bool {
	switch t {
	case ResourceTypeTableLike,
		ResourceTypeStream,
		ResourceTypeFileCollection,
		ResourceTypeAPI,
		ResourceTypeDocument,
		ResourceTypeImageCollection,
		ResourceTypeOther:
		return true
	default:
		return false
	}
}
