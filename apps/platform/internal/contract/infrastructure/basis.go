package infrastructure

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	q "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/readmodel"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"sort"
	"time"
)

func validBasisQuery(query q.BasisQuery) bool {
	return query.DeliveryID != nil && *query.DeliveryID != uuid.Nil && query.DatasetVersionID != nil && *query.DatasetVersionID != uuid.Nil && query.MaterialOffset >= 0 && query.MaterialOffset <= 2147483647
}

func (r AcceptanceIdentityReader) ReadBusinessBasis(ctx context.Context, query q.BasisQuery) (q.Basis, error) {
	result := q.Basis{ProfileVersion: "business-basis.v1", ResponsibilitySource: "UNCONFIGURED", StandardSource: "CORE_RECORD_ONLY", ReceiptSource: "UNCONFIGURED", Evidence: []q.BasisEvidence{}, Blockers: []string{"ACCEPTANCE_WRITES_DISABLED", "DIRECTORY_SOURCE_UNCONFIGURED", "RESPONSIBILITY_SOURCE_UNCONFIGURED", "STANDARD_AUTHORITY_UNCONFIGURED", "RECEIPT_SOURCE_UNCONFIGURED"}}
	if !validBasisQuery(query) || query.MaterialID != nil {
		return result, q.ErrNotFound
	}
	err := r.withAcceptanceAuthority(ctx, query.Query, func(tx pgx.Tx, a principal.Authority, s q.Snapshot) error {
		query.ActorID = a.Principal.ActorID
		query.Subject = a.Principal.Subject
		_, c, e := q.Project(query.Query, s)
		if e != nil {
			return e
		}
		result.Context = c
		result.Identity = q.IdentityBasis{PrincipalID: a.Principal.ActorID, WorkspaceID: a.Principal.WorkspaceID, SubjectBindingID: a.SubjectBindingID, WorkspaceGrantID: a.WorkspaceGrantID, AuthorityRevision: a.Principal.IdentityRevision, Authentication: "OIDC_CONFIRMED", DirectorySource: "UNCONFIGURED"}
		repo := evidence.NewQueryRepository(tx)
		// Inspect exact dependencies of the full scope, never the history page only.
		current := map[string]bool{}
		for _, v := range c.Responsibilities {
			if v.Binding != nil {
				current[v.Binding.Kind+":"+v.Binding.ID] = true
			}
			if v.Representative != nil {
				current[v.Representative.Kind+":"+v.Representative.ID] = true
			}
		}
		if c.Applicability.Binding != nil {
			current[c.Applicability.Binding.Kind+":"+c.Applicability.Binding.ID] = true
		}
		type fact struct {
			id           uuid.UUID
			kind, origin string
			basis        uuid.UUID
		}
		facts := []fact{}
		ids := []uuid.UUID{}
		seen := map[uuid.UUID]bool{}
		for _, f := range s.Bindings {
			if !current[f.Kind+":"+f.ID] || f.Kind == "DISPOSITION" {
				continue
			}
			id, e := uuid.Parse(f.ID)
			if e != nil {
				return e
			}
			basis, e := uuid.Parse(f.BasisEvidenceID)
			if e != nil {
				return e
			}
			facts = append(facts, fact{id, f.Kind, f.Origin, basis})
			if !seen[basis] {
				seen[basis] = true
				ids = append(ids, basis)
			}
		}
		if len(ids) > 13 {
			return errors.New("current basis evidence scope exceeds inspection bound")
		}
		refs, e := repo.InspectAcceptanceReferences(ctx, query.WorkspaceID, ids)
		if e != nil {
			return e
		}
		byID := map[uuid.UUID]evidence.IntegrityReference{}
		for _, ref := range refs {
			byID[ref.ID] = ref
		}
		sort.Slice(facts, func(i, j int) bool { return facts[i].id.String() < facts[j].id.String() })
		for _, f := range facts {
			result.Evidence = append(result.Evidence, q.BasisEvidence{FactID: f.id, Kind: f.kind, Origin: f.origin, Evidence: byID[f.basis]})
		}
		page, e := repo.AcceptanceMaterialReferences(ctx, query.WorkspaceID, *query.DeliveryID, *query.DatasetVersionID, query.MaterialOffset, nil)
		if e != nil {
			return e
		}
		result.Materials.Items = []q.Material{}
		result.Materials.Page.Limit = 25
		result.Materials.Page.Offset = query.MaterialOffset
		result.Materials.Page.Total = page.Total
		for _, ref := range page.Items {
			result.Materials.Items = append(result.Materials.Items, q.InspectMaterial(ref))
		}
		if page.Total == 0 {
			result.Blockers = append(result.Blockers, "RECEIPT_MATERIAL_UNAVAILABLE")
		}
		return nil
	})
	return result, err
}

func (r AcceptanceIdentityReader) ReadReceiptMaterial(ctx context.Context, query q.BasisQuery) (q.MaterialDetail, error) {
	var result q.MaterialDetail
	if !validBasisQuery(query) || query.MaterialID == nil || *query.MaterialID == uuid.Nil || query.MaterialOffset != 0 || query.VersionID != nil {
		return result, q.ErrNotFound
	}
	err := r.withAcceptanceAuthority(ctx, query.Query, func(tx pgx.Tx, a principal.Authority, _ q.Snapshot) error {
		page, e := evidence.NewQueryRepository(tx).AcceptanceMaterialReferences(ctx, query.WorkspaceID, *query.DeliveryID, *query.DatasetVersionID, 0, query.MaterialID)
		if errors.Is(e, pgx.ErrNoRows) {
			return q.ErrNotFound
		}
		if e != nil {
			return e
		}
		if len(page.Items) != 1 || page.Items[0].EnvelopeStatus == "MISSING" {
			return q.ErrNotFound
		}
		result = q.MaterialDetail{ProfileVersion: "business-basis.v1", WorkspaceID: query.WorkspaceID, DeliveryOperationID: *query.DeliveryID, DatasetVersionID: *query.DatasetVersionID, PrincipalID: a.Principal.ActorID, AuthorityRevision: a.Principal.IdentityRevision, EvaluatedAt: a.EvaluatedAt.UTC().Format(time.RFC3339Nano), Material: q.InspectMaterial(page.Items[0])}
		return nil
	})
	return result, err
}
