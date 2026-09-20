package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

const issueCommandType = "DELIVERY.ISSUE_CREDENTIAL"

var (
	ErrUnknownProviderOutcome = errors.New("provider outcome is unknown")
	ErrCapabilityNotFound     = errors.New("provider capability not found")
	ErrCredentialReplay       = errors.New("credential replay requires the separate fresh-authorization replay path")
)

type IssueCredentialCommand struct {
	WorkspaceID          uuid.UUID
	DatasetVersionID     uuid.UUID
	CertificationRef     *uuid.UUID
	ProviderName         string
	PrincipalRef         string
	EffectiveConsumerRef string
	DelegationRef        string
	Purpose              string
	Action               string
	ScopeRef             string
	DeliveryChannel      string
	DeliveryMode         string
	RequestedExpiresAt   time.Time
	IdempotencyKey       string
	ActorID              *uuid.UUID
	TraceID              string
}

type Result struct {
	Operation  domain.Operation
	Capability *domain.Capability
}

type Gate interface {
	// Evaluate runs against the transaction that holds the delivery
	// authorization fence. Implementations own CurrentDeliveryGate and the
	// trusted principal -> effective consumer boundary.
	Evaluate(context.Context, pgx.Tx, domain.GateRequest) (domain.GateEvaluation, error)
}

type CredentialProvider interface {
	Issue(context.Context, ProviderRequest) (domain.Capability, error)
	Recover(context.Context, string) (domain.Capability, error)
	Revoke(context.Context, string) error
}

type ProviderRequest struct {
	ProviderRequestKey string
	DatasetVersionID   uuid.UUID
	ConsumerRef        string
	Purpose            string
	Action             string
	ScopeRef           string
	DeliveryChannel    string
	DeliveryMode       string
	ExpiresAt          time.Time
}

type Service struct {
	tx       *transaction.Manager
	repo     *infrastructure.PostgresRepository
	gate     Gate
	provider CredentialProvider
	// AfterProviderCall is a test-only fault window. Returning an error here
	// models a process crash after provider success and before Core observes it.
	AfterProviderCall func()
}

func NewService(tx *transaction.Manager, repo *infrastructure.PostgresRepository, gate Gate, provider CredentialProvider) *Service {
	return &Service{tx: tx, repo: repo, gate: gate, provider: provider}
}

func (s *Service) IssueCredential(ctx context.Context, cmd IssueCredentialCommand) (Result, error) {
	if s == nil || s.tx == nil || s.repo == nil || s.gate == nil || s.provider == nil {
		return Result{}, fmt.Errorf("delivery service is not configured")
	}
	if err := validateCommand(cmd); err != nil {
		return Result{}, err
	}
	fingerprint, err := commandFingerprint(cmd)
	if err != nil {
		return Result{}, err
	}

	var operation domain.Operation
	created := false
	terminalContainmentPending := false
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		candidateID := uuid.New()
		inserted, err := s.repo.TryInsertIdempotency(ctx, tx, cmd.WorkspaceID, cmd.IdempotencyKey, fingerprint, candidateID)
		if err != nil {
			return err
		}
		if !inserted {
			record, found, err := s.repo.FindIdempotency(ctx, tx, cmd.WorkspaceID, cmd.IdempotencyKey)
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("delivery idempotency record disappeared after conflict")
			}
			if record.RequestFingerprint != fingerprint {
				return errors.New("delivery idempotency key conflicts with another request")
			}
			operation, err = s.repo.GetOperation(ctx, tx, record.ObjectID, false)
			if err == nil && operation.IsTerminal() {
				status, found, containmentErr := s.repo.GetContainmentStatus(ctx, tx, operation.ID)
				if containmentErr != nil {
					return containmentErr
				}
				terminalContainmentPending = found && status == domain.ContainmentPending
			}
			return err
		}
		candidate, err := domain.NewOperation(
			cmd.WorkspaceID, cmd.DatasetVersionID, cmd.IdempotencyKey, cmd.ProviderName,
			cmd.PrincipalRef, cmd.EffectiveConsumerRef, cmd.DelegationRef, cmd.Purpose, cmd.Action,
			cmd.ScopeRef, cmd.DeliveryChannel, cmd.DeliveryMode, cmd.RequestedExpiresAt, cmd.CertificationRef,
		)
		if err != nil {
			return err
		}
		candidate.ID = candidateID

		if err := s.repo.InsertOperation(ctx, tx, candidate); err != nil {
			return err
		}
		revision, err := s.repo.LockFence(ctx, tx, candidate.WorkspaceID)
		if err != nil {
			return err
		}
		evaluation, err := s.evaluate(ctx, tx, candidate, domain.GateInitial, revision, "initial/"+candidate.ID.String())
		if err != nil {
			return err
		}
		if err := s.repo.InsertGateEvaluation(ctx, tx, candidate.ID, evaluation, candidate.CreatedAt); err != nil {
			return err
		}
		if err := appendGateFacts(ctx, tx, candidate, evaluation, cmd.ActorID, cmd.TraceID); err != nil {
			return err
		}
		candidate.DependencyRevision = evaluation.DependencyRevision
		candidate.FreshCapExpiresAt = evaluation.FreshCapExpiresAt
		candidate.CurrentGateDecision = evaluation.Decision()
		if evaluation.Allowed {
			if err := candidate.Transition(domain.StatusIssuancePending); err != nil {
				return err
			}
			if err := s.repo.InsertTransition(ctx, tx, candidate.ID, "initial/pending", domain.StatusPrepared, candidate.Status, &evaluation.ID, nil, "provider side effect is pending", candidate.UpdatedAt); err != nil {
				return err
			}
			if err := s.repo.UpdateProjection(ctx, tx, candidate); err != nil {
				return err
			}
			if err := appendTransitionFacts(ctx, tx, candidate, domain.StatusPrepared, "DatasetDeliveryIssuancePending", "provider side effect is pending", cmd.ActorID, cmd.TraceID); err != nil {
				return err
			}
		} else {
			if err := candidate.Transition(domain.StatusBlocked); err != nil {
				return err
			}
			candidate.TerminalReason = firstBlocker(evaluation.Blockers)
			if err := s.repo.InsertTransition(ctx, tx, candidate.ID, "initial/blocked", domain.StatusPrepared, candidate.Status, &evaluation.ID, nil, candidate.TerminalReason, candidate.UpdatedAt); err != nil {
				return err
			}
			if err := s.repo.UpdateProjection(ctx, tx, candidate); err != nil {
				return err
			}
			if err := appendTransitionFacts(ctx, tx, candidate, domain.StatusPrepared, "DatasetDeliveryBlocked", candidate.TerminalReason, cmd.ActorID, cmd.TraceID); err != nil {
				return err
			}
		}
		operation = candidate
		created = true
		return nil
	})
	if err != nil {
		return Result{}, err
	}

	if operation.Status == domain.StatusIssued {
		return s.replayIssued(ctx, operation, cmd)
	}
	if operation.Status == domain.StatusBlocked || operation.Status == domain.StatusFailed {
		if terminalContainmentPending {
			return s.reconcileTerminalContainment(ctx, operation, cmd)
		}
		return Result{Operation: operation}, nil
	}
	if operation.Status == domain.StatusContainmentPending {
		return s.reconcileContainment(ctx, operation, cmd)
	}
	return s.processPending(ctx, operation, cmd, created)
}

type replayPreparation struct {
	operation  domain.Operation
	evaluation domain.GateEvaluation
	replayID   uuid.UUID
	attemptID  uuid.UUID
	revoke     bool
}

