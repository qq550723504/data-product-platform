package application

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
)

type DisposeAuthorizationProvenanceBindingCommand struct {
	BindingID    uuid.UUID
	Disposition  string
	EffectiveAt  time.Time
	Reason       string
	SupersededBy *uuid.UUID
	EvidenceID   *uuid.UUID
	ActivityID   *uuid.UUID
	ActorID      *uuid.UUID
	TraceID      string
}

type CreateDelegationChainCommand struct {
	WorkspaceID         uuid.UUID
	SourceDeclarationID uuid.UUID
	Edges               []domain.DelegationEdge
	ActivityID          *uuid.UUID
	ActorID             *uuid.UUID
	TraceID             string
}

type FinalizeDelegationChainCommand struct {
	ChainID    uuid.UUID
	ActivityID *uuid.UUID
	ActorID    *uuid.UUID
	TraceID    string
}

type DisposeDelegationCommand struct {
	ChainID     uuid.UUID
	EdgeID      *uuid.UUID
	Disposition string
	EffectiveAt time.Time
	Reason      string
	EvidenceID  *uuid.UUID
	ActivityID  *uuid.UUID
	ActorID     *uuid.UUID
	TraceID     string
}

func (s *Service) DisposeAuthorizationProvenanceBinding(ctx context.Context, cmd DisposeAuthorizationProvenanceBindingCommand) (domain.BindingDisposition, error) {
	kind := strings.ToUpper(strings.TrimSpace(cmd.Disposition))
	if kind != domain.DispositionInvalidated && kind != domain.DispositionSuperseded {
		return domain.BindingDisposition{}, domain.ErrRightsDisposition
	}
	effectiveAtProvided := !cmd.EffectiveAt.IsZero()
	if cmd.EffectiveAt.IsZero() {
		cmd.EffectiveAt = time.Now().UTC()
	}
	if strings.TrimSpace(cmd.Reason) == "" || (kind == domain.DispositionSuperseded && cmd.SupersededBy == nil) {
		return domain.BindingDisposition{}, domain.ErrRightsDisposition
	}
	if cmd.ActivityID == nil {
		id := uuid.New()
		cmd.ActivityID = &id
	}
	d := domain.BindingDisposition{ID: uuid.New(), BindingID: cmd.BindingID, Disposition: kind, EffectiveAt: cmd.EffectiveAt.UTC(), Reason: strings.TrimSpace(cmd.Reason), SupersededBy: cmd.SupersededBy, EvidenceID: cmd.EvidenceID, ActivityID: cmd.ActivityID, ActorID: cmd.ActorID}
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var workspace, authorizationID, resourceID uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT workspace_id,authorization_id,data_resource_id FROM authorization_provenance_binding WHERE id=$1 FOR SHARE`, d.BindingID).Scan(&workspace, &authorizationID, &resourceID); err != nil {
			return err
		}
		if d.EvidenceID != nil {
			var evidenceWorkspace uuid.UUID
			if err := tx.QueryRow(ctx, `SELECT workspace_id FROM evidence WHERE id=$1`, *d.EvidenceID).Scan(&evidenceWorkspace); err != nil || evidenceWorkspace != workspace {
				return domain.ErrRightsDisposition
			}
		}
		if d.SupersededBy != nil {
			var replacementWorkspace, replacementAuthorizationID, replacementResourceID uuid.UUID
			if err := tx.QueryRow(ctx, `SELECT workspace_id,authorization_id,data_resource_id FROM authorization_provenance_binding WHERE id=$1 FOR SHARE`, *d.SupersededBy).Scan(&replacementWorkspace, &replacementAuthorizationID, &replacementResourceID); err != nil {
				return err
			}
			if *d.SupersededBy == d.BindingID || replacementWorkspace != workspace || replacementAuthorizationID != authorizationID || replacementResourceID != resourceID {
				return domain.ErrRightsDisposition
			}
		}
		if _, err := deliveryfence.Advance(ctx, tx, workspace); err != nil {
			return err
		}
		if err := s.repo.InsertBindingDisposition(ctx, tx, d); err != nil {
			return err
		}
		eventType, action := "AuthorizationProvenanceBindingInvalidated", "AUTHORIZATION_PROVENANCE_BINDING_INVALIDATED"
		if kind == domain.DispositionSuperseded {
			eventType, action = "AuthorizationProvenanceBindingSuperseded", "AUTHORIZATION_PROVENANCE_BINDING_SUPERSEDED"
		}
		if err := appendEvent(ctx, tx, "AUTHORIZATION_PROVENANCE_BINDING", d.BindingID, eventType, map[string]any{"bindingId": d.BindingID, "dispositionId": d.ID, "effectiveAt": d.EffectiveAt}); err != nil {
			return err
		}
		if err := appendRightsCost(ctx, tx, workspace, *cmd.ActivityID, "AUTHORIZATION_PROVENANCE_BINDING_DISPOSITION", "authorization_provenance_binding_disposition_id", d.ID); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{WorkspaceID: &workspace, ActorType: actorType(cmd.ActorID), ActorID: cmd.ActorID, Action: action, ObjectType: "AUTHORIZATION_PROVENANCE_BINDING_DISPOSITION", ObjectID: d.ID, AfterState: map[string]any{"bindingId": d.BindingID, "disposition": kind}, TraceID: cmd.TraceID})
	})
	if errors.Is(err, infrastructure.ErrBindingDispositionIdempotentReplay) {
		existing, findErr := s.repo.GetBindingDispositionByActivityID(ctx, d.BindingID, kind, *d.ActivityID)
		if findErr != nil {
			return domain.BindingDisposition{}, findErr
		}
		if !sameBindingDisposition(existing, d, effectiveAtProvided) {
			return domain.BindingDisposition{}, domain.ErrRightsDisposition
		}
		return existing, nil
	}
	return d, err
}

func sameBindingDisposition(existing, requested domain.BindingDisposition, compareEffectiveAt bool) bool {
	if existing.BindingID != requested.BindingID || existing.Disposition != requested.Disposition || (compareEffectiveAt && !existing.EffectiveAt.Equal(requested.EffectiveAt)) || existing.Reason != requested.Reason {
		return false
	}
	if (existing.SupersededBy == nil) != (requested.SupersededBy == nil) || (existing.EvidenceID == nil) != (requested.EvidenceID == nil) || (existing.ActorID == nil) != (requested.ActorID == nil) {
		return false
	}
	if existing.SupersededBy != nil && *existing.SupersededBy != *requested.SupersededBy {
		return false
	}
	if existing.EvidenceID != nil && *existing.EvidenceID != *requested.EvidenceID {
		return false
	}
	return existing.ActorID == nil || *existing.ActorID == *requested.ActorID
}

func (s *Service) CreateDelegationChain(ctx context.Context, cmd CreateDelegationChainCommand) (domain.DelegationChain, error) {
	if len(cmd.Edges) == 0 {
		return domain.DelegationChain{}, domain.ErrInvalidBinding
	}
	chain := domain.DelegationChain{ID: uuid.New(), WorkspaceID: cmd.WorkspaceID, SourceDeclarationID: cmd.SourceDeclarationID, Status: "DRAFT", Edges: cmd.Edges, CreatedAt: time.Now().UTC(), CreatedBy: cmd.ActorID}
	if cmd.ActivityID != nil {
		chain.ID = uuid.NewSHA1(uuid.NameSpaceURL, []byte("grantor-delegation-chain-create:"+cmd.WorkspaceID.String()+":"+cmd.ActivityID.String()))
	}
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.repo.InsertDelegationChain(ctx, tx, chain); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{WorkspaceID: &chain.WorkspaceID, ActorType: actorType(cmd.ActorID), ActorID: cmd.ActorID, Action: "GRANTOR_AUTHORITY_DELEGATION_CHAIN_CREATED", ObjectType: "GRANTOR_AUTHORITY_DELEGATION_CHAIN", ObjectID: chain.ID, AfterState: map[string]any{"sourceDeclarationId": chain.SourceDeclarationID}, TraceID: cmd.TraceID})
	})
	if errors.Is(err, infrastructure.ErrDelegationChainIdempotentReplay) {
		existing, findErr := s.repo.GetDelegationChain(ctx, chain.ID)
		if findErr != nil {
			return domain.DelegationChain{}, findErr
		}
		if !sameDelegationChain(existing, chain) {
			return domain.DelegationChain{}, domain.ErrInvalidBinding
		}
		return existing, nil
	}
	return chain, err
}

func sameDelegationChain(existing, requested domain.DelegationChain) bool {
	if existing.ID != requested.ID || existing.WorkspaceID != requested.WorkspaceID || existing.SourceDeclarationID != requested.SourceDeclarationID || (existing.CreatedBy == nil) != (requested.CreatedBy == nil) {
		return false
	}
	if existing.CreatedBy != nil && *existing.CreatedBy != *requested.CreatedBy {
		return false
	}
	if len(existing.Edges) != len(requested.Edges) {
		return false
	}
	for i := range existing.Edges {
		left, right := existing.Edges[i], requested.Edges[i]
		if left.ID != right.ID || left.Ordinal != right.Ordinal || left.DelegatorRef != right.DelegatorRef || left.DelegateRef != right.DelegateRef || left.DataResourceID != right.DataResourceID || left.Scope != right.Scope || !sameTime(left.ValidFrom, right.ValidFrom) || !sameTime(left.ValidTo, right.ValidTo) || !sameStringSet(left.GrantableActions, right.GrantableActions) || !sameStringSet(left.GrantablePurposes, right.GrantablePurposes) {
			return false
		}
	}
	return true
}

func sameTime(left, right *time.Time) bool {
	if (left == nil) != (right == nil) {
		return false
	}
	return left == nil || left.Equal(*right)
}

func sameStringSet(left, right []string) bool {
	left = append([]string(nil), left...)
	right = append([]string(nil), right...)
	sort.Strings(left)
	sort.Strings(right)
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func (s *Service) FinalizeDelegationChain(ctx context.Context, cmd FinalizeDelegationChainCommand) (domain.DelegationChain, error) {
	var chain domain.DelegationChain
	var err error
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var finalizeErr error
		chain, finalizeErr = s.repo.FinalizeDelegationChain(ctx, tx, cmd.ChainID)
		if finalizeErr != nil {
			return finalizeErr
		}
		if err := appendEvent(ctx, tx, "GRANTOR_AUTHORITY_DELEGATION_CHAIN", chain.ID, "GrantorAuthorityDelegationChainFinalized", map[string]any{"chainId": chain.ID, "chainHash": chain.ChainHash}); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{WorkspaceID: &chain.WorkspaceID, ActorType: actorType(cmd.ActorID), ActorID: cmd.ActorID, Action: "GRANTOR_AUTHORITY_DELEGATION_CHAIN_FINALIZED", ObjectType: "GRANTOR_AUTHORITY_DELEGATION_CHAIN", ObjectID: chain.ID, AfterState: map[string]any{"chainHash": chain.ChainHash}, TraceID: cmd.TraceID})
	})
	if errors.Is(err, infrastructure.ErrDelegationFinalizeIdempotentReplay) {
		existing, findErr := s.repo.GetDelegationChain(ctx, cmd.ChainID)
		if findErr != nil {
			return domain.DelegationChain{}, findErr
		}
		if existing.Status != "FINALIZED" {
			return domain.DelegationChain{}, domain.ErrInvalidBinding
		}
		return existing, nil
	}
	return chain, err
}

func (s *Service) DisposeDelegation(ctx context.Context, cmd DisposeDelegationCommand) (domain.DelegationDisposition, error) {
	kind := strings.ToUpper(strings.TrimSpace(cmd.Disposition))
	if kind != "REVOKED" && kind != "INVALIDATED" && kind != "SUPERSEDED" || strings.TrimSpace(cmd.Reason) == "" {
		return domain.DelegationDisposition{}, domain.ErrRightsDisposition
	}
	effectiveAtProvided := !cmd.EffectiveAt.IsZero()
	if cmd.EffectiveAt.IsZero() {
		cmd.EffectiveAt = time.Now().UTC()
	}
	if cmd.ActivityID == nil {
		id := uuid.New()
		cmd.ActivityID = &id
	}
	disposition := domain.DelegationDisposition{ID: uuid.New(), ChainID: cmd.ChainID, EdgeID: cmd.EdgeID, Disposition: kind, EffectiveAt: cmd.EffectiveAt.UTC(), Reason: strings.TrimSpace(cmd.Reason), EvidenceID: cmd.EvidenceID, ActivityID: cmd.ActivityID, ActorID: cmd.ActorID}
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var workspace uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT workspace_id FROM grantor_authority_delegation_chain WHERE id=$1 FOR SHARE`, cmd.ChainID).Scan(&workspace); err != nil {
			return err
		}
		if disposition.EvidenceID != nil {
			var evidenceWorkspace uuid.UUID
			if err := tx.QueryRow(ctx, `SELECT workspace_id FROM evidence WHERE id=$1`, *disposition.EvidenceID).Scan(&evidenceWorkspace); err != nil || evidenceWorkspace != workspace {
				return domain.ErrRightsDisposition
			}
		}
		if disposition.EdgeID != nil {
			var edgeChainID uuid.UUID
			if err := tx.QueryRow(ctx, `SELECT chain_id FROM grantor_authority_delegation_edge WHERE id=$1 FOR SHARE`, *disposition.EdgeID).Scan(&edgeChainID); err != nil {
				return err
			}
			if edgeChainID != cmd.ChainID {
				return domain.ErrRightsDisposition
			}
		}
		if _, err := deliveryfence.Advance(ctx, tx, workspace); err != nil {
			return err
		}
		if err := s.repo.InsertDelegationDisposition(ctx, tx, disposition); err != nil {
			return err
		}
		eventType, action := "GrantorAuthorityDelegationInvalidated", "GRANTOR_AUTHORITY_DELEGATION_INVALIDATED"
		if kind == "REVOKED" {
			eventType = "GrantorAuthorityDelegationRevoked"
			action = "GRANTOR_AUTHORITY_DELEGATION_REVOKED"
		} else if kind == "SUPERSEDED" {
			eventType = "GrantorAuthorityDelegationSuperseded"
			action = "GRANTOR_AUTHORITY_DELEGATION_SUPERSEDED"
		}
		if err := appendEvent(ctx, tx, "GRANTOR_AUTHORITY_DELEGATION_CHAIN", cmd.ChainID, eventType, map[string]any{"chainId": cmd.ChainID, "edgeId": cmd.EdgeID, "dispositionId": disposition.ID, "effectiveAt": disposition.EffectiveAt}); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{WorkspaceID: &workspace, ActorType: actorType(cmd.ActorID), ActorID: cmd.ActorID, Action: action, ObjectType: "GRANTOR_AUTHORITY_DELEGATION_DISPOSITION", ObjectID: disposition.ID, AfterState: map[string]any{"chainId": cmd.ChainID, "edgeId": cmd.EdgeID, "disposition": kind}, TraceID: cmd.TraceID})
	})
	if errors.Is(err, infrastructure.ErrDelegationDispositionIdempotentReplay) {
		existing, findErr := s.repo.GetDelegationDispositionByActivityID(ctx, cmd.ChainID, kind, *disposition.ActivityID)
		if findErr != nil {
			return domain.DelegationDisposition{}, findErr
		}
		if !sameDelegationDisposition(existing, disposition, effectiveAtProvided) {
			return domain.DelegationDisposition{}, domain.ErrRightsDisposition
		}
		return existing, nil
	}
	return disposition, err
}

func sameDelegationDisposition(existing, requested domain.DelegationDisposition, compareEffectiveAt bool) bool {
	if existing.ChainID != requested.ChainID || existing.Disposition != requested.Disposition || (compareEffectiveAt && !existing.EffectiveAt.Equal(requested.EffectiveAt)) || existing.Reason != requested.Reason {
		return false
	}
	if (existing.EdgeID == nil) != (requested.EdgeID == nil) || (existing.EvidenceID == nil) != (requested.EvidenceID == nil) || (existing.ActorID == nil) != (requested.ActorID == nil) {
		return false
	}
	if existing.EdgeID != nil && *existing.EdgeID != *requested.EdgeID {
		return false
	}
	if existing.EvidenceID != nil && *existing.EvidenceID != *requested.EvidenceID {
		return false
	}
	return existing.ActorID == nil || *existing.ActorID == *requested.ActorID
}
