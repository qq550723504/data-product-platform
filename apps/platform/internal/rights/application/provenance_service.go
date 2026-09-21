package application

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
)

type CreateRightsDeclarationCommand struct {
	Spec       domain.RightsDeclarationSpec
	TraceID    string
	ActivityID *uuid.UUID
	EvidenceID *uuid.UUID
}

type VerifyRightsDeclarationCommand struct {
	DeclarationID uuid.UUID
	Outcome       string
	Reason        string
	EvidenceID    *uuid.UUID
	ActivityID    *uuid.UUID
	ActorID       *uuid.UUID
	TraceID       string
}

type DisposeRightsDeclarationCommand struct {
	DeclarationID uuid.UUID
	Disposition   string
	EffectiveAt   time.Time
	Reason        string
	SupersededBy  *uuid.UUID
	EvidenceID    *uuid.UUID
	ActivityID    *uuid.UUID
	ActorID       *uuid.UUID
	TraceID       string
}

type BindAuthorizationProvenanceCommand struct {
	WorkspaceID         uuid.UUID
	AuthorizationID     uuid.UUID
	DataResourceID      uuid.UUID
	DeclarationID       uuid.UUID
	GrantorRef          string
	AuthorityMode       string
	DelegationChainID   *uuid.UUID
	DelegationChainHash string
	AsOf                time.Time
	ActivityID          *uuid.UUID
	ActorID             *uuid.UUID
	TraceID             string
}

type CheckCurrentEntitlementCommand struct {
	domain.EntitlementRequest
	TraceID string
}

type ComputeEffectiveRightsCommand struct {
	WorkspaceID            uuid.UUID
	TargetDatasetVersionID uuid.UUID
	ConsumerRef            string
	Purpose                string
	AsOf                   time.Time
	AsOfProvided           bool
	EvidenceID             *uuid.UUID
	ActivityID             *uuid.UUID
	ActorID                *uuid.UUID
	TraceID                string
}

var ErrRightsDeclarationIdempotencyConflict = errors.New("rights declaration idempotency key was reused with different input")

func (s *Service) CreateRightsDeclaration(ctx context.Context, cmd CreateRightsDeclarationCommand) (domain.RightsDeclaration, error) {
	d, err := domain.NewRightsDeclaration(cmd.Spec)
	if err != nil {
		return d, err
	}
	if cmd.ActivityID != nil {
		d.ID = uuid.NewSHA1(uuid.NameSpaceURL, []byte("rights-declaration-create:"+d.WorkspaceID.String()+":"+cmd.ActivityID.String()))
	}
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.repo.InsertRightsDeclaration(ctx, tx, d); err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, "RIGHTS_DECLARATION", d.ID, "RightsDeclarationCreated", map[string]any{"rightsDeclarationId": d.ID, "dataResourceId": d.DataResourceID, "claimantRef": d.ClaimantRef}); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{WorkspaceID: &d.WorkspaceID, ActorType: actorType(cmd.Spec.ActorID), ActorID: cmd.Spec.ActorID, Action: "RIGHTS_DECLARATION_CREATED", ObjectType: "RIGHTS_DECLARATION", ObjectID: d.ID, AfterState: map[string]any{"dataResourceId": d.DataResourceID, "claimantRef": d.ClaimantRef, "basisType": d.BasisType}, TraceID: cmd.TraceID})
	})
	if errors.Is(err, infrastructure.ErrDeclarationIdempotentReplay) {
		existing, findErr := s.repo.GetRightsDeclaration(ctx, d.ID)
		if findErr != nil {
			return domain.RightsDeclaration{}, findErr
		}
		if declarationFingerprint(existing) != declarationFingerprint(d) {
			return domain.RightsDeclaration{}, ErrRightsDeclarationIdempotencyConflict
		}
		return existing, nil
	}
	return d, err
}