func (s *Service) replayIssued(ctx context.Context, operation domain.Operation, cmd IssueCredentialCommand) (Result, error) {
	prep, err := s.prepareReplay(ctx, operation.ID, cmd)
	if err != nil {
		return Result{}, err
	}
	if prep.revoke {
		return s.executeReplayRevoke(ctx, prep, "fresh replay gate blocked", cmd)
	}

	capability, err := s.provider.Recover(ctx, prep.operation.ProviderRequestKey)
	if err == nil && s.AfterProviderCall != nil {
		s.AfterProviderCall()
	}
	if errors.Is(err, ErrCapabilityNotFound) {
		if recordErr := s.recordObservation(ctx, prep.operation.ID, prep.attemptID, domain.ObservationReconciliation, domain.OutcomeNotFound, domain.Capability{}, "provider reports no active capability"); recordErr != nil {
			return Result{}, recordErr
		}
		result, recordErr := s.appendReplayDecision(ctx, prep, "BLOCKED", domain.Capability{}, "provider reports no active capability", cmd)
		if recordErr != nil {
			return Result{}, recordErr
		}
		return result, ErrCredentialReplay
	}
	if err != nil {
		outcome := domain.OutcomeFailed
		kind := domain.ObservationReconciliation
		if errors.Is(err, ErrUnknownProviderOutcome) {
			outcome = domain.OutcomeUnknown
			kind = domain.ObservationTimeout
		}
		reason := providerFailureReason(err)
		if recordErr := s.recordObservation(ctx, prep.operation.ID, prep.attemptID, kind, outcome, domain.Capability{}, reason); recordErr != nil {
			return Result{}, recordErr
		}
		result, recordErr := s.appendReplayDecision(ctx, prep, "CONTAINMENT_PENDING", domain.Capability{}, reason, cmd)
		if recordErr != nil {
			return Result{}, recordErr
		}
		return result, ErrCredentialReplay
	}
	if err := s.recordObservation(ctx, prep.operation.ID, prep.attemptID, domain.ObservationReconciliation, domain.OutcomeSuccess, capability, "provider replay recovered capability"); err != nil {
		return Result{}, err
	}
	return s.finalizeReplay(ctx, prep, capability, cmd)
}

func (s *Service) prepareReplay(ctx context.Context, operationID uuid.UUID, cmd IssueCredentialCommand) (replayPreparation, error) {
	prep := replayPreparation{replayID: uuid.New()}
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		prep.operation, err = s.repo.GetOperation(ctx, tx, operationID, true)
		if err != nil {
			return err
		}
		if prep.operation.Status != domain.StatusIssued {
			return ErrCredentialReplay
		}
		revision, err := s.repo.LockFence(ctx, tx, prep.operation.WorkspaceID)
		if err != nil {
			return err
		}
		prep.evaluation, err = s.evaluate(ctx, tx, prep.operation, domain.GateReplay, revision, "replay/"+prep.replayID.String())
		if err != nil {
			return err
		}
		if err := s.repo.InsertGateEvaluation(ctx, tx, prep.operation.ID, prep.evaluation, time.Now().UTC()); err != nil {
			return err
		}
		if err := appendGateFacts(ctx, tx, prep.operation, prep.evaluation, cmd.ActorID, cmd.TraceID); err != nil {
			return err
		}
		prep.operation.DependencyRevision = prep.evaluation.DependencyRevision
		prep.operation.FreshCapExpiresAt = prep.evaluation.FreshCapExpiresAt
		prep.operation.CurrentGateDecision = prep.evaluation.Decision()
		kind := domain.InvocationVerify
		if !prep.evaluation.Allowed {
			kind = domain.InvocationRevoke
			prep.revoke = true
		}
		prep.attemptID, err = s.repo.InsertProviderAttempt(ctx, tx, prep.operation, prep.replayID.String(), kind)
		if err != nil {
			return err
		}
		if err := s.repo.InsertProviderCost(ctx, tx, prep.operation, prep.attemptID); err != nil {
			return err
		}
		if err := appendAttemptFacts(ctx, tx, prep.operation, prep.attemptID, kind, cmd.ActorID, cmd.TraceID); err != nil {
			return err
		}
		return s.repo.UpdateProjection(ctx, tx, prep.operation)
	})
	if err != nil {
		return replayPreparation{}, err
	}
	return prep, nil
}

func (s *Service) finalizeReplay(ctx context.Context, prep replayPreparation, capability domain.Capability, cmd IssueCredentialCommand) (Result, error) {
	var operation domain.Operation
	var evaluation domain.GateEvaluation
	reason := ""
	needsContainment := false
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		operation, err = s.repo.GetOperation(ctx, tx, prep.operation.ID, true)
		if err != nil {
			return err
		}
		revision, err := s.repo.LockFence(ctx, tx, operation.WorkspaceID)
		if err != nil {
			return err
		}
		evaluation, err = s.evaluate(ctx, tx, operation, domain.GateReplay, revision, "replay/finalize/"+prep.replayID.String())
		if err != nil {
			return err
		}
		if err := s.repo.InsertGateEvaluation(ctx, tx, operation.ID, evaluation, time.Now().UTC()); err != nil {
			return err
		}
		if err := appendGateFacts(ctx, tx, operation, evaluation, cmd.ActorID, cmd.TraceID); err != nil {
			return err
		}
		operation.DependencyRevision = evaluation.DependencyRevision
		operation.FreshCapExpiresAt = evaluation.FreshCapExpiresAt
		operation.CurrentGateDecision = evaluation.Decision()
		if !evaluation.Allowed {
			needsContainment = true
			reason = firstBlocker(evaluation.Blockers)
			return s.repo.UpdateProjection(ctx, tx, operation)
		}
		if !matchesIssuedCapability(operation, capability) {
			needsContainment = true
			reason = "provider replay did not return the originally issued capability"
			return s.repo.UpdateProjection(ctx, tx, operation)
		}
		if err := capability.ValidateAgainst(operation, evaluation); err != nil {
			needsContainment = true
			reason = err.Error()
			return s.repo.UpdateProjection(ctx, tx, operation)
		}
		if err := s.repo.UpdateProjection(ctx, tx, operation); err != nil {
			return err
		}
		if err := s.repo.InsertReplayDecision(ctx, tx, operation.ID, prep.replayID, evaluation.ID, "ALLOWED", evaluation, capability, "fresh replay authorization allowed"); err != nil {
			return err
		}
		return s.appendReplayFactsTx(ctx, tx, operation, prep.replayID, "ALLOWED", evaluation, capability, "fresh replay authorization allowed", cmd)
	})
	if err != nil {
		return Result{}, err
	}
	if needsContainment {
		prep.operation = operation
		prep.evaluation = evaluation
		result, err := s.executeReplayRevoke(ctx, prep, reason, cmd)
		if err != nil {
			return result, err
		}
		return result, ErrCredentialReplay
	}
	return Result{Operation: operation, Capability: &capability}, nil
}

func (s *Service) executeReplayRevoke(ctx context.Context, prep replayPreparation, reason string, cmd IssueCredentialCommand) (Result, error) {
	if prep.revoke {
		if err := s.provider.Revoke(ctx, prep.operation.ProviderRequestKey); err != nil {
			if errors.Is(err, ErrCapabilityNotFound) {
				if recordErr := s.recordObservation(ctx, prep.operation.ID, prep.attemptID, domain.ObservationCallReturn, domain.OutcomeNotFound, domain.Capability{}, "provider reports no active capability during replay containment"); recordErr != nil {
					return Result{}, recordErr
				}
				result, recordErr := s.appendReplayDecision(ctx, prep, "BLOCKED", domain.Capability{}, reason, cmd)
				if recordErr != nil {
					return Result{}, recordErr
				}
				return result, ErrCredentialReplay
			}
			reasonCode := providerFailureReason(err)
			if recordErr := s.recordObservation(ctx, prep.operation.ID, prep.attemptID, domain.ObservationCallReturn, domain.OutcomeUnknown, domain.Capability{}, reasonCode); recordErr != nil {
				return Result{}, recordErr
			}
			result, recordErr := s.appendReplayDecision(ctx, prep, "CONTAINMENT_PENDING", domain.Capability{}, reason+": "+reasonCode, cmd)
			if recordErr != nil {
				return Result{}, recordErr
			}
			return result, ErrCredentialReplay
		}
		if err := s.recordObservation(ctx, prep.operation.ID, prep.attemptID, domain.ObservationCallReturn, domain.OutcomeSuccess, domain.Capability{}, "replay capability contained"); err != nil {
			return Result{}, err
		}
		result, recordErr := s.appendReplayDecision(ctx, prep, "BLOCKED", domain.Capability{}, reason, cmd)
		if recordErr != nil {
			return Result{}, recordErr
		}
		return result, ErrCredentialReplay
	}

	var err error
	prep.attemptID, err = s.prepareReplayRevoke(ctx, prep.operation, prep.replayID, cmd)
	if err != nil {
		return Result{}, err
	}
	prep.revoke = true
	return s.executeReplayRevoke(ctx, prep, reason, cmd)
}

