// Package agentquality exposes a read-only composition of existing platform facts.
// It deliberately owns no Agent task state, engine, workflow or decision runtime.
package agentquality

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("source not found in workspace")

type Dataset struct {
	ID          uuid.UUID `json:"id"`
	WorkspaceID uuid.UUID `json:"workspaceId"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	DatasetType string    `json:"datasetType"`
}

// Version is a whitelist, not the storage/engine DTO: never return storage URIs,
// credentials or arbitrary metadata as Agent context.
type Version struct {
	ID        uuid.UUID `json:"id"`
	DatasetID uuid.UUID `json:"datasetId"`
	VersionNo int64     `json:"versionNo"`
	Status    string    `json:"status"`
	RowCount  *int64    `json:"rowCount,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type Page[T any] struct {
	Items []T      `json:"items"`
	Page  PageInfo `json:"page"`
}
type PageInfo struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}
type Input struct {
	Name        string `json:"name"`
	DatasetType string `json:"datasetType"`
	Required    bool   `json:"required"`
}
type Template struct {
	Configured   bool    `json:"configured"`
	Name         string  `json:"name,omitempty"`
	Version      string  `json:"version,omitempty"`
	Hash         string  `json:"hash,omitempty"`
	WorkflowHash string  `json:"workflowHash,omitempty"`
	Inputs       []Input `json:"inputs"`
}

type Reader interface {
	Datasets(context.Context, uuid.UUID, int, int) (Page[Dataset], error)
	Dataset(context.Context, uuid.UUID) (Dataset, error)
	Versions(context.Context, uuid.UUID, int, int) (Page[Version], error)
	Version(context.Context, uuid.UUID) (Version, error)
}

type Query struct {
	WorkspaceID, DatasetID, VersionID uuid.UUID
	Limit, Offset                     int
}
type Context struct {
	WorkspaceID        uuid.UUID      `json:"workspaceId"`
	ReadAt             time.Time      `json:"readAt"`
	Mode               string         `json:"mode"`
	Datasets           *Page[Dataset] `json:"datasets,omitempty"`
	Versions           *Page[Version] `json:"versions,omitempty"`
	Dataset            *Dataset       `json:"dataset,omitempty"`
	Version            *Version       `json:"version,omitempty"`
	Template           Template       `json:"template"`
	ExecutionReadiness string         `json:"executionReadiness"`
	Unverified         []string       `json:"unverified"`
}

type Service struct {
	Reader   Reader
	Template Template
}

func (s *Service) Read(ctx context.Context, q Query) (Context, error) {
	result := Context{WorkspaceID: q.WorkspaceID, ReadAt: time.Now().UTC(), Mode: "READ_ONLY",
		Template: s.Template, ExecutionReadiness: "NOT_EVALUATED",
		Unverified: []string{"ACCEPTED_BATCH_THREE_RAW", "MATCHING_ENTITY_RESOLUTION_OUTPUT", "OUTPUT_DATASET_BINDING", "CURRENT_EXECUTION_AUTHORITY"}}
	if result.Template.Inputs == nil {
		result.Template.Inputs = []Input{}
	}
	if q.VersionID != uuid.Nil {
		v, err := s.Reader.Version(ctx, q.VersionID)
		if err != nil {
			return Context{}, err
		}
		if q.DatasetID != uuid.Nil && v.DatasetID != q.DatasetID {
			return Context{}, ErrNotFound
		}
		q.DatasetID = v.DatasetID
		result.Version = &v
	}
	if q.DatasetID == uuid.Nil {
		items, err := s.Reader.Datasets(ctx, q.WorkspaceID, q.Limit, q.Offset)
		if err != nil {
			return Context{}, err
		}
		result.Datasets = &items
		return result, nil
	}
	d, err := s.Reader.Dataset(ctx, q.DatasetID)
	if err != nil {
		return Context{}, err
	}
	if d.WorkspaceID != q.WorkspaceID {
		return Context{}, ErrNotFound
	}
	result.Dataset = &d
	if q.VersionID == uuid.Nil {
		items, err := s.Reader.Versions(ctx, d.ID, q.Limit, q.Offset)
		if err != nil {
			return Context{}, err
		}
		result.Versions = &items
	}
	return result, nil
}
