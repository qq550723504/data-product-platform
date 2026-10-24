// Package readmodel owns the slice-0 query contract, not acceptance write facts.
package readmodel

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/contract/domain"
	"time"
)

var ErrNotFound = errors.New("scoped contract or delivery not found")

type Query struct {
	WorkspaceID      uuid.UUID
	VersionID        *uuid.UUID
	DeliveryID       *uuid.UUID
	DatasetVersionID *uuid.UUID
	Limit, Offset    int
}
type StoredVersion struct {
	WorkspaceID uuid.UUID
	Name        string
	Version     domain.ContractVersion
}
type Delivery struct {
	ID               uuid.UUID `json:"id"`
	DatasetVersionID uuid.UUID `json:"datasetVersionId"`
	Status           string    `json:"status"`
}
type Snapshot struct {
	Versions []StoredVersion
	Total    int
	Delivery *Delivery
}
type Reader interface {
	ReadAcceptanceSources(context.Context, Query) (Snapshot, error)
}
type Version struct {
	ID               uuid.UUID               `json:"id"`
	WorkspaceID      uuid.UUID               `json:"workspaceId"`
	ContractID       uuid.UUID               `json:"contractId"`
	Name             string                  `json:"name"`
	Version          string                  `json:"version"`
	Status           domain.VersionStatus    `json:"status"`
	SourceSHA256     string                  `json:"sourceSha256"`
	CreatedAt        time.Time               `json:"createdAt"`
	PublishedAt      *time.Time              `json:"publishedAt"`
	AcceptanceSource domain.AcceptanceSource `json:"acceptanceSource"`
}
type Page struct {
	WorkspaceID uuid.UUID `json:"workspaceId"`
	Items       []Version `json:"items"`
	Page        struct {
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
		Total  int `json:"total"`
	} `json:"page"`
}
type Responsibility struct {
	Role   string `json:"role"`
	Status string `json:"status"`
}
type Context struct {
	WorkspaceID          uuid.UUID        `json:"workspaceId"`
	Delivery             *Delivery        `json:"delivery"`
	Standards            Page             `json:"standards"`
	Responsibilities     []Responsibility `json:"responsibilities"`
	ReceivingPartyStatus string           `json:"receivingPartyStatus"`
	Blockers             []string         `json:"blockers"`
	WriteEnabled         bool             `json:"writeEnabled"`
}
type Service struct{ Reader Reader }

func (s Service) Read(ctx context.Context, q Query) (Page, *Context, error) {
	stored, err := s.Reader.ReadAcceptanceSources(ctx, q)
	if err != nil {
		return Page{}, nil, err
	}
	p := Page{WorkspaceID: q.WorkspaceID, Items: []Version{}}
	p.Page.Limit = q.Limit
	p.Page.Offset = q.Offset
	p.Page.Total = stored.Total
	for _, v := range stored.Versions {
		p.Items = append(p.Items, Version{ID: v.Version.ID, WorkspaceID: v.WorkspaceID, ContractID: v.Version.ContractID, Name: v.Name, Version: v.Version.Semver(), Status: v.Version.Status, SourceSHA256: v.Version.SourceSHA256, CreatedAt: v.Version.CreatedAt, PublishedAt: v.Version.PublishedAt, AcceptanceSource: domain.ReadAcceptanceSource(v.Version.Document)})
	}
	if q.DeliveryID == nil {
		return p, nil, nil
	}
	c := &Context{WorkspaceID: q.WorkspaceID, Delivery: stored.Delivery, Standards: p, ReceivingPartyStatus: "UNVERIFIED", Responsibilities: []Responsibility{}, Blockers: []string{"RESPONSIBILITIES_NOT_CONFIGURED", "RECEIVING_PARTY_UNVERIFIED", "ACCEPTANCE_WRITES_DISABLED"}}
	for _, role := range []string{"PROVIDER", "RECEIVER", "BASIS_CONFIRMER", "REVIEWER", "ARCHIVIST", "DISPOSER"} {
		c.Responsibilities = append(c.Responsibilities, Responsibility{Role: role, Status: "UNASSIGNED"})
	}
	if stored.Delivery == nil {
		return Page{}, nil, ErrNotFound
	}
	if stored.Delivery.Status != "ISSUED" {
		c.Blockers = append(c.Blockers, "DELIVERY_NOT_ISSUED")
	}
	if q.VersionID == nil {
		c.Blockers = append(c.Blockers, "EXPLICIT_STANDARD_REQUIRED")
	} else if len(p.Items) != 1 {
		return Page{}, nil, ErrNotFound
	} else {
		v := p.Items[0]
		if v.Status != domain.VersionPublished {
			c.Blockers = append(c.Blockers, "CONTRACT_NOT_PUBLISHED")
		}
		if v.AcceptanceSource.Status != "SUPPORTED" {
			c.Blockers = append(c.Blockers, v.AcceptanceSource.Status)
		}
	}
	return p, c, nil
}