func declarationFingerprint(d domain.RightsDeclaration) string {
	parties := append([]domain.RightsParty(nil), d.Parties...)
	sort.Slice(parties, func(i, j int) bool {
		if parties[i].PartyRef != parties[j].PartyRef {
			return parties[i].PartyRef < parties[j].PartyRef
		}
		return parties[i].Role < parties[j].Role
	})
	permissions := append([]domain.RightsPermission(nil), d.Permissions...)
	sort.Slice(permissions, func(i, j int) bool {
		left := strings.Join([]string{permissions[i].Kind, permissions[i].Action, permissions[i].Purpose, permissions[i].Scope.Type, permissions[i].Scope.Ref}, "\x00")
		right := strings.Join([]string{permissions[j].Kind, permissions[j].Action, permissions[j].Purpose, permissions[j].Scope.Type, permissions[j].Scope.Ref}, "\x00")
		return left < right
	})
	evidence := make([]string, 0, len(d.EvidenceIDs))
	for _, id := range d.EvidenceIDs {
		evidence = append(evidence, id.String())
	}
	sort.Strings(evidence)
	payload, _ := json.Marshal(struct {
		WorkspaceID       uuid.UUID                 `json:"workspaceId"`
		DataResourceID    uuid.UUID                 `json:"dataResourceId"`
		ClaimantRef       string                    `json:"claimantRef"`
		BasisType         string                    `json:"basisType"`
		BasisRef          string                    `json:"basisRef"`
		ConsumerScopeType string                    `json:"consumerScopeType"`
		ConsumerRef       string                    `json:"consumerRef"`
		EffectiveFrom     *time.Time                `json:"effectiveFrom"`
		EffectiveTo       *time.Time                `json:"effectiveTo"`
		Parties           []domain.RightsParty      `json:"parties"`
		Permissions       []domain.RightsPermission `json:"permissions"`
		Restrictions      map[string]any            `json:"restrictions"`
		EvidenceIDs       []string                  `json:"evidenceIds"`
		CreatedBy         *uuid.UUID                `json:"createdBy"`
	}{d.WorkspaceID, d.DataResourceID, d.ClaimantRef, d.BasisType, d.BasisRef, d.ConsumerScopeType, d.ConsumerRef, d.EffectiveFrom, d.EffectiveTo, parties, permissions, d.Restrictions, evidence, d.CreatedBy})
	hash := sha256.Sum256(payload)
	return fmt.Sprintf("%x", hash[:])
}

