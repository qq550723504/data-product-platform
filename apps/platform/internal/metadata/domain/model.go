package domain

import (
	"time"

	"github.com/google/uuid"
)

type Provider string

type ProjectionStatus string

const (
	ProviderOpenMetadata Provider = "OPENMETADATA"

	ProjectionPending   ProjectionStatus = "PENDING"
	ProjectionSucceeded ProjectionStatus = "SUCCEEDED"
	ProjectionFailed    ProjectionStatus = "FAILED"
)

type ResourceBinding struct {
	ID              uuid.UUID
	ResourceID      uuid.UUID
	Provider        Provider
	EntityType      string
	ExternalID      string
	ExternalFQN     string
	BindingMetadata map[string]any
	IsPrimary       bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type GovernanceProjection struct {
	ID            uuid.UUID
	WorkspaceID   uuid.UUID
	Provider      Provider
	ObjectType    string
	ObjectID      uuid.UUID
	SourceEventID *uuid.UUID
	ExternalID    string
	ExternalFQN   string
	Status        ProjectionStatus
	Attempts      int
	LastError     string
	Metadata      map[string]any
	ProjectedAt   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