func (s *Service) prepareReplayRevoke(ctx context.Context, operation domain.Operation, replayID uuid.UUID, cmd IssueCredentialCommand) (uuid.UUID, error) {
	var attemptID uuid.UUID
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		current, err := s.repo.GetOperation(ctx, tx, operation.ID, true)
		if err != nil {
			return err
		}
		attemptID, err = s.repo.InsertProviderAttempt(ctx, tx, current, replayID.String()+"/revoke", domain.InvocationRevoke)
		if err != nil {
			return err
		}
		if err := s.repo.InsertProviderCost(ctx, tx, current, attemptID); err != nil {
			return err
		}
		return appendAttemptFacts(ctx, tx, current, attemptID, domain.InvocationRevoke, cmd.ActorID, cmd.TraceID)
	})
	return attemptID, err
}

func (s *Service) appendReplayDecision(ctx context.Context, prep replayPreparation, decision string, capability domain.Capability, reason string, cmd IssueCredentialCommand) (Result, error) {
	var operation domain.Operation
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		operation, err = s.repo.GetOperation(ctx, tx, prep.operation.ID, true)
		if err != nil {
			return err
		}
		if err := s.repo.InsertReplayDecision(ctx, tx, operation.ID, prep.replayID, prep.evaluation.ID, decision, prep.evaluation, capability, reason); err != nil {
			return err
		}
		if err := s.repo.UpdateProjection(ctx, tx, operation); err != nil {
			return err
		}
		return s.appendReplayFactsTx(ctx, tx, operation, prep.replayID, decision, prep.evaluation, capability, reason, cmd)
	})
	if err != nil {
		return Result{}, err
	}
	return Result{Operation: operation}, nil
}

func (s *Service) appendReplayFactsTx(ctx context.Context, tx pgx.Tx, operation domain.Operation, replayID uuid.UUID, decision string, evaluation domain.GateEvaluation, capability domain.Capability, reason string, cmd IssueCredentialCommand) error {
	event, err := outbox.NewEvent("DELIVERY_OPERATION", operation.ID, "DatasetCredentialReplayDecision", map[string]any{
		"operationId": operation.ID, "replayAttemptId": replayID, "decision": decision,
		"dependencyRevision": evaluation.DependencyRevision, "capabilityRef": capability.CapabilityRef,
		"capabilityHash": capability.CapabilityHash, "reason": reason,
		"principalRef": evaluation.PrincipalRef, "effectiveConsumerRef": evaluation.EffectiveConsumerRef,
		"delegationRef": evaluation.DelegationRef,
	})
	if err != nil {
		return err
	}
	if err := outbox.Append(ctx, tx, event); err != nil {
		return err
	}
	if decision == "CONTAINMENT_PENDING" {
		pending, err := outbox.NewEvent("DELIVERY_OPERATION", operation.ID, "DatasetCredentialReplayContainmentPending", map[string]any{
			"operationId": operation.ID, "replayAttemptId": replayID, "reason": reason,
		})
		if err != nil {
			return err
		}
		if err := outbox.Append(ctx, tx, pending); err != nil {
			return err
		}
	}
	if err := audit.Append(ctx, tx, audit.Event{
		WorkspaceID: &operation.WorkspaceID, ActorType: "SYSTEM", ActorID: cmd.ActorID,
		Action: "DELIVERY_CREDENTIAL_REPLAY_" + decision, ObjectType: "DELIVERY_OPERATION", ObjectID: operation.ID,
		AfterState: map[string]any{
			"replayAttemptId": replayID, "decision": decision, "reason": reason,
			"principalRef": evaluation.PrincipalRef, "effectiveConsumerRef": evaluation.EffectiveConsumerRef,
			"delegationRef": evaluation.DelegationRef,
		}, TraceID: cmd.TraceID,
	}); err != nil {
		return err
	}
	_, err = evidence.Append(ctx, tx, evidence.Record{
		WorkspaceID:  operation.WorkspaceID,
		EvidenceType: "DELIVERY_CREDENTIAL_REPLAY",
		Title:        "Delivery credential replay decision",
		SourceType:   "DELIVERY_OPERATION",
		SourceID:     &operation.ID,
		Metadata: map[string]any{
			"replayAttemptId":      replayID,
			"decision":             decision,
			"dependencyRevision":   evaluation.DependencyRevision,
			"capabilityRef":        capability.CapabilityRef,
			"capabilityHash":       capability.CapabilityHash,
			"reason":               reason,
			"principalRef":         evaluation.PrincipalRef,
			"effectiveConsumerRef": evaluation.EffectiveConsumerRef,
			"delegationRef":        evaluation.DelegationRef,
		},
		CreatedBy: cmd.ActorID,
	}, evidence.Relation{ObjectType: "DELIVERY_OPERATION", ObjectID: operation.ID, RelationType: "CREDENTIAL_REPLAY"})
	return err
}

func (s *Service) processPending(ctx context.Context, operation domain.Operation, cmd IssueCredentialCommand, initial bool) (Result, error) {
	stage := domain.GateReconciliation
	kind := domain.InvocationReconcile
	if initial {
		stage = domain.GateProviderPrepare
		kind = domain.InvocationIssue
	}
	operation, evaluation, attemptID, proceed, err := s.prepareProviderCall(ctx, operation.ID, stage, kind, cmd)
	if err != nil {
		return Result{}, err
	}
	if !proceed {
		if !initial && operation.Status == domain.StatusContainmentPending {
			return s.reconcileContainment(ctx, operation, cmd)
		}
		return Result{Operation: operation}, nil
	}

	request := ProviderRequest{
		ProviderRequestKey: operation.ProviderRequestKey,
		DatasetVersionID:   operation.DatasetVersionID,
		ConsumerRef:        operation.EffectiveConsumerRef,
		Purpose:            operation.Purpose,
		Action:             operation.Action,
		ScopeRef:           operation.ScopeRef,
		DeliveryChannel:    operation.DeliveryChannel,
		DeliveryMode:       operation.DeliveryMode,
	}
	request.ExpiresAt = operation.RequestedExpiresAt
	if evaluation.FreshCapExpiresAt != nil {
		request.ExpiresAt = evaluation.FreshCapExpiresAt.UTC()
	}
	var capability domain.Capability
	if initial {
		capability, err = s.provider.Issue(ctx, request)
	} else {
		capability, err = s.provider.Recover(ctx, operation.ProviderRequestKey)
	}
	if err == nil && s.AfterProviderCall != nil {
		s.AfterProviderCall()
		// The hook is intentionally allowed to panic in integration tests to
		// model process loss. If it returns normally, the provider result is
		// still processed below.
	}

	if err != nil {
		outcome := domain.OutcomeFailed
		observation := domain.ObservationCallReturn
		if errors.Is(err, ErrCapabilityNotFound) && !initial {
			if recordErr := s.recordObservation(ctx, operation.ID, attemptID, domain.ObservationReconciliation, domain.OutcomeNotFound, domain.Capability{}, "provider reports no active capability"); recordErr != nil {
				return Result{}, recordErr
			}
			target := domain.StatusFailed
			if operation.Status == domain.StatusContainmentPending && !evaluation.Allowed {
				target = domain.StatusBlocked
			}
			return s.finishWithoutCapability(ctx, operation.ID, cmd, target, "provider reports no active capability", &attemptID)
		}
		if errors.Is(err, ErrUnknownProviderOutcome) {
			outcome = domain.OutcomeUnknown
			observation = domain.ObservationTimeout
		}
		reasonCode := providerFailureReason(err)
		if recordErr := s.recordObservation(ctx, operation.ID, attemptID, observation, outcome, domain.Capability{}, reasonCode); recordErr != nil {
			return Result{}, recordErr
		}
		if outcome == domain.OutcomeUnknown {
			return s.enterContainmentPending(ctx, operation.ID, reasonCode, cmd, &attemptID)
		}
		if !initial {
			return s.enterContainmentPending(ctx, operation.ID, reasonCode, cmd, &attemptID)
		}
		return s.finishWithoutCapability(ctx, operation.ID, cmd, domain.StatusFailed, reasonCode, &attemptID)
	}
	if err := s.recordObservation(ctx, operation.ID, attemptID, domain.ObservationCallReturn, domain.OutcomeSuccess, capability, "provider returned capability"); err != nil {
		return Result{}, err
	}
	if !initial && operation.Status == domain.StatusContainmentPending {
		target := domain.StatusBlocked
		if evaluation.Allowed {
			target = domain.StatusFailed
		}
		return s.containCapability(ctx, operation, target, "recovered capability requires containment", capability, cmd)
	}
	return s.finalizeCapability(ctx, operation.ID, attemptID, capability, cmd)
}

