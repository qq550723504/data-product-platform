package metadata

import "context"

type Asset struct {
	ID                 string
	EntityType         string
	Name               string
	FullyQualifiedName string
	DisplayName        string
	Description        string
	Metadata           map[string]any
}

type GovernanceProduct struct {
	Name        string
	DisplayName string
	Description string
	Domain      string
	Metadata    map[string]any
}

type ExternalEntity struct {
	ID                 string
	EntityType         string
	FullyQualifiedName string
	Metadata           map[string]any
}

// Engine is the provider-neutral outbound port for metadata/governance systems.
// Core domain packages must depend on this abstraction, never on OpenMetadata-specific APIs.
type Engine interface {
	GetAsset(ctx context.Context, entityType, fullyQualifiedName string) (Asset, error)
	UpsertDataProduct(ctx context.Context, product GovernanceProduct) (ExternalEntity, error)
}
