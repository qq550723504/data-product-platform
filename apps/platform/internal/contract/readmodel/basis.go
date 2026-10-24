package readmodel

import (
	"context"
	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
)

type BasisQuery struct {
	Query
	MaterialOffset int
	MaterialID     *uuid.UUID
}
type BasisReader interface {
	ReadBusinessBasis(context.Context, BasisQuery) (Basis, error)
	ReadReceiptMaterial(context.Context, BasisQuery) (MaterialDetail, error)
}
type IdentityBasis struct {
	PrincipalID       uuid.UUID `json:"principalId"`
	WorkspaceID       uuid.UUID `json:"workspaceId"`
	SubjectBindingID  uuid.UUID `json:"subjectBindingId"`
	WorkspaceGrantID  uuid.UUID `json:"workspaceGrantId"`
	AuthorityRevision string    `json:"authorityRevision"`
	Authentication    string    `json:"authentication"`
	DirectorySource   string    `json:"directorySource"`
}
type BasisEvidence struct {
	FactID   uuid.UUID                   `json:"factId"`
	Kind     string                      `json:"kind"`
	Origin   string                      `json:"origin"`
	Evidence evidence.IntegrityReference `json:"evidence"`
}
type Material struct {
	Evidence        evidence.IntegrityReference `json:"evidence"`
	ContentStatus   string                      `json:"contentStatus"`
	RecipientStatus string                      `json:"recipientStatus"`
	BusinessUse     string                      `json:"businessUse"`
	Blockers        []string                    `json:"blockers"`
}

func InspectMaterial(ref evidence.IntegrityReference) Material {
	m := Material{Evidence: ref, ContentStatus: "UNATTESTED", RecipientStatus: "UNATTESTED", BusinessUse: "BLOCKED", Blockers: []string{"RECEIPT_SOURCE_UNCONFIGURED", "CONTENT_NOT_ATTESTED", "RECIPIENT_NOT_ATTESTED"}}
	if ref.EnvelopeStatus != "VALID" {
		m.Blockers = append(m.Blockers, "EVIDENCE_ENVELOPE_"+ref.EnvelopeStatus)
	}
	if ref.ClaimKind != "RECEIPT_CLAIM" {
		m.Blockers = append(m.Blockers, "NOT_A_RECEIPT_CLAIM")
	}
	return m
}

type MaterialPage struct {
	Items []Material `json:"items"`
	Page  struct {
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
		Total  int `json:"total"`
	} `json:"page"`
}
type Basis struct {
	ProfileVersion       string          `json:"profileVersion"`
	Context              *Context        `json:"context"`
	Identity             IdentityBasis   `json:"identity"`
	ResponsibilitySource string          `json:"responsibilitySource"`
	StandardSource       string          `json:"standardSource"`
	ReceiptSource        string          `json:"receiptSource"`
	Evidence             []BasisEvidence `json:"evidence"`
	Materials            MaterialPage    `json:"materials"`
	WriteEnabled         bool            `json:"writeEnabled"`
	Blockers             []string        `json:"blockers"`
}
type MaterialDetail struct {
	ProfileVersion      string    `json:"profileVersion"`
	WorkspaceID         uuid.UUID `json:"workspaceId"`
	DeliveryOperationID uuid.UUID `json:"deliveryOperationId"`
	DatasetVersionID    uuid.UUID `json:"datasetVersionId"`
	PrincipalID         uuid.UUID `json:"principalId"`
	AuthorityRevision   string    `json:"authorityRevision"`
	EvaluatedAt         string    `json:"evaluatedAt"`
	Material            Material  `json:"material"`
	WriteEnabled        bool      `json:"writeEnabled"`
}