func (s *Service) prepareProviderCall(ctx context.Context, operationID uuid.UUID, stage domain.GateStage, kind domain.InvocationKind, cmd IssueCredentialCommand) (domain.Operation, domain.GateEvaluation, uuid.UUID, bool, error) {
	var operation domain.Operation
	var evaluation domain.GateEvaluation
	var attemptID uuid.UUID
	proceed := false
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		operation, err = s.repo.GetOperation(ctx, tx, operationID, true)
		if err != nil {
			return err
		}
		if operation.Status != domain.StatusIssuancePending && operation.Status != domain.StatusContainmentPending {
			return nil
		}
		revision, err := s.repo.LockFence(ctx, tx, operation.WorkspaceID)
		if err != nil {
			return err
		}
		evaluation, err = s.evaluate(ctx, tx, operation, stage, revision, string(stage)+"/"+uuid.NewString())
		if err != nil {
			return err
		}
		if err := s.repo.InsertGateEvaluation(ctx, tx, operation.ID, evaluation, time.Now().UTC()); err != nil {
			return err
		}
		if err := appendGateFacts(ctx, tx, operation, evaluation, cmd.ActorID, cmd.TraceID); err != nil {
			return err
		}
		operation.DependencyRevision = evaluation.DependencyRevision
		operation.FreshCapExpiresAt = evaluation.FreshCapExpiresAt
		operation.CurrentGateDecision = evaluation.Decision()
		if !evaluation.Allowed && kind == domain.InvocationIssue {
			before := operation.Status
			operation.TerminalReason = firstBlocker(evaluation.Blockers)
			if err := operation.Transition(domain.StatusBlocked); err != nil {
				return err
			}
			if err := s.repo.InsertTransition(ctx, tx, operation.ID, "blocked/"+evaluation.EvaluationKey, before, operation.Status, &evaluation.ID, nil, operation.TerminalReason, operation.UpdatedAt); err != nil {
				return err
			}
			if err := s.repo.UpdateProjection(ctx, tx, operation); err != nil {
				return err
			}
			return appendTransitionFacts(ctx, tx, operation, before, "DatasetDeliveryBlocked", operation.TerminalReason, cmd.ActorID, cmd.TraceID)
		}
		attemptID, err = s.repo.InsertProviderAttempt(ctx, tx, operation, uuid.NewString(), kind)
		if err != nil {
			return err
		}
		if err := s.repo.InsertProviderCost(ctx, tx, operation, attemptID); err != nil {
			return err
		}
		if err := appendAttemptFacts(ctx, tx, operation, attemptID, kind, cmd.ActorID, cmd.TraceID); err != nil {
			return err
		}
		if !evaluation.Allowed && kind == domain.InvocationReconcile && operation.Status == domain.StatusIssuancePending {
			before := operation.Status
			operation.TerminalReason = firstBlocker(evaluation.Blockers)
			if err := operation.Transition(domain.StatusContainmentPending); err != nil {
				return err
			}
			if err := s.repo.InsertTransition(ctx, tx, operation.ID, "containment/"+evaluation.EvaluationKey, before, operation.Status, &evaluation.ID, &attemptID, operation.TerminalReason, operation.UpdatedAt); err != nil {
				return err
			}
			if err := s.repo.UpdateProjection(ctx, tx, operation); err != nil {
				return err
			}
			if err := appendTransitionFacts(ctx, tx, operation, before, "DatasetDeliveryContainmentPending", operation.TerminalReason, cmd.ActorID, cmd.TraceID); err != nil {
				return err
			}
		}
		operation.DependencyRevision = evaluation.DependencyRevision
		if err := s.repo.UpdateProjection(ctx, tx, operation); err != nil {
			return err
		}
		proceed = true
		return nil
	})
	return operation, evaluation, attemptID, proceed, err
}

func (s *Service) finalizeCapability(ctx context.Context, operationID, attemptID uuid.UUID, capability domain.Capability, cmd IssueCredentialCommand) (Result, error) {
	var operation domain.Operation
	var evaluation domain.GateEvaluation
	var needsContainment bool
	var finalizationSkipped bool
	reason := ""
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		operation, err = s.repo.GetOperation(ctx, tx, operationID, true)
		if err != nil {
			return err
		}
		if operation.Status != domain.StatusIssuancePending {
			finalizationSkipped = true
			return nil
		}
		revision, err := s.repo.LockFence(ctx, tx, operation.WorkspaceID)
		if err != nil {
			return err
		}
		evaluation, err = s.evaluate(ctx, tx, operation, domain.GateTerminalFinalize, revision, "terminal/"+uuid.NewString())
		if err != nil {
			return err
		}
		if err := s.repo.InsertGateEvaluation(ctx, tx, operation.ID, evaluation, time.Now().UTC()); err != nil {
			return err
		}
		if err := appendGateFacts(ctx, tx, operation, evaluation, cmd.ActorID, cmd.TraceID); err != nil {
			return err
		}
		operation.DependencyRevision = evaluation.DependencyRevision
		operation.FreshCapExpiresAt = evaluation.FreshCapExpiresAt
		operation.CurrentGateDecision = evaluation.Decision()
		if !evaluation.Allowed {
			needsContainment = true
			reason = firstBlocker(evaluation.Blockers)
			return nil
		}
		if err := capability.ValidateAgainst(operation, evaluation); err != nil {
			needsContainment = true
			reason = err.Error()
			return nil
		}
		operation.CredentialRef = capability.CapabilityRef
		operation.CredentialHash = hashSecret(capability.Credential)
		operation.ProviderCredentialExpiresAt = &capability.ProviderCredentialExpiresAt
		if err := operation.Transition(domain.StatusIssued); err != nil {
			return err
		}
		if err := s.repo.InsertTransition(ctx, tx, operation.ID, "terminal/issued/"+attemptID.String(), domain.StatusIssuancePending, operation.Status, &evaluation.ID, &attemptID, "credential capability committed", operation.UpdatedAt); err != nil {
			return err
		}
		if err := s.repo.UpdateProjection(ctx, tx, operation); err != nil {
			return err
		}
		return appendTransitionFacts(ctx, tx, operation, domain.StatusIssuancePending, "DatasetDeliveryIssued", "credential capability committed; secret returned only after commit", cmd.ActorID, cmd.TraceID)
	})
	if err != nil {
		return Result{}, err
	}
	if finalizationSkipped {
		if operation.Status == domain.StatusIssued && matchesIssuedCapability(operation, capability) {
			return s.replayIssued(ctx, operation, cmd)
		}
		return s.containUnexpectedCapability(ctx, operation, capability, cmd)
	}
	if needsContainment {
		target := domain.StatusFailed
		if !evaluation.Allowed {
			target = domain.StatusBlocked
		}
		return s.containCapability(ctx, operation, target, reason, capability, cmd)
	}
	return Result{Operation: operation, Capability: &capability}, nil
}