func (s *Service) VerifyRightsDeclaration(ctx context.Context, cmd VerifyRightsDeclarationCommand) (domain.RightsVerification, error) {
	outcome := strings.ToUpper(strings.TrimSpace(cmd.Outcome))
	if outcome != domain.DeclarationVerified && outcome != domain.DeclarationRejected {
		return domain.RightsVerification{}, domain.ErrDeclarationTerminal
	}
	d, err := s.repo.GetRightsDeclaration(ctx, cmd.DeclarationID)
	if err != nil {
		return domain.RightsVerification{}, err
	}
	verification := domain.RightsVerification{ID: uuid.New(), DeclarationID: d.ID, Outcome: outcome, Reason: strings.TrimSpace(cmd.Reason), EvidenceID: cmd.EvidenceID, OccurredAt: time.Now().UTC(), ActorID: cmd.ActorID, ActivityID: cmd.ActivityID}
	if verification.ActivityID == nil {
		id := uuid.New()
		verification.ActivityID = &id
	}
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var locked uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM rights_declaration WHERE id=$1 FOR UPDATE`, d.ID).Scan(&locked); err != nil {
			return err
		}
		if verification.EvidenceID != nil {
			var evidenceWorkspace uuid.UUID
			if err := tx.QueryRow(ctx, `SELECT workspace_id FROM evidence WHERE id=$1`, *verification.EvidenceID).Scan(&evidenceWorkspace); err != nil || evidenceWorkspace != d.WorkspaceID {
				return domain.ErrInvalidRightsDeclaration
			}
		}
		var replay bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM rights_declaration_verification WHERE declaration_id=$1 AND activity_id=$2)`, d.ID, *verification.ActivityID).Scan(&replay); err != nil {
			return err
		}
		if replay {
			return infrastructure.ErrVerificationIdempotentReplay
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM rights_declaration_verification WHERE declaration_id=$1)`, d.ID).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return domain.ErrDeclarationTerminal
		}
		if err := s.repo.InsertDeclarationVerification(ctx, tx, verification); err != nil {
			return err
		}
		eventType, action := "RightsDeclarationRejected", "RIGHTS_DECLARATION_REJECTED"
		if outcome == domain.DeclarationVerified {
			eventType = "RightsDeclarationVerified"
			action = "RIGHTS_DECLARATION_VERIFIED"
		}
		if err := appendEvent(ctx, tx, "RIGHTS_DECLARATION", d.ID, eventType, map[string]any{"rightsDeclarationId": d.ID, "outcome": outcome, "verificationId": verification.ID}); err != nil {
			return err
		}
		if err := appendRightsCost(ctx, tx, d.WorkspaceID, *verification.ActivityID, "RIGHTS_DECLARATION_VERIFICATION", "rights_declaration_verification_id", verification.ID); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{WorkspaceID: &d.WorkspaceID, ActorType: actorType(cmd.ActorID), ActorID: cmd.ActorID, Action: action, ObjectType: "RIGHTS_DECLARATION_VERIFICATION", ObjectID: verification.ID, AfterState: map[string]any{"declarationId": d.ID, "outcome": outcome}, TraceID: cmd.TraceID})
	})
	if errors.Is(err, infrastructure.ErrVerificationIdempotentReplay) {
		existing, findErr := s.repo.GetDeclarationVerificationByActivityID(ctx, d.ID, *verification.ActivityID)
		if findErr != nil {
			return domain.RightsVerification{}, findErr
		}
		if !sameRightsVerification(existing, verification) {
			return domain.RightsVerification{}, domain.ErrDeclarationTerminal
		}
		return existing, nil
	}
	return verification, err
}

func sameRightsVerification(existing, requested domain.RightsVerification) bool {
	if existing.DeclarationID != requested.DeclarationID || existing.Outcome != requested.Outcome || existing.Reason != requested.Reason {
		return false
	}
	if (existing.EvidenceID == nil) != (requested.EvidenceID == nil) || (existing.ActorID == nil) != (requested.ActorID == nil) {
		return false
	}
	if existing.EvidenceID != nil && *existing.EvidenceID != *requested.EvidenceID {
		return false
	}
	return existing.ActorID == nil || *existing.ActorID == *requested.ActorID
}

func (s *Service) DisposeRightsDeclaration(ctx context.Context, cmd DisposeRightsDeclarationCommand) (domain.RightsDisposition, error) {
	d, err := s.repo.GetRightsDeclaration(ctx, cmd.DeclarationID)
	if err != nil {
		return domain.RightsDisposition{}, err
	}
	kind := strings.ToUpper(strings.TrimSpace(cmd.Disposition))
	if kind != domain.DispositionInvalidated && kind != domain.DispositionSuperseded {
		return domain.RightsDisposition{}, domain.ErrRightsDisposition
	}
	effectiveAtProvided := !cmd.EffectiveAt.IsZero()
	if cmd.EffectiveAt.IsZero() {
		cmd.EffectiveAt = time.Now().UTC()
	}
	if strings.TrimSpace(cmd.Reason) == "" {
		return domain.RightsDisposition{}, domain.ErrRightsDisposition
	}
	disposition := domain.RightsDisposition{ID: uuid.New(), DeclarationID: d.ID, Disposition: kind, EffectiveAt: cmd.EffectiveAt.UTC(), Reason: strings.TrimSpace(cmd.Reason), SupersededBy: cmd.SupersededBy, EvidenceID: cmd.EvidenceID, ActivityID: cmd.ActivityID, ActorID: cmd.ActorID}
	if disposition.ActivityID == nil {
		id := uuid.New()
		disposition.ActivityID = &id
	}
	if kind == domain.DispositionSuperseded && cmd.SupersededBy == nil {
		return domain.RightsDisposition{}, domain.ErrRightsDisposition
	}
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := deliveryfence.Advance(ctx, tx, d.WorkspaceID); err != nil {
			return err
		}
		var verified string
		if err := tx.QueryRow(ctx, `SELECT outcome FROM rights_declaration_verification WHERE declaration_id=$1`, d.ID).Scan(&verified); err != nil {
			return domain.ErrDeclarationNotVerified
		}
		if verified != "VERIFIED" {
			return domain.ErrDeclarationNotVerified
		}
		if disposition.EvidenceID != nil {
			var evidenceWorkspace uuid.UUID
			if err := tx.QueryRow(ctx, `SELECT workspace_id FROM evidence WHERE id=$1`, *disposition.EvidenceID).Scan(&evidenceWorkspace); err != nil || evidenceWorkspace != d.WorkspaceID {
				return domain.ErrRightsDisposition
			}
		}
		if kind == domain.DispositionSuperseded {
			if disposition.SupersededBy == nil || *disposition.SupersededBy == d.ID {
				return domain.ErrRightsDisposition
			}
			var replacementWorkspaceID, replacementResourceID uuid.UUID
			if err := tx.QueryRow(ctx, `SELECT workspace_id,data_resource_id FROM rights_declaration WHERE id=$1 FOR SHARE`, *disposition.SupersededBy).Scan(&replacementWorkspaceID, &replacementResourceID); err != nil {
				return domain.ErrRightsDisposition
			}
			if replacementWorkspaceID != d.WorkspaceID || replacementResourceID != d.DataResourceID {
				return domain.ErrRightsDisposition
			}
		}
		if err := s.repo.InsertDeclarationDisposition(ctx, tx, disposition); err != nil {
			return err
		}
		eventType, action := "RightsDeclarationInvalidated", "RIGHTS_DECLARATION_INVALIDATED"
		if kind == domain.DispositionSuperseded {
			eventType = "RightsDeclarationSuperseded"
			action = "RIGHTS_DECLARATION_SUPERSEDED"
		}
		if err := appendEvent(ctx, tx, "RIGHTS_DECLARATION", d.ID, eventType, map[string]any{"rightsDeclarationId": d.ID, "dispositionId": disposition.ID, "effectiveAt": disposition.EffectiveAt}); err != nil {
			return err
		}
		if err := appendRightsCost(ctx, tx, d.WorkspaceID, *disposition.ActivityID, "RIGHTS_DECLARATION_DISPOSITION", "rights_declaration_disposition_id", disposition.ID); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{WorkspaceID: &d.WorkspaceID, ActorType: actorType(cmd.ActorID), ActorID: cmd.ActorID, Action: action, ObjectType: "RIGHTS_DECLARATION_DISPOSITION", ObjectID: disposition.ID, AfterState: map[string]any{"declarationId": d.ID, "disposition": kind}, TraceID: cmd.TraceID})
	})
	if errors.Is(err, infrastructure.ErrDeclarationDispositionIdempotentReplay) {
		existing, findErr := s.repo.GetDeclarationDispositionByActivityID(ctx, d.ID, kind, *disposition.ActivityID)
		if findErr != nil {
			return domain.RightsDisposition{}, findErr
		}
		if !sameRightsDisposition(existing, disposition, effectiveAtProvided) {
			return domain.RightsDisposition{}, domain.ErrRightsDisposition
		}
		return existing, nil
	}
	return disposition, err
}

func sameRightsDisposition(existing, requested domain.RightsDisposition, compareEffectiveAt bool) bool {
	if existing.DeclarationID != requested.DeclarationID || existing.Disposition != requested.Disposition || (compareEffectiveAt && !existing.EffectiveAt.Equal(requested.EffectiveAt)) || existing.Reason != requested.Reason {
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

func (s *Service) BindAuthorizationProvenance(ctx context.Context, cmd BindAuthorizationProvenanceCommand) (domain.AuthorizationProvenanceBinding, error) {
	if cmd.AsOf.IsZero() {
		cmd.AsOf = time.Now().UTC()
	}
	if cmd.ActivityID == nil {
		id := uuid.New()
		cmd.ActivityID = &id
	}
	binding := domain.AuthorizationProvenanceBinding{ID: uuid.New(), WorkspaceID: cmd.WorkspaceID, AuthorizationID: cmd.AuthorizationID, DataResourceID: cmd.DataResourceID, DeclarationID: cmd.DeclarationID, GrantorRef: strings.TrimSpace(cmd.GrantorRef), AuthorityMode: strings.ToUpper(strings.TrimSpace(cmd.AuthorityMode)), DelegationChainID: cmd.DelegationChainID, DelegationChainHash: strings.TrimSpace(cmd.DelegationChainHash), CreatedAt: time.Now().UTC(), CreatedBy: cmd.ActorID, ActivityID: cmd.ActivityID}
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.repo.InsertBinding(ctx, tx, binding, cmd.AsOf); err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, "AUTHORIZATION_PROVENANCE_BINDING", binding.ID, "AuthorizationProvenanceBound", map[string]any{"bindingId": binding.ID, "authorizationId": binding.AuthorizationID, "rightsDeclarationId": binding.DeclarationID}); err != nil {
			return err
		}
		if err := appendRightsCost(ctx, tx, binding.WorkspaceID, *cmd.ActivityID, "AUTHORIZATION_PROVENANCE_BINDING", "authorization_provenance_binding_id", binding.ID); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{WorkspaceID: &binding.WorkspaceID, ActorType: actorType(cmd.ActorID), ActorID: cmd.ActorID, Action: "AUTHORIZATION_PROVENANCE_BOUND", ObjectType: "AUTHORIZATION_PROVENANCE_BINDING", ObjectID: binding.ID, AfterState: map[string]any{"authorizationId": binding.AuthorizationID, "declarationId": binding.DeclarationID}, TraceID: cmd.TraceID})
	})
	if errors.Is(err, infrastructure.ErrBindingIdempotentReplay) {
		existing, findErr := s.repo.GetBindingByActivityID(ctx, binding.WorkspaceID, *cmd.ActivityID)
		if findErr != nil {
			return domain.AuthorizationProvenanceBinding{}, findErr
		}
		if !sameBindingRequest(existing, binding) {
			return domain.AuthorizationProvenanceBinding{}, domain.ErrInvalidBinding
		}
		return existing, nil
	}
	return binding, err
}

func sameBindingRequest(existing, requested domain.AuthorizationProvenanceBinding) bool {
	if existing.WorkspaceID != requested.WorkspaceID || existing.AuthorizationID != requested.AuthorizationID || existing.DataResourceID != requested.DataResourceID || existing.DeclarationID != requested.DeclarationID || existing.GrantorRef != requested.GrantorRef || existing.AuthorityMode != requested.AuthorityMode || existing.DelegationChainHash != requested.DelegationChainHash {
		return false
	}
	if (existing.DelegationChainID == nil) != (requested.DelegationChainID == nil) {
		return false
	}
	return existing.DelegationChainID == nil || *existing.DelegationChainID == *requested.DelegationChainID
}

func (s *Service) CheckCurrentEntitlement(ctx context.Context, cmd CheckCurrentEntitlementCommand) (domain.EntitlementDecision, error) {
	return s.repo.CheckCurrentEntitlement(ctx, cmd.EntitlementRequest)
}

type effectiveRightsProvenanceDecision struct {
	declarationID uuid.UUID
	bindingID     *uuid.UUID
}

func (s *Service) resolveEffectiveRightsProvenanceTx(ctx context.Context, tx pgx.Tx, workspaceID, resourceID uuid.UUID, consumer, purpose, action string, asOf time.Time) (effectiveRightsProvenanceDecision, error) {
	scope, err := domain.NewNormalizedScope("ALL_RESOURCE", resourceID.String())
	if err != nil {
		return effectiveRightsProvenanceDecision{}, err
	}
	declarationID, err := s.repo.CurrentDirectDeclarationTx(ctx, tx, workspaceID, resourceID, consumer, purpose, action, scope, asOf)
	if err == nil {
		return effectiveRightsProvenanceDecision{declarationID: declarationID}, nil
	}
	if !errors.Is(err, domain.ErrDeclarationNotVerified) {
		return effectiveRightsProvenanceDecision{}, err
	}
	decision, err := s.repo.CheckCurrentEntitlementTx(ctx, tx, domain.EntitlementRequest{
		WorkspaceID:     workspaceID,
		AuthorizationID: uuid.Nil,
		DataResourceID:  resourceID,
		ConsumerRef:     consumer,
		Purpose:         purpose,
		Action:          action,
		Scope:           scope,
		AsOf:            asOf,
		Path:            domain.EntitlementDownstream,
	})
	if err != nil {
		return effectiveRightsProvenanceDecision{}, err
	}
	if decision.Decision != domain.DecisionAllowed || decision.DeclarationID == nil || decision.BindingID == nil {
		return effectiveRightsProvenanceDecision{}, domain.ErrDeclarationNotVerified
	}
	return effectiveRightsProvenanceDecision{declarationID: *decision.DeclarationID, bindingID: decision.BindingID}, nil
}

func (s *Service) ComputeEffectiveRights(ctx context.Context, cmd ComputeEffectiveRightsCommand) (domain.EffectiveRightsSnapshot, error) {
	requestedAsOf := cmd.AsOf
	if cmd.AsOf.IsZero() {
		cmd.AsOf = time.Now().UTC()
	}
	if cmd.ActivityID != nil {
		replayID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("effective-rights-compute:"+cmd.WorkspaceID.String()+":"+cmd.ActivityID.String()))
		existing, replayErr := s.repo.GetEffectiveRights(ctx, replayID)
		if replayErr == nil {
			if !sameEffectiveRightsRequest(existing, cmd, requestedAsOf) {
				return domain.EffectiveRightsSnapshot{}, domain.ErrEffectiveRights
			}
			storedEvidenceID, evidenceErr := s.repo.GetEffectiveRightsSupportingEvidence(ctx, existing.ID)
			if evidenceErr != nil {
				return domain.EffectiveRightsSnapshot{}, evidenceErr
			}
			if !sameOptionalUUID(storedEvidenceID, cmd.EvidenceID) {
				return domain.EffectiveRightsSnapshot{}, domain.ErrEffectiveRights
			}
			return existing, nil
		}
		if !errors.Is(replayErr, pgx.ErrNoRows) {
			return domain.EffectiveRightsSnapshot{}, replayErr
		}
	}
	workspaceID, err := s.repo.DatasetVersionWorkspace(ctx, cmd.TargetDatasetVersionID)
	if err != nil {
		return domain.EffectiveRightsSnapshot{}, err
	}
	if cmd.WorkspaceID != workspaceID {
		return domain.EffectiveRightsSnapshot{}, domain.ErrWorkspaceMismatch
	}
	inputs, err := s.repo.RequiredLineageInputs(ctx, cmd.TargetDatasetVersionID)
	if err != nil {
		return domain.EffectiveRightsSnapshot{}, err
	}
	if len(inputs) == 0 {
		return domain.EffectiveRightsSnapshot{}, domain.ErrEffectiveRights
	}
	var snapshot domain.EffectiveRightsSnapshot
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := deliveryfence.Lock(ctx, tx, cmd.WorkspaceID); err != nil {
			return err
		}
		currentInputs, err := s.repo.RequiredLineageInputsTx(ctx, tx, cmd.TargetDatasetVersionID)
		if err != nil {
			return err
		}
		if !sameLineageInputs(inputs, currentInputs) {
			return fmt.Errorf("%w: lineage changed during effective-rights finalization", domain.ErrEffectiveRights)
		}
		var buildErr error
		snapshot, buildErr = s.buildEffectiveRightsSnapshotTx(ctx, tx, cmd, currentInputs)
		if buildErr != nil {
			return buildErr
		}
		if err := s.repo.InsertEffectiveRightsHeader(ctx, tx, snapshot); err != nil {
			return err
		}
		for _, input := range snapshot.Inputs {
			if err := s.repo.InsertEffectiveRightsInput(ctx, tx, input, snapshot.ID); err != nil {
				return err
			}
		}
		for _, action := range snapshot.Actions {
			if err := s.repo.InsertEffectiveRightsAction(ctx, tx, action, snapshot.ID); err != nil {
				return err
			}
		}
		if err := s.repo.FinalizeEffectiveRights(ctx, tx, snapshot); err != nil {
			return err
		}
		if cmd.EvidenceID != nil {
			var evidenceWorkspace uuid.UUID
			if err := tx.QueryRow(ctx, `SELECT workspace_id FROM evidence WHERE id=$1`, *cmd.EvidenceID).Scan(&evidenceWorkspace); err != nil || evidenceWorkspace != snapshot.WorkspaceID {
				return domain.ErrEffectiveRights
			}
		}
		decisionEvidenceID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("effective-rights-evidence:"+snapshot.ID.String()))
		metadata, _ := json.Marshal(map[string]any{"targetDatasetVersionId": snapshot.TargetDatasetVersionID, "requiredInputHash": snapshot.RequiredInputHash, "calculationRuleHash": snapshot.CalculationRuleHash})
		if _, err := tx.Exec(ctx, `
			INSERT INTO evidence(id,workspace_id,evidence_type,title,source_type,source_id,hash_algorithm,hash_value,metadata,created_at)
			VALUES ($1,$2,'EFFECTIVE_RIGHTS_SNAPSHOT','Effective rights finalization','EFFECTIVE_RIGHTS_SNAPSHOT',$3,'SHA256',$4,$5,now())
		`, decisionEvidenceID, snapshot.WorkspaceID, snapshot.ID, snapshot.RootHash, string(metadata)); err != nil {
			return err
		}
		items := []evidence.SnapshotItem{{EvidenceID: decisionEvidenceID, Category: "EFFECTIVE_RIGHTS_DECISION"}}
		if cmd.EvidenceID != nil {
			items = append(items, evidence.SnapshotItem{EvidenceID: *cmd.EvidenceID, Category: "SUPPORTING_EVIDENCE"})
		}
		evidenceSnapshot, err := evidence.CreateSnapshot(ctx, tx, snapshot.WorkspaceID, "EFFECTIVE_RIGHTS_SNAPSHOT", snapshot.ID, map[string]any{"effectiveRightsSnapshotId": snapshot.ID, "rootHash": snapshot.RootHash, "requiredInputHash": snapshot.RequiredInputHash}, items, cmd.ActorID)
		if err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, "EFFECTIVE_RIGHTS_SNAPSHOT", snapshot.ID, "EffectiveRightsFinalized", map[string]any{"effectiveRightsSnapshotId": snapshot.ID, "targetDatasetVersionId": snapshot.TargetDatasetVersionID, "rootHash": snapshot.RootHash, "evidenceSnapshotId": evidenceSnapshot.ID}); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{WorkspaceID: &snapshot.WorkspaceID, ActorType: actorType(cmd.ActorID), ActorID: cmd.ActorID, Action: "EFFECTIVE_RIGHTS_FINALIZED", ObjectType: "EFFECTIVE_RIGHTS_SNAPSHOT", ObjectID: snapshot.ID, AfterState: map[string]any{"targetDatasetVersionId": snapshot.TargetDatasetVersionID, "rootHash": snapshot.RootHash}, TraceID: cmd.TraceID})
	})
	if errors.Is(err, infrastructure.ErrEffectiveRightsIdempotentReplay) {
		existing, findErr := s.repo.GetEffectiveRights(ctx, snapshot.ID)
		if findErr != nil {
			return domain.EffectiveRightsSnapshot{}, findErr
		}
		if !sameEffectiveRightsRequest(existing, cmd, requestedAsOf) {
			return domain.EffectiveRightsSnapshot{}, domain.ErrEffectiveRights
		}
		return existing, nil
	}
	return snapshot, err
}

func sameOptionalUUID(left, right *uuid.UUID) bool {
	if (left == nil) != (right == nil) {
		return false
	}
	return left == nil || *left == *right
}

func sameEffectiveRightsRequest(existing domain.EffectiveRightsSnapshot, requested ComputeEffectiveRightsCommand, requestedAsOf time.Time) bool {
	if existing.WorkspaceID != requested.WorkspaceID || existing.TargetDatasetVersionID != requested.TargetDatasetVersionID || existing.ConsumerRef != strings.TrimSpace(requested.ConsumerRef) || existing.Purpose != strings.TrimSpace(requested.Purpose) {
		return false
	}
	if requested.AsOfProvided && !existing.CalculationAsOf.Equal(requestedAsOf.UTC()) {
		return false
	}
	if (existing.CreatedBy == nil) != (requested.ActorID == nil) {
		return false
	}
	return existing.CreatedBy == nil || *existing.CreatedBy == *requested.ActorID
}

func (s *Service) buildEffectiveRightsSnapshotTx(ctx context.Context, tx pgx.Tx, cmd ComputeEffectiveRightsCommand, inputs []infrastructure.LineageInput) (domain.EffectiveRightsSnapshot, error) {
	snapshot := domain.EffectiveRightsSnapshot{ID: uuid.New(), WorkspaceID: cmd.WorkspaceID, TargetDatasetVersionID: cmd.TargetDatasetVersionID, CalculationAsOf: cmd.AsOf.UTC(), ConsumerRef: strings.TrimSpace(cmd.ConsumerRef), Purpose: strings.TrimSpace(cmd.Purpose), CalculationRuleVersion: "intersection-v1", CalculationRuleHash: "rights-intersection-v1", CreatedAt: time.Now().UTC(), CreatedBy: cmd.ActorID}
	if cmd.ActivityID != nil {
		snapshot.ID = uuid.NewSHA1(uuid.NameSpaceURL, []byte("effective-rights-compute:"+cmd.WorkspaceID.String()+":"+cmd.ActivityID.String()))
	}
	provenanceByInput := make([]map[string]effectiveRightsProvenanceDecision, len(inputs))
	for idx, lineage := range inputs {
		if !lineage.ResourceMapped || lineage.DataResourceID == uuid.Nil {
			return domain.EffectiveRightsSnapshot{}, fmt.Errorf("%w: lineage input %s has no mapped data resource", domain.ErrEffectiveRights, lineage.DatasetVersionID)
		}
		provenanceByInput[idx] = make(map[string]effectiveRightsProvenanceDecision)
		for _, action := range domain.SupportedRightsActions {
			provenance, err := s.resolveEffectiveRightsProvenanceTx(ctx, tx, cmd.WorkspaceID, lineage.DataResourceID, cmd.ConsumerRef, cmd.Purpose, action, cmd.AsOf)
			if err == nil {
				provenanceByInput[idx][action] = provenance
			} else if !errors.Is(err, domain.ErrDeclarationNotVerified) {
				return domain.EffectiveRightsSnapshot{}, fmt.Errorf("resolve current rights provenance for input %s action %s: %w", lineage.DatasetVersionID, action, err)
			}
		}
		if len(provenanceByInput[idx]) == 0 {
			return domain.EffectiveRightsSnapshot{}, fmt.Errorf("%w: missing current rights provenance for input %s", domain.ErrEffectiveRights, lineage.DatasetVersionID)
		}
		selected := provenanceByInput[idx][domain.SupportedRightsActions[0]]
		if selected.declarationID == uuid.Nil {
			for _, action := range domain.SupportedRightsActions {
				if candidate, ok := provenanceByInput[idx][action]; ok {
					selected = candidate
					break
				}
			}
		}
		declarationID := selected.declarationID
		snapshot.Inputs = append(snapshot.Inputs, domain.EffectiveRightsInput{ID: uuid.New(), InputDatasetVersionID: lineage.DatasetVersionID, DataResourceID: lineage.DataResourceID, DeclarationID: &declarationID, BindingID: selected.bindingID, InputHash: infrastructure.HashEffectiveProvenance(declarationID, selected.bindingID)})
	}
	snapshot.RequiredInputHash = infrastructure.HashEffectiveInputs(snapshot.Inputs)
	for _, action := range domain.SupportedRightsActions {
		result := domain.EffectiveRightsAction{ID: uuid.New(), Action: action, Decision: domain.DecisionAllowed, Reason: "all required lineage inputs currently allow the action"}
		for idx := range inputs {
			provenance, ok := provenanceByInput[idx][action]
			if !ok {
				result.Decision = domain.DecisionNotAllowed
				result.Reason = "required input does not currently allow action"
				result.BlockingInputID = &snapshot.Inputs[idx].ID
				break
			}
			result.Provenance = append(result.Provenance, domain.EffectiveRightsProvenance{InputID: snapshot.Inputs[idx].ID, DeclarationID: provenance.declarationID, BindingID: provenance.bindingID})
		}
		snapshot.Actions = append(snapshot.Actions, result)
	}
	snapshot.RootHash = infrastructure.EffectiveRightsRootHash(snapshot)
	return snapshot, nil
}

func sameLineageInputs(left, right []infrastructure.LineageInput) bool {
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

func appendRightsCost(ctx context.Context, tx pgx.Tx, workspaceID, activityID uuid.UUID, costType, column string, subjectID uuid.UUID) error {
	metadata, _ := json.Marshal(map[string]any{"activity": "rights-provenance"})
	var eventID uuid.UUID
	err := tx.QueryRow(ctx, `INSERT INTO cost_event(id,workspace_id,execution_id,activity_id,cost_type,quantity,unit,pricing_mode,metadata,occurred_at) VALUES ($1,$2,NULL,$3,$4,1,'operation','ACTUAL',$5,now()) ON CONFLICT (workspace_id,activity_id,cost_type) WHERE activity_id IS NOT NULL DO NOTHING RETURNING id`, uuid.New(), workspaceID, activityID, costType, metadata).Scan(&eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		if err = tx.QueryRow(ctx, `SELECT id FROM cost_event WHERE workspace_id=$1 AND activity_id=$2 AND cost_type=$3`, workspaceID, activityID, costType).Scan(&eventID); err != nil {
			return err
		}
	}
	if err != nil {
		return err
	}
	allowed := map[string]bool{"rights_declaration_verification_id": true, "rights_declaration_disposition_id": true, "authorization_provenance_binding_id": true, "authorization_provenance_binding_disposition_id": true, "effective_rights_snapshot_id": true}
	if !allowed[column] {
		return fmt.Errorf("unsupported rights cost subject %s", column)
	}
	_, err = tx.Exec(ctx, `INSERT INTO cost_allocation(id,cost_event_id,`+column+`) VALUES ($1,$2,$3) ON CONFLICT (cost_event_id) DO NOTHING`, uuid.New(), eventID, subjectID)
	return err
}
