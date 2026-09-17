package resolution

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// EngineDescriptor identifies a probabilistic candidate engine without leaking
// provider-specific settings into the Core Entity domain.
type EngineDescriptor struct {
	Name         string
	Version      string
	ModelVersion string
}

// MatchRecord is the provider-neutral representation supplied to candidate
// engines. Fields are normalized Core values, not provider-specific payloads.
type MatchRecord struct {
	ID     string
	Name   string
	Fields map[string]string
}

// ReferenceRecord represents one canonical Core Entity that a candidate engine
// may propose for the source record.
type ReferenceRecord struct {
	EntityID uuid.UUID
	Name     string
	Fields   map[string]string
}

// CandidateRequest freezes the matching context that may influence candidate
// generation. Engines may score references but never mutate Core Entity state.
type CandidateRequest struct {
	EntityType    string
	Source        MatchRecord
	References    []ReferenceRecord
	PolicyRef     string
	PolicyVersion string
}

// Candidate is a provider-neutral proposed link. Score is normalized to [0,1].
type Candidate struct {
	EntityID uuid.UUID
	Score    float64
	Method   string
	Engine   EngineDescriptor
	Metadata map[string]any
}

func (c Candidate) Validate() error {
	if c.EntityID == uuid.Nil {
		return fmt.Errorf("candidate entity id is required")
	}
	if c.Score < 0 || c.Score > 1 {
		return fmt.Errorf("candidate score %.6f must be between 0 and 1", c.Score)
	}
	if strings.TrimSpace(c.Engine.Name) == "" {
		return fmt.Errorf("candidate engine name is required")
	}
	return nil
}

// CandidateGenerator is the SPI implemented by probabilistic engines such as
// Splink. It generates ranked proposals only; canonical Entity/EntityMapping
// writes remain Core responsibilities.
type CandidateGenerator interface {
	Descriptor() EngineDescriptor
	Generate(ctx context.Context, request CandidateRequest) ([]Candidate, error)
}

// Registry keeps optional candidate engines replaceable. An absent engine is a
// normal condition and lets callers fall back to deterministic rule matching.
type Registry struct {
	engines map[string]CandidateGenerator
}

func NewRegistry(engines ...CandidateGenerator) (*Registry, error) {
	registry := &Registry{engines: map[string]CandidateGenerator{}}
	for _, engine := range engines {
		if engine == nil {
			continue
		}
		descriptor := engine.Descriptor()
		name := normalizeName(descriptor.Name)
		if name == "" {
			return nil, fmt.Errorf("candidate engine name is required")
		}
		if _, exists := registry.engines[name]; exists {
			return nil, fmt.Errorf("candidate engine %s is registered more than once", name)
		}
		registry.engines[name] = engine
	}
	return registry, nil
}

func (r *Registry) Get(name string) (CandidateGenerator, bool) {
	if r == nil {
		return nil, false
	}
	engine, ok := r.engines[normalizeName(name)]
	return engine, ok
}

func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}
	names := make([]string, 0, len(r.engines))
	for name := range r.engines {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func normalizeName(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}