func matchesIssuedCapability(operation domain.Operation, capability domain.Capability) bool {
	if operation.Status != domain.StatusIssued || operation.CredentialRef == "" || operation.CredentialHash == "" {
		return false
	}
	if capability.CapabilityRef != operation.CredentialRef {
		return false
	}
	if operation.ProviderCredentialExpiresAt == nil || capability.ProviderCredentialExpiresAt.IsZero() || capability.ProviderCredentialExpiresAt.After(operation.ProviderCredentialExpiresAt.UTC()) {
		return false
	}
	derivedHash := hashSecret(capability.Credential)
	if capability.CapabilityHash != "" && capability.CapabilityHash != derivedHash {
		return false
	}
	return derivedHash == operation.CredentialHash
}

func (s *Service) containUnexpectedCapability(ctx context.Context, operation domain.Operation, capability domain.Capability, cmd IssueCredentialCommand) (Result, error) {
	return s.revokeContainment(ctx, operation, capability, "late capability contained after terminal race", cmd)
}

func (s *Service) reconcileTerminalContainment(ctx context.Context, operation domain.Operation, cmd IssueCredentialCommand) (Result, error) {
	return s.revokeContainment(ctx, operation, domain.Capability{}, "terminal containment retry", cmd)
}

func (s *Service) revokeContainment(ctx context.Context, operation domain.Operation, capability domain.Capability, reason string, cmd IssueCredentialCommand) (Result, error) {
	attemptID, current, proceed, err := s.startContainmentAttempt(ctx, operation, capability, reason, cmd)
	if err != nil {
		return Result{}, err
	}
	if !proceed {
		return Result{Operation: current}, ErrCredentialReplay
	}
	err = s.provider.Revoke(ctx, current.ProviderRequestKey)
	if errors.Is(err, ErrCapabilityNotFound) {
		if recordErr := s.recordObservation(ctx, current.ID, attemptID, domain.ObservationCallReturn, domain.OutcomeNotFound, capability, "provider reports no active capability during containment"); recordErr != nil {
			return Result{}, recordErr
		}
		if resolveErr := s.resolveContainment(ctx, current.ID, attemptID, "provider reports no active capability during containment", cmd); resolveErr != nil {
			return Result{}, resolveErr
		}
		return Result{Operation: current}, ErrCredentialReplay
	}
	if err != nil {
		if recordErr := s.recordObservation(ctx, current.ID, attemptID, domain.ObservationCallReturn, domain.OutcomeUnknown, capability, "containment outcome is unknown: "+providerFailureReason(err)); recordErr != nil {
			return Result{}, recordErr
		}
		// The PENDING projection deliberately remains durable so the same-key
		// command can retry revoke without rewriting the terminal operation.
		return Result{Operation: current}, ErrCredentialReplay
	}
	if err := s.recordObservation(ctx, current.ID, attemptID, domain.ObservationCallReturn, domain.OutcomeSuccess, capability, reason); err != nil {
		return Result{}, err
	}
	if err := s.resolveContainment(ctx, current.ID, attemptID, reason, cmd); err != nil {
		return Result{}, err
	}
	return Result{Operation: current}, ErrCredentialReplay
}

func (s *Service) startContainmentAttempt(ctx context.Context, operation domain.Operation, capability domain.Capability, reason string, cmd IssueCredentialCommand) (uuid.UUID, domain.Operation, bool, error) {
	var attemptID uuid.UUID
	var current domain.Operation
	proceed := true
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		current, err = s.repo.GetOperation(ctx, tx, operation.ID, true)
		if err != nil {
			return err
		}
		status, found, err := s.repo.GetContainmentStatus(ctx, tx, current.ID)
		if err != nil {
			return err
		}
		if found && status == domain.ContainmentResolved && capability.Credential == "" {
			proceed = false
			return nil
		}
		attemptID, err = s.repo.InsertProviderAttempt(ctx, tx, current, "late-containment/"+uuid.NewString(), domain.InvocationRevoke)
		if err != nil {
			return err
		}
		if err := s.repo.InsertProviderCost(ctx, tx, current, attemptID); err != nil {
			return err
		}
		if err := appendAttemptFacts(ctx, tx, current, attemptID, domain.InvocationRevoke, cmd.ActorID, cmd.TraceID); err != nil {
			return err
		}
		if !found || status != domain.ContainmentPending {
			if err := s.repo.UpsertContainmentProjection(ctx, tx, current.ID, domain.ContainmentPending, &attemptID, reason); err != nil {
				return err
			}
			if err := s.repo.InsertContainmentTransition(ctx, tx, current.ID, status, domain.ContainmentPending, &attemptID, reason); err != nil {
				return err
			}
			return appendContainmentFactsTx(ctx, tx, current, attemptID, domain.ContainmentPending, reason, cmd)
		}
		return s.repo.UpsertContainmentProjection(ctx, tx, current.ID, domain.ContainmentPending, &attemptID, reason)
	})
	return attemptID, current, proceed, err
}

func (s *Service) resolveContainment(ctx context.Context, operationID, attemptID uuid.UUID, reason string, cmd IssueCredentialCommand) error {
	return s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		operation, err := s.repo.GetOperation(ctx, tx, operationID, true)
		if err != nil {
			return err
		}
		status, found, err := s.repo.GetContainmentStatus(ctx, tx, operationID)
		if err != nil {
			return err
		}
		if !found || status == domain.ContainmentResolved {
			return nil
		}
		if err := s.repo.UpsertContainmentProjection(ctx, tx, operationID, domain.ContainmentResolved, &attemptID, reason); err != nil {
			return err
		}
		if err := s.repo.InsertContainmentTransition(ctx, tx, operationID, status, domain.ContainmentResolved, &attemptID, reason); err != nil {
			return err
		}
		return appendContainmentFactsTx(ctx, tx, operation, attemptID, domain.ContainmentResolved, reason, cmd)
	})
}

