package application

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
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
	ActorID             *uuid.UUID
	TraceID             string
}

type FinalizeDelegationChainCommand struct {
	ChainID uuid.UUID
	ActorID *uuid.UUID
	TraceID string
}

type DisposeDelegationCommand struct {
	ChainID     uuid.UUID
	EdgeID      *uuid.UUID
	Disposition string
	EffectiveAt time.Time
	Reason      string
	EvidenceID  *uuid.UUID
	ActorID     *uuid.UUID
	TraceID     string
}

func (s *Service) DisposeAuthorizationProvenanceBinding(ctx context.Context, cmd DisposeAuthorizationProvenanceBindingCommand) (domain.BindingDisposition, error) {
	kind := strings.ToUpper(strings.TrimSpace(cmd.Disposition))
	if kind != domain.DispositionInvalidated && kind != domain.DispositionSuperseded {
		return domain.BindingDisposition{}, domain.ErrRightsDisposition
	}
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
		var workspace uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT workspace_id FROM authorization_provenance_binding WHERE id=$1 FOR SHARE`, d.BindingID).Scan(&workspace); err != nil {
			return err
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
	return d, err
}

func (s *Service) CreateDelegationChain(ctx context.Context, cmd CreateDelegationChainCommand) (domain.DelegationChain, error) {
	if len(cmd.Edges) == 0 {
		return domain.DelegationChain{}, domain.ErrInvalidBinding
	}
	chain := domain.DelegationChain{ID: uuid.New(), WorkspaceID: cmd.WorkspaceID, SourceDeclarationID: cmd.SourceDeclarationID, Status: "DRAFT", Edges: cmd.Edges, CreatedAt: time.Now().UTC(), CreatedBy: cmd.ActorID}
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.repo.InsertDelegationChain(ctx, tx, chain); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{WorkspaceID: &chain.WorkspaceID, ActorType: actorType(cmd.ActorID), ActorID: cmd.ActorID, Action: "GRANTOR_AUTHORITY_DELEGATION_CHAIN_CREATED", ObjectType: "GRANTOR_AUTHORITY_DELEGATION_CHAIN", ObjectID: chain.ID, AfterState: map[string]any{"sourceDeclarationId": chain.SourceDeclarationID}, TraceID: cmd.TraceID})
	})
	return chain, err
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
	return chain, err
}

func (s *Service) DisposeDelegation(ctx context.Context, cmd DisposeDelegationCommand) error {
	kind := strings.ToUpper(strings.TrimSpace(cmd.Disposition))
	if kind != "REVOKED" && kind != "INVALIDATED" && kind != "SUPERSEDED" || strings.TrimSpace(cmd.Reason) == "" {
		return domain.ErrRightsDisposition
	}
	if cmd.EffectiveAt.IsZero() {
		cmd.EffectiveAt = time.Now().UTC()
	}
	disposition := domain.DelegationDisposition{ID: uuid.New(), ChainID: cmd.ChainID, EdgeID: cmd.EdgeID, Disposition: kind, EffectiveAt: cmd.EffectiveAt.UTC(), Reason: strings.TrimSpace(cmd.Reason), EvidenceID: cmd.EvidenceID, ActorID: cmd.ActorID}
	return s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var workspace uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT workspace_id FROM grantor_authority_delegation_chain WHERE id=$1 FOR SHARE`, cmd.ChainID).Scan(&workspace); err != nil {
			return err
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
		}
		if err := appendEvent(ctx, tx, "GRANTOR_AUTHORITY_DELEGATION_CHAIN", cmd.ChainID, eventType, map[string]any{"chainId": cmd.ChainID, "edgeId": cmd.EdgeID, "dispositionId": disposition.ID, "effectiveAt": disposition.EffectiveAt}); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{WorkspaceID: &workspace, ActorType: actorType(cmd.ActorID), ActorID: cmd.ActorID, Action: action, ObjectType: "GRANTOR_AUTHORITY_DELEGATION_DISPOSITION", ObjectID: disposition.ID, AfterState: map[string]any{"chainId": cmd.ChainID, "edgeId": cmd.EdgeID, "disposition": kind}, TraceID: cmd.TraceID})
	})
}
