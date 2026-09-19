package domain

import "github.com/google/uuid"

// DependencyBinding is an immutable dependency snapshot prepared for one
// Execution. DatasetVersionID identifies a platform input; Content is used for
// file-backed policies so a later path mutation cannot change the calculation.
type DependencyBinding struct {
	ID               uuid.UUID
	ExecutionID      uuid.UUID
	WorkspaceID      uuid.UUID
	Name             string
	DatasetVersionID *uuid.UUID
	Reference        string
	Version          string
	ContentSHA256    string
	Content          []byte
}

// MappingUsage records the exact immutable mapping decision consumed for one
// source scope. It is intentionally separate from MappingDecision.SourceJobID:
// the latter names the producer of a decision, while this object names its
// consumer.
type MappingUsage struct {
	ID                         uuid.UUID
	ExecutionID                uuid.UUID
	WorkspaceID                uuid.UUID
	InputName                  string
	InputDatasetVersionID      uuid.UUID
	ResolutionDatasetVersionID uuid.UUID
	SourceType                 string
	SourceRef                  string
	SourceKey                  string
	DecisionID                 uuid.UUID
	EntityID                   uuid.UUID
}

type DependencyPreparation struct {
	ExecutionID        uuid.UUID
	WorkspaceID        uuid.UUID
	BindingFingerprint string
	MappingUsageCount  int
	Status             string
	Dependencies       []DependencyBinding
	MappingUsages      []MappingUsage
}