func (s *Service) containCapability(ctx context.Context, operation domain.Operation, target domain.Status, reason string, capability domain.Capability, cmd IssueCredentialCommand) (Result, error) {
	// Revoke is a new physical provider attempt. It is never folded into the
	// original ISSUE attempt or its cost identity.
	var attemptID uuid.UUID
	proceed := false
	terminalRace := false
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		current, err := s.repo.GetOperation(ctx, tx, operation.ID, true)
		if err != nil {
			return err
		}
		if current.Status != domain.StatusIssuancePending && current.Status != domain.StatusContainmentPending {
			operation = current
			terminalRace = capability.Credential != ""
			return nil
		}
		attemptID, err = s.repo.InsertProviderAttempt(ctx, tx, current, uuid.NewString(), domain.InvocationRevoke)
		if err != nil {
			return err
		}
		if err := s.repo.InsertProviderCost(ctx, tx, current, attemptID); err != nil {
			return err
		}
		if err := appendAttemptFacts(ctx, tx, current, attemptID, domain.InvocationRevoke, cmd.ActorID, cmd.TraceID); err != nil {
			return err
		}
		if current.Status == domain.StatusIssuancePending {
			before := current.Status
			current.TerminalReason = reason
			if err := current.Transition(domain.StatusContainmentPending); err != nil {
				return err
			}
			if err := s.repo.InsertTransition(ctx, tx, current.ID, "containment/revoke/"+attemptID.String(), before, current.Status, nil, &attemptID, reason, current.UpdatedAt); err != nil {
				return err
			}
			if err := s.repo.UpdateProjection(ctx, tx, current); err != nil {
				return err
			}
			if err := appendTransitionFacts(ctx, tx, current, before, "DatasetDeliveryContainmentPending", reason, cmd.ActorID, cmd.TraceID); err != nil {
				return err
			}
		}
		operation = current
		proceed = true
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	if !proceed {
		if terminalRace {
			return s.revokeContainment(ctx, operation, capability, reason, cmd)
		}
		return Result{Operation: operation}, nil
	}
	if err := s.provider.Revoke(ctx, operation.ProviderRequestKey); errors.Is(err, ErrCapabilityNotFound) {
		if recordErr := s.recordObservation(ctx, operation.ID, attemptID, domain.ObservationCallReturn, domain.OutcomeNotFound, capability, "provider reports no active capability during containment"); recordErr != nil {
			return Result{}, recordErr
		}
		return s.finishWithoutCapability(ctx, operation.ID, cmd, target, reason, &attemptID)
	} else if err != nil {
		outcome := domain.OutcomeFailed
		if errors.Is(err, ErrUnknownProviderOutcome) {
			outcome = domain.OutcomeUnknown
		}
		reasonCode := providerFailureReason(err)
		if recordErr := s.recordObservation(ctx, operation.ID, attemptID, domain.ObservationCallReturn, outcome, domain.Capability{}, reasonCode); recordErr != nil {
			return Result{}, recordErr
		}
		return s.enterContainmentPending(ctx, operation.ID, reason+": "+reasonCode, cmd, &attemptID)
	}
	if err := s.recordObservation(ctx, operation.ID, attemptID, domain.ObservationCallReturn, domain.OutcomeSuccess, domain.Capability{}, "capability contained"); err != nil {
		return Result{}, err
	}
	return s.finishWithoutCapability(ctx, operation.ID, cmd, target, reason, &attemptID)
}

func (s *Service) reconcileContainment(ctx context.Context, operation domain.Operation, cmd IssueCredentialCommand) (Result, error) {
	// Recovery of an unknown operation must use the same provider key. The
	// reconciliation lookup is itself a physical provider attempt and is
	// durable before the external call.
	operation, evaluation, attemptID, proceed, err := s.prepareProviderCall(ctx, operation.ID, domain.GateReconciliation, domain.InvocationReconcile, cmd)
	if err != nil {
		return Result{}, err
	}
	if !proceed {
		return Result{Operation: operation}, nil
	}
	capability, err := s.provider.Recover(ctx, operation.ProviderRequestKey)
	if errors.Is(err, ErrCapabilityNotFound) {
		if recordErr := s.recordObservation(ctx, operation.ID, attemptID, domain.ObservationReconciliation, domain.OutcomeNotFound, domain.Capability{}, "provider reports no active capability"); recordErr != nil {
			return Result{}, recordErr
		}
		target := domain.StatusBlocked
		if evaluation.Allowed {
			target = domain.StatusFailed
		}
		return s.finishWithoutCapability(ctx, operation.ID, cmd, target, "provider reports no active capability", &attemptID)
	}
	if err != nil {
		outcome := domain.OutcomeFailed
		observation := domain.ObservationReconciliation
		if errors.Is(err, ErrUnknownProviderOutcome) {
			outcome = domain.OutcomeUnknown
			observation = domain.ObservationTimeout
		}
		reasonCode := providerFailureReason(err)
		if recordErr := s.recordObservation(ctx, operation.ID, attemptID, observation, outcome, domain.Capability{}, reasonCode); recordErr != nil {
			return Result{}, recordErr
		}
		return s.enterContainmentPending(ctx, operation.ID, reasonCode, cmd, &attemptID)
	}
	if err := s.recordObservation(ctx, operation.ID, attemptID, domain.ObservationReconciliation, domain.OutcomeSuccess, capability, "provider reports an existing capability"); err != nil {
		return Result{}, err
	}
	target := domain.StatusBlocked
	if evaluation.Allowed {
		target = domain.StatusFailed
	}
	return s.containCapability(ctx, operation, target, "recovered capability requires containment", capability, cmd)
}

func (s *Service) recordObservation(ctx context.Context, operationID, attemptID uuid.UUID, kind domain.ObservationKind, outcome domain.Outcome, capability domain.Capability, evidenceRef string) error {
	return s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		operation, err := s.repo.GetOperation(ctx, tx, operationID, false)
		if err != nil {
			return err
		}
		observationAttempts := []struct {
			id                  uuid.UUID
			kind                domain.ObservationKind
			resolutionAttemptID *uuid.UUID
		}{{id: attemptID, kind: kind}}
		originalAttemptID, found, err := s.repo.FindUnobservedIssueAttempt(ctx, tx, operationID, attemptID)
		if err != nil {
			return err
		}
		if found {
			observationAttempts = append(observationAttempts, struct {
				id                  uuid.UUID
				kind                domain.ObservationKind
				resolutionAttemptID *uuid.UUID
			}{id: originalAttemptID, kind: domain.ObservationReconciliation, resolutionAttemptID: &attemptID})
		}
		for _, observation := range observationAttempts {
			if err := s.repo.InsertProviderObservation(ctx, tx, observation.id, observation.kind, outcome, capability, evidenceRef); err != nil {
				return err
			}
			eventPayload := map[string]any{
				"operationId": operation.ID, "providerAttemptId": observation.id, "outcome": outcome,
				"observationKind": observation.kind, "capabilityRef": capability.CapabilityRef,
				"capabilityHash": capability.CapabilityHash,
			}
			afterState := map[string]any{"providerAttemptId": observation.id, "outcome": outcome, "observationKind": observation.kind}
			metadata := map[string]any{
				"providerAttemptId": observation.id, "observationKind": observation.kind,
				"outcome": outcome, "capabilityRef": capability.CapabilityRef,
				"capabilityHash": capability.CapabilityHash, "evidenceRef": evidenceRef,
			}
			if observation.resolutionAttemptID != nil {
				eventPayload["resolutionProviderAttemptId"] = *observation.resolutionAttemptID
				afterState["resolutionProviderAttemptId"] = *observation.resolutionAttemptID
				metadata["resolutionProviderAttemptId"] = *observation.resolutionAttemptID
			}
			event, err := outbox.NewEvent("DELIVERY_OPERATION", operation.ID, "DatasetDeliveryProviderObservationRecorded", eventPayload)
			if err != nil {
				return err
			}
			if err := outbox.Append(ctx, tx, event); err != nil {
				return err
			}
			if err := audit.Append(ctx, tx, audit.Event{
				WorkspaceID: &operation.WorkspaceID, ActorType: "SYSTEM", Action: "DELIVERY_PROVIDER_OBSERVED",
				ObjectType: "DELIVERY_OPERATION", ObjectID: operation.ID,
				AfterState: afterState, Reason: evidenceRef,
			}); err != nil {
				return err
			}
			if _, err := evidence.Append(ctx, tx, evidence.Record{
				WorkspaceID: operation.WorkspaceID, EvidenceType: "DELIVERY_PROVIDER_OBSERVATION",
				Title: "Delivery provider observation", SourceType: "DELIVERY_OPERATION", SourceID: &operation.ID,
				Metadata: metadata,
			}, evidence.Relation{ObjectType: "DELIVERY_OPERATION", ObjectID: operation.ID, RelationType: "PROVIDER_OBSERVATION"}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) enterContainmentPending(ctx context.Context, operationID uuid.UUID, reason string, cmd IssueCredentialCommand, providerAttemptID *uuid.UUID) (Result, error) {
	var operation domain.Operation
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		operation, err = s.repo.GetOperation(ctx, tx, operationID, true)
		if err != nil {
			return err
		}
		if operation.Status == domain.StatusContainmentPending {
			return nil
		}
		before := operation.Status
		if err := operation.Transition(domain.StatusContainmentPending); err != nil {
			return err
		}
		operation.TerminalReason = reason
		if err := s.repo.InsertTransition(ctx, tx, operation.ID, "containment/"+uuid.NewString(), before, operation.Status, nil, providerAttemptID, reason, operation.UpdatedAt); err != nil {
			return err
		}
		if err := s.repo.UpdateProjection(ctx, tx, operation); err != nil {
			return err
		}
		return appendTransitionFacts(ctx, tx, operation, before, "DatasetDeliveryContainmentPending", reason, cmd.ActorID, cmd.TraceID)
	})
	return Result{Operation: operation}, err
}

