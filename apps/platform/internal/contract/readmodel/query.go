// Package readmodel owns the slice-0 query contract, not acceptance write facts.
package readmodel

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/contract/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"sort"
	"time"
)

var ErrNotFound = errors.New("scoped contract or delivery not found")

type Query struct {
	WorkspaceID      uuid.UUID
	VersionID        *uuid.UUID
	DeliveryID       *uuid.UUID
	DatasetVersionID *uuid.UUID
	Limit, Offset    int
	BindingOffset    int
	BindingID        *uuid.UUID
	ActorID          uuid.UUID
	Subject          string
	Principal        *principal.RequestPrincipal
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
	ConsumerRef      string    `json:"-"`
}
type Snapshot struct {
	Versions    []StoredVersion
	Total       int
	Delivery    *Delivery
	EvaluatedAt time.Time
	Bindings    []domain.AcceptanceBindingFact
	Authority   *principal.Authority
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
type BindingHistory struct {
	Items []domain.AcceptanceBindingFact `json:"items"`
	Page  struct {
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
		Total  int `json:"total"`
	} `json:"page"`
	Selected *domain.AcceptanceBindingFact  `json:"selected"`
	Related  []domain.AcceptanceBindingFact `json:"related"`
}
type Context struct {
	WorkspaceID          uuid.UUID                         `json:"workspaceId"`
	Delivery             *Delivery                         `json:"delivery"`
	Standards            Page                              `json:"standards"`
	Responsibilities     []domain.AcceptanceResponsibility `json:"responsibilities"`
	Applicability        domain.AcceptanceApplicability    `json:"applicability"`
	BindingHistory       BindingHistory                    `json:"bindingHistory"`
	EvaluatedAt          time.Time                         `json:"evaluatedAt"`
	ReceivingPartyStatus string                            `json:"receivingPartyStatus"`
	Blockers             []string                          `json:"blockers"`
	WriteEnabled         bool                              `json:"writeEnabled"`
	Admission            *Admission                        `json:"admission,omitempty"`
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
	c := &Context{WorkspaceID: q.WorkspaceID, Delivery: stored.Delivery, Standards: p, EvaluatedAt: stored.EvaluatedAt, ReceivingPartyStatus: "UNVERIFIED", Blockers: []string{"ACCEPTANCE_WRITES_DISABLED"}}
	if stored.Delivery == nil {
		return Page{}, nil, ErrNotFound
	}
	if stored.Delivery.Status != "ISSUED" {
		c.Blockers = append(c.Blockers, "DELIVERY_NOT_ISSUED")
	}
	bc := domain.AcceptanceBindingContext{WorkspaceID: q.WorkspaceID.String(), DeliveryID: stored.Delivery.ID.String(), DatasetVersionID: stored.Delivery.DatasetVersionID.String(), ConsumerRef: stored.Delivery.ConsumerRef, ActorID: q.ActorID.String(), Subject: q.Subject, At: stored.EvaluatedAt}
	if q.VersionID == nil {
		c.Blockers = append(c.Blockers, "EXPLICIT_STANDARD_REQUIRED")
	} else if len(p.Items) != 1 {
		return Page{}, nil, ErrNotFound
	} else {
		v := p.Items[0]
		bc.ContractVersionID = v.ID.String()
		bc.SourceSHA256 = v.SourceSHA256
		bc.CriteriaSetHash = v.AcceptanceSource.SetHash
		bc.StandardSupported = v.Status == domain.VersionPublished && v.AcceptanceSource.Status == "SUPPORTED"
		if v.Status != domain.VersionPublished {
			c.Blockers = append(c.Blockers, "CONTRACT_NOT_PUBLISHED")
		}
		if v.AcceptanceSource.Status != "SUPPORTED" {
			c.Blockers = append(c.Blockers, v.AcceptanceSource.Status)
		}
	}
	c.Responsibilities, c.Applicability, c.BindingHistory.Items = domain.EvaluateAcceptanceBindings(bc, stored.Bindings)
	if stored.Authority != nil {
		c.Admission = admissionView(*stored.Authority, c)
	}
	for _, r := range c.Responsibilities {
		if r.Status != "ASSIGNED" {
			c.Blockers = append(c.Blockers, "ROLE_"+r.Role+"_"+r.Status)
		}
		if r.Role == "RECEIVER" && r.Status == "ASSIGNED" {
			c.ReceivingPartyStatus = "VERIFIED"
		}
	}
	if c.ReceivingPartyStatus != "VERIFIED" {
		c.Blockers = append(c.Blockers, "RECEIVING_PARTY_UNVERIFIED")
	}
	if c.Applicability.Status != "MATCHED" {
		c.Blockers = append(c.Blockers, "APPLICABILITY_"+c.Applicability.Status)
	}
	sort.Slice(c.BindingHistory.Items, func(i, j int) bool {
		a, b := c.BindingHistory.Items[i], c.BindingHistory.Items[j]
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.After(b.CreatedAt)
		}
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		return a.Kind < b.Kind
	})
	if q.BindingID != nil {
		matches := 0
		for _, f := range c.BindingHistory.Items {
			if f.ID == q.BindingID.String() {
				copy := f
				c.BindingHistory.Selected = &copy
				matches++
			}
		}
		if matches != 1 {
			return Page{}, nil, ErrNotFound
		}
	}
	c.BindingHistory.Related = relatedBindingFacts(c.BindingHistory.Selected, c.BindingHistory.Items)
	total := len(c.BindingHistory.Items)
	start := q.BindingOffset
	if start > total {
		start = total
	}
	end := start + 25
	if end > total {
		end = total
	}
	c.BindingHistory.Items = c.BindingHistory.Items[start:end]
	c.BindingHistory.Page.Limit = 25
	c.BindingHistory.Page.Offset = q.BindingOffset
	c.BindingHistory.Page.Total = total
	return p, c, nil
}

// Follow immutable typed dependencies and their append-only dispositions. This
// uses the complete scoped history, independently of the requested history page.
func relatedBindingFacts(root *domain.AcceptanceBindingFact, all []domain.AcceptanceBindingFact) []domain.AcceptanceBindingFact {
	out := []domain.AcceptanceBindingFact{}
	if root == nil {
		return out
	}
	seen := map[string]bool{root.Kind + ":" + root.ID: true}
	queue := []domain.AcceptanceBindingFact{*root}
	for len(queue) > 0 {
		f := queue[0]
		queue = queue[1:]
		for _, candidate := range all {
			dependency := (f.Kind == "RESPONSIBILITY" && candidate.Kind == "REPRESENTATION" && f.RepresentationID == candidate.ID) || (f.Kind == "APPLICABILITY" && candidate.Kind == "RESPONSIBILITY" && f.ConfirmerBindingID == candidate.ID) || (f.Kind == "DISPOSITION" && candidate.Kind == f.TargetKind && candidate.ID == f.TargetID) || (candidate.Kind == "DISPOSITION" && candidate.TargetKind == f.Kind && candidate.TargetID == f.ID)
			key := candidate.Kind + ":" + candidate.ID
			if dependency && !seen[key] {
				seen[key] = true
				out = append(out, candidate)
				queue = append(queue, candidate)
			}
		}
	}
	return out
}