func (s *Service) finishWithoutCapability(ctx context.Context, operationID uuid.UUID, cmd IssueCredentialCommand, target domain.Status, reason string, providerAttemptID *uuid.UUID) (Result, error) {
	var operation domain.Operation
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		operation, err = s.repo.GetOperation(ctx, tx, operationID, true)
		if err != nil {
			return err
		}
		if operation.IsTerminal() {
			return nil
		}
		before := operation.Status
		if err := operation.Transition(target); err != nil {
			return err
		}
		operation.TerminalReason = reason
		if err := s.repo.InsertTransition(ctx, tx, operation.ID, "terminal/"+string(target)+"/"+uuid.NewString(), before, operation.Status, nil, providerAttemptID, reason, operation.UpdatedAt); err != nil {
			return err
		}
		if err := s.repo.UpdateProjection(ctx, tx, operation); err != nil {
			return err
		}
		return appendTransitionFacts(ctx, tx, operation, before, eventForTerminal(target), reason, cmd.ActorID, cmd.TraceID)
	})
	return Result{Operation: operation}, err
}

func (s *Service) evaluate(ctx context.Context, tx pgx.Tx, operation domain.Operation, stage domain.GateStage, revision int64, key string) (domain.GateEvaluation, error) {
	evaluation, err := s.gate.Evaluate(ctx, tx, domain.GateRequest{
		OperationID: operation.ID, WorkspaceID: operation.WorkspaceID, DatasetVersionID: operation.DatasetVersionID,
		CertificationRef: operation.CertificationRef, PrincipalRef: operation.PrincipalRef,
		EffectiveConsumerRef: operation.EffectiveConsumerRef, DelegationRef: operation.DelegationRef,
		Purpose: operation.Purpose, Action: operation.Action, ScopeRef: operation.ScopeRef,
		DeliveryChannel: operation.DeliveryChannel, DeliveryMode: operation.DeliveryMode, RequestedExpiresAt: operation.RequestedExpiresAt, Stage: stage,
	})
	if err != nil {
		return domain.GateEvaluation{}, fmt.Errorf("evaluate delivery gate: %w", err)
	}
	evaluation.ID = uuid.New()
	evaluation.EvaluationKey = key
	evaluation.Stage = stage
	evaluation.DependencyRevision = revision
	if evaluation.Allowed {
		if evaluation.PrincipalRef == "" || evaluation.EffectiveConsumerRef == "" ||
			evaluation.PrincipalRef != operation.PrincipalRef ||
			evaluation.EffectiveConsumerRef != operation.EffectiveConsumerRef ||
			evaluation.DelegationRef != operation.DelegationRef {
			return domain.GateEvaluation{}, fmt.Errorf("delivery gate returned untrusted caller context")
		}
	} else {
		if evaluation.PrincipalRef == "" {
			evaluation.PrincipalRef = operation.PrincipalRef
		}
		if evaluation.EffectiveConsumerRef == "" {
			evaluation.EffectiveConsumerRef = operation.EffectiveConsumerRef
		}
		if evaluation.DelegationRef == "" {
			evaluation.DelegationRef = operation.DelegationRef
		}
	}
	if evaluation.Allowed && (evaluation.FreshCapExpiresAt == nil || evaluation.FreshCapExpiresAt.After(operation.RequestedExpiresAt)) {
		return domain.GateEvaluation{}, fmt.Errorf("delivery gate returned an expiry cap wider than the request")
	}
	return evaluation, nil
}

func appendGateFacts(ctx context.Context, tx pgx.Tx, operation domain.Operation, evaluation domain.GateEvaluation, actorID *uuid.UUID, traceID string) error {
	event, err := outbox.NewEvent("DELIVERY_OPERATION", operation.ID, "DatasetDeliveryGateEvaluated", map[string]any{
		"operationId": operation.ID, "evaluationId": evaluation.ID, "evaluationKey": evaluation.EvaluationKey,
		"stage": evaluation.Stage, "decision": evaluation.Decision(), "blockers": evaluation.Blockers,
		"dependencyRevision": evaluation.DependencyRevision, "freshCapExpiresAt": evaluation.FreshCapExpiresAt,
		"principalRef": evaluation.PrincipalRef, "effectiveConsumerRef": evaluation.EffectiveConsumerRef,
		"delegationRef": evaluation.DelegationRef,
	})
	if err != nil {
		return err
	}
	if err := outbox.Append(ctx, tx, event); err != nil {
		return err
	}
	if err := audit.Append(ctx, tx, audit.Event{
		WorkspaceID: &operation.WorkspaceID, ActorType: "SYSTEM", ActorID: actorID,
		Action: "DELIVERY_GATE_EVALUATED", ObjectType: "DELIVERY_OPERATION", ObjectID: operation.ID,
		AfterState: map[string]any{
			"evaluationId": evaluation.ID, "stage": evaluation.Stage, "decision": evaluation.Decision(),
			"blockers": evaluation.Blockers, "dependencyRevision": evaluation.DependencyRevision,
			"principalRef": evaluation.PrincipalRef, "effectiveConsumerRef": evaluation.EffectiveConsumerRef,
			"delegationRef": evaluation.DelegationRef,
		}, TraceID: traceID,
	}); err != nil {
		return err
	}
	_, err = evidence.Append(ctx, tx, evidence.Record{
		WorkspaceID: operation.WorkspaceID, EvidenceType: "DELIVERY_GATE_EVALUATION", Title: "Delivery gate evaluation",
		SourceType: "DELIVERY_OPERATION", SourceID: &operation.ID,
		Metadata: map[string]any{
			"evaluationId": evaluation.ID, "stage": evaluation.Stage, "decision": evaluation.Decision(),
			"blockers": evaluation.Blockers, "dependencyRevision": evaluation.DependencyRevision,
			"principalRef": evaluation.PrincipalRef, "effectiveConsumerRef": evaluation.EffectiveConsumerRef,
			"delegationRef": evaluation.DelegationRef,
		}, CreatedBy: actorID,
	}, evidence.Relation{ObjectType: "DELIVERY_OPERATION", ObjectID: operation.ID, RelationType: "GATE_EVALUATION"})
	return err
}

func appendAttemptFacts(ctx context.Context, tx pgx.Tx, operation domain.Operation, attemptID uuid.UUID, kind domain.InvocationKind, actorID *uuid.UUID, traceID string) error {
	event, err := outbox.NewEvent("DELIVERY_OPERATION", operation.ID, "DatasetDeliveryProviderAttemptStarted", map[string]any{
		"operationId": operation.ID, "providerAttemptId": attemptID, "invocationKind": kind,
		"providerRequestKey": operation.ProviderRequestKey,
	})
	if err != nil {
		return err
	}
	if err := outbox.Append(ctx, tx, event); err != nil {
		return err
	}
	if err := audit.Append(ctx, tx, audit.Event{
		WorkspaceID: &operation.WorkspaceID, ActorType: "SYSTEM", ActorID: actorID,
		Action: "DELIVERY_PROVIDER_ATTEMPT_STARTED", ObjectType: "DELIVERY_OPERATION", ObjectID: operation.ID,
		AfterState: map[string]any{"providerAttemptId": attemptID, "invocationKind": kind}, TraceID: traceID,
	}); err != nil {
		return err
	}
	_, err = evidence.Append(ctx, tx, evidence.Record{
		WorkspaceID: operation.WorkspaceID, EvidenceType: "DELIVERY_PROVIDER_ATTEMPT", Title: "Provider attempt started",
		SourceType: "DELIVERY_OPERATION", SourceID: &operation.ID,
		Metadata: map[string]any{"providerAttemptId": attemptID, "invocationKind": kind, "providerRequestKey": operation.ProviderRequestKey}, CreatedBy: actorID,
	}, evidence.Relation{ObjectType: "DELIVERY_OPERATION", ObjectID: operation.ID, RelationType: "PROVIDER_ATTEMPT"})
	return err
}

func appendTransitionFacts(ctx context.Context, tx pgx.Tx, operation domain.Operation, before domain.Status, eventType, reason string, actorID *uuid.UUID, traceID string) error {
	event, err := outbox.NewEvent("DELIVERY_OPERATION", operation.ID, eventType, map[string]any{
		"operationId": operation.ID, "fromStatus": before, "toStatus": operation.Status,
		"reason": reason, "dependencyRevision": operation.DependencyRevision,
		"credentialRef": operation.CredentialRef, "credentialHash": operation.CredentialHash,
	})
	if err != nil {
		return err
	}
	if err := outbox.Append(ctx, tx, event); err != nil {
		return err
	}
	if err := audit.Append(ctx, tx, audit.Event{
		WorkspaceID: &operation.WorkspaceID, ActorType: "SYSTEM", ActorID: actorID,
		Action: strings.ToUpper(eventType), ObjectType: "DELIVERY_OPERATION", ObjectID: operation.ID,
		BeforeState: map[string]any{"status": before}, AfterState: map[string]any{"status": operation.Status, "reason": reason, "credentialRef": operation.CredentialRef, "credentialHash": operation.CredentialHash}, TraceID: traceID,
	}); err != nil {
		return err
	}
	_, err = evidence.Append(ctx, tx, evidence.Record{
		WorkspaceID: operation.WorkspaceID, EvidenceType: "DELIVERY_TRANSITION", Title: "Delivery operation transition",
		SourceType: "DELIVERY_OPERATION", SourceID: &operation.ID,
		Metadata: map[string]any{"fromStatus": before, "toStatus": operation.Status, "reason": reason, "credentialRef": operation.CredentialRef, "credentialHash": operation.CredentialHash}, CreatedBy: actorID,
	}, evidence.Relation{ObjectType: "DELIVERY_OPERATION", ObjectID: operation.ID, RelationType: "TRANSITION"})
	return err
}

func appendContainmentFactsTx(ctx context.Context, tx pgx.Tx, operation domain.Operation, attemptID uuid.UUID, status domain.ContainmentStatus, reason string, cmd IssueCredentialCommand) error {
	eventType := "DatasetDeliveryContainmentPending"
	action := "DELIVERY_CONTAINMENT_PENDING"
	if status == domain.ContainmentResolved {
		eventType = "DatasetDeliveryContainmentResolved"
		action = "DELIVERY_CONTAINMENT_RESOLVED"
	}
	event, err := outbox.NewEvent("DELIVERY_OPERATION", operation.ID, eventType, map[string]any{
		"operationId": operation.ID, "providerAttemptId": attemptID, "status": status, "reason": reason,
	})
	if err != nil {
		return err
	}
	if err := outbox.Append(ctx, tx, event); err != nil {
		return err
	}
	if err := audit.Append(ctx, tx, audit.Event{
		WorkspaceID: &operation.WorkspaceID, ActorType: "SYSTEM", ActorID: cmd.ActorID,
		Action: action, ObjectType: "DELIVERY_OPERATION", ObjectID: operation.ID,
		AfterState: map[string]any{"containmentStatus": status, "providerAttemptId": attemptID, "reason": reason}, TraceID: cmd.TraceID,
	}); err != nil {
		return err
	}
	_, err = evidence.Append(ctx, tx, evidence.Record{
		WorkspaceID: operation.WorkspaceID, EvidenceType: "DELIVERY_CONTAINMENT", Title: "Delivery containment state",
		SourceType: "DELIVERY_OPERATION", SourceID: &operation.ID,
		Metadata: map[string]any{"containmentStatus": status, "providerAttemptId": attemptID, "reason": reason}, CreatedBy: cmd.ActorID,
	}, evidence.Relation{ObjectType: "DELIVERY_OPERATION", ObjectID: operation.ID, RelationType: "CONTAINMENT"})
	return err
}

func validateCommand(cmd IssueCredentialCommand) error {
	if cmd.WorkspaceID == uuid.Nil || cmd.DatasetVersionID == uuid.Nil || strings.TrimSpace(cmd.ProviderName) == "" || strings.TrimSpace(cmd.PrincipalRef) == "" || strings.TrimSpace(cmd.EffectiveConsumerRef) == "" || strings.TrimSpace(cmd.Purpose) == "" || strings.TrimSpace(cmd.Action) == "" || strings.TrimSpace(cmd.ScopeRef) == "" || strings.TrimSpace(cmd.DeliveryChannel) == "" || strings.TrimSpace(cmd.DeliveryMode) == "" || strings.TrimSpace(cmd.IdempotencyKey) == "" || len(cmd.IdempotencyKey) > 255 || cmd.RequestedExpiresAt.IsZero() {
		return domain.ErrInvalidOperation
	}
	return nil
}

func commandFingerprint(cmd IssueCredentialCommand) (string, error) {
	payload := struct {
		WorkspaceID        uuid.UUID  `json:"workspaceId"`
		DatasetVersionID   uuid.UUID  `json:"datasetVersionId"`
		CertificationRef   *uuid.UUID `json:"certificationRef,omitempty"`
		ProviderName       string     `json:"providerName"`
		PrincipalRef       string     `json:"principalRef"`
		EffectiveConsumer  string     `json:"effectiveConsumerRef"`
		DelegationRef      string     `json:"delegationRef,omitempty"`
		Purpose            string     `json:"purpose"`
		Action             string     `json:"action"`
		ScopeRef           string     `json:"scopeRef"`
		DeliveryChannel    string     `json:"deliveryChannel"`
		DeliveryMode       string     `json:"deliveryMode"`
		RequestedExpiresAt time.Time  `json:"requestedExpiresAt"`
	}{cmd.WorkspaceID, cmd.DatasetVersionID, cmd.CertificationRef, strings.TrimSpace(cmd.ProviderName), strings.TrimSpace(cmd.PrincipalRef), strings.TrimSpace(cmd.EffectiveConsumerRef), strings.TrimSpace(cmd.DelegationRef), strings.TrimSpace(cmd.Purpose), strings.TrimSpace(cmd.Action), strings.TrimSpace(cmd.ScopeRef), strings.TrimSpace(cmd.DeliveryChannel), strings.TrimSpace(cmd.DeliveryMode), cmd.RequestedExpiresAt.UTC()}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal delivery command fingerprint: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func hashSecret(secret string) string {
	digest := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(digest[:])
}

func providerFailureReason(err error) string {
	switch {
	case errors.Is(err, ErrCapabilityNotFound):
		return "PROVIDER_CAPABILITY_NOT_FOUND"
	case errors.Is(err, ErrUnknownProviderOutcome):
		return "PROVIDER_OUTCOME_UNKNOWN"
	default:
		return "PROVIDER_CALL_FAILED"
	}
}

func firstBlocker(blockers []string) string {
	if len(blockers) == 0 {
		return "DELIVERY_GATE_BLOCKED"
	}
	return blockers[0]
}

func eventForTerminal(status domain.Status) string {
	switch status {
	case domain.StatusBlocked:
		return "DatasetDeliveryBlocked"
	case domain.StatusFailed:
		return "DatasetDeliveryFailed"
	default:
		return "DatasetDeliveryFailed"
	}
}
