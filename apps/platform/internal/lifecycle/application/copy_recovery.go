package application

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"time"
)

const CapabilityReconcileCopy = "LIFECYCLE_RECONCILE_COPY"
const CopyReconciliationRequestedEvent = "LifecycleCopyRecoveryRequested"
const StorageRecoverCopyVersions = "RECOVER_COPY_VERSIONS"
const StorageRecoverCopyExact = "RECOVER_COPY_EXACT"

var ErrCopyTargetUnknown = errors.New("copy target missing or ambiguous; original call remains unknown")

// Discovery recovers custody identity, not original-call outcome. No method can
// resend COPY/RESTORE, delete a version, export a restore witness or release a hold.
type CopyDiscoveryProof struct {
	Copy                        domain.Copy
	InventoryCallID, StatCallID uuid.UUID
	OperationTag                uuid.UUID
	ObservedAt                  time.Time
}
type CopyRecoveryPort interface {
	DiscoverCopyTarget(context.Context, domain.CopyOperation) (CopyDiscoveryProof, error)
	VerifyCopyBytes(context.Context, domain.CopyOperation, domain.Copy) (CopyBytesProof, error)
	InspectRetentionAndHold(context.Context, domain.Copy) (CopyProtection, error)
}
type CopyRecoveryService struct {
	copies *CopyService
	port   CopyRecoveryPort
}

// Per-run view avoids sharing mutable protection caches between queue workers.
type recoveredProtection map[uuid.UUID]CopyProtection

func (p recoveredProtection) CurrentCopyProtection(_ context.Context, _ pgx.Tx, c domain.Copy, _ time.Time) (CopyProtection, error) {
	v, ok := p[c.ID]
	if !ok || v.Copy != c {
		return CopyProtection{}, ErrStorageUnknown
	}
	return v, nil
}

func NewCopyRecoveryService(c *CopyService, p CopyRecoveryPort) *CopyRecoveryService {
	return &CopyRecoveryService{c, p}
}

// Separate journal type rejects mutations even if a caller reaches the adapter.
type CopyRecoveryCallJournal struct {
	Service  *CopyService
	Metadata *PostgresStorageCallJournal
}

func (j CopyRecoveryCallJournal) StartStorageCall(ctx context.Context, c StorageCallStart) error {
	if c.CopyOperationID == uuid.Nil {
		return j.Metadata.StartStorageCall(ctx, c)
	}
	switch c.Operation {
	case StorageRecoverCopyVersions, StorageRecoverCopyExact, StorageVerifyCopy:
		return j.Service.startCopyCall(ctx, c, true)
	default:
		return ErrStorageUnsupported
	}
}
func (j CopyRecoveryCallJournal) ObserveStorageCall(ctx context.Context, c StorageCallObservation) error {
	return j.Metadata.ObserveStorageCall(ctx, c)
}

func appendCopyRecoveryWork(ctx context.Context, tx pgx.Tx, op domain.CopyOperation, call uuid.UUID, basis string, p principal.RequestPrincipal, rev int64, at time.Time) (uuid.UUID, error) {
	id := uuid.New()
	_, e := tx.Exec(ctx, `INSERT INTO lifecycle_reconciliation_work(id,workspace_id,copy_call_id,basis_ref,requested_at,actor_id,principal_ref,dependency_revision) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, op.WorkspaceID, call, basis, at, p.ActorID, p.Subject, rev)
	if e != nil {
		return uuid.Nil, e
	}
	return id, appendLifecycleFacts(ctx, tx, "LIFECYCLE_RECONCILIATION", CopyReconciliationRequestedEvent, "LIFECYCLE_RECONCILIATION_WORK", id, p, rev, at, map[string]any{"copy_call_id": call, "copy_operation_id": op.ID, "basis_ref": basis})
}

type RequestCopyRecoveryCommand struct {
	CommandHeader
	CopyCallID uuid.UUID
	BasisRef   string
}

func (s *CopyRecoveryService) RequestRecovery(ctx context.Context, c RequestCopyRecoveryCommand) (FactResult, error) {
	if s == nil || s.copies == nil || s.port == nil || c.CopyCallID == uuid.Nil || !validText(c.BasisRef, 255) {
		return FactResult{}, ErrStorageScope
	}
	o := s.copies.occupancy
	return o.command(ctx, c.CommandHeader, "LIFECYCLE.REQUEST_COPY_RECOVERY", CapabilityReconcileCopy, c, func(ctx context.Context, tx pgx.Tx, p principal.RequestPrincipal, rev int64) (uuid.UUID, error) {
		var opID uuid.UUID
		if e := tx.QueryRow(ctx, `SELECT copy_operation_id FROM lifecycle_storage_call WHERE workspace_id=$1 AND id=$2 AND operation IN('ARCHIVE_COPY','RESTORE_COPY')`, c.WorkspaceID, c.CopyCallID).Scan(&opID); e != nil {
			return uuid.Nil, e
		}
		op, e := s.copies.lock(ctx, tx, c.WorkspaceID, opID)
		if e != nil {
			return uuid.Nil, e
		}
		p, at, e := o.authorize(ctx, tx, c.WorkspaceID, CapabilityReconcileCopy, &p)
		if e != nil {
			return uuid.Nil, e
		}
		return appendCopyRecoveryWork(ctx, tx, op, c.CopyCallID, c.BasisRef, p, rev, at)
	})
}

type bindRecoveredCommand struct {
	CommandHeader
	OperationID, OriginalCallID uuid.UUID
	Proof                       CopyDiscoveryProof
}

func (s *CopyService) bindRecoveredCopy(ctx context.Context, c bindRecoveredCommand) (FactResult, error) {
	p := c.Proof
	if p.Copy.Validate() != nil || p.InventoryCallID == uuid.Nil || p.StatCallID == uuid.Nil || p.OperationTag != c.OperationID || c.OriginalCallID == uuid.Nil {
		return FactResult{}, ErrStorageScope
	}
	o := s.occupancy
	return o.command(ctx, c.CommandHeader, "LIFECYCLE.BIND_RECOVERED_COPY", CapabilityReconcileCopy, c, func(ctx context.Context, tx pgx.Tx, caller principal.RequestPrincipal, _ int64) (uuid.UUID, error) {
		op, e := s.lock(ctx, tx, c.WorkspaceID, c.OperationID)
		if e != nil {
			return uuid.Nil, e
		}
		if p.Copy != op.Materialized(p.Copy.ProviderVersion) {
			return uuid.Nil, ErrStorageScope
		}
		caller, at, e := o.authorize(ctx, tx, c.WorkspaceID, CapabilityReconcileCopy, &caller)
		if e != nil {
			return uuid.Nil, e
		}
		if p.ObservedAt.Before(op.CreatedAt) || p.ObservedAt.After(at) || at.Sub(p.ObservedAt) > time.Minute {
			return uuid.Nil, ErrStorageContract
		}
		id := uuid.New()
		_, e = tx.Exec(ctx, `INSERT INTO lifecycle_copy_discovery(id,workspace_id,operation_id,original_call_id,inventory_call_id,stat_call_id,provider_version,observed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, c.WorkspaceID, op.ID, c.OriginalCallID, p.InventoryCallID, p.StatCallID, p.Copy.ProviderVersion, at)
		if e != nil {
			return uuid.Nil, e
		}
		_, e = tx.Exec(ctx, `INSERT INTO lifecycle_fixture_copy(id,workspace_id,namespace_id,dataset_version_id,object_key,provider_version,content_sha256,byte_size) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, p.Copy.ID, c.WorkspaceID, op.TargetNamespaceID, p.Copy.DatasetVersionID, p.Copy.Key, p.Copy.ProviderVersion, p.Copy.ContentSHA256, p.Copy.ByteSize)
		if e != nil {
			return uuid.Nil, e
		}
		_, e = tx.Exec(ctx, `INSERT INTO lifecycle_copy_materialization(operation_id,workspace_id,copy_id,copy_call_id,discovery_id,observed_at) VALUES($1,$2,$3,$4,$5,$6)`, op.ID, c.WorkspaceID, p.Copy.ID, c.OriginalCallID, id, at)
		if e != nil {
			return uuid.Nil, e
		}
		rev, e := deliveryfence.Advance(ctx, tx, c.WorkspaceID)
		if e != nil {
			return uuid.Nil, e
		}
		return id, appendDispositionFacts(ctx, tx, "LifecycleCopyTargetRecovered", "LIFECYCLE_COPY_DISCOVERY", id, caller, rev, at, map[string]any{"operation_id": op.ID, "original_call_id": c.OriginalCallID, "target_copy_id": p.Copy.ID, "original_observation_unchanged": true})
	})
}

type copyRecoveryRun struct {
	id, work, w, original                uuid.UUID
	op                                   domain.CopyOperation
	caller                               principal.RequestPrincipal
	inventory, stat, discovery, decision uuid.UUID
	outcome                              string
}

func recoveryAuthorityError(e error) bool {
	return errors.Is(e, principal.ErrCapabilityDenied) || errors.Is(e, principal.ErrNotConfigured) || errors.Is(e, principal.ErrUnauthenticated) || errors.Is(e, principal.ErrWorkspaceDenied)
}
func (s *CopyRecoveryService) run(ctx context.Context, w, work, original uuid.UUID) error {
	if s == nil || s.copies == nil || s.port == nil {
		return ErrStorageUnsupported
	}
	o := s.copies.occupancy
	r := copyRecoveryRun{w: w, work: work, original: original}
	finished := false
	e := o.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rev, e := deliveryfence.Lock(ctx, tx, w)
		if e != nil {
			return e
		}
		var call uuid.UUID
		if e = tx.QueryRow(ctx, `SELECT copy_call_id FROM lifecycle_reconciliation_work WHERE workspace_id=$1 AND id=$2 FOR UPDATE`, w, work).Scan(&call); e != nil || call != original {
			return ErrStorageScope
		}
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_reconciliation_result WHERE work_id=$1 AND terminal)`, work).Scan(&finished); e != nil || finished {
			return e
		}
		var n int
		if e = tx.QueryRow(ctx, `SELECT count(*) FROM lifecycle_reconciliation_run WHERE work_id=$1`, work).Scan(&n); e != nil {
			return e
		}
		if n >= MaxRecoveryRuns {
			return ErrRecoveryExhausted
		}
		var opID uuid.UUID
		if e = tx.QueryRow(ctx, `SELECT copy_operation_id FROM lifecycle_storage_call WHERE id=$1 AND workspace_id=$2 AND operation IN('ARCHIVE_COPY','RESTORE_COPY')`, original, w).Scan(&opID); e != nil {
			return e
		}
		r.op, e = s.copies.lock(ctx, tx, w, opID)
		if e != nil {
			return e
		}
		p, at, authErr := o.authorize(ctx, tx, w, CapabilityReconcileCopy, nil)
		if authErr == nil {
			_, _, authErr = o.authorize(ctx, tx, w, CapabilityRestoreAdmission, &p)
		}
		if authErr != nil {
			if !recoveryAuthorityError(authErr) {
				return authErr
			}
			if e = tx.QueryRow(ctx, `SELECT actor_id,principal_ref FROM lifecycle_reconciliation_work WHERE id=$1`, work).Scan(&p.ActorID, &p.Subject); e != nil {
				return e
			}
			p.WorkspaceID = w
			r.outcome = "BLOCKED_AUTHORITY"
		}
		r.caller = p
		r.id = uuid.New()
		if _, e = tx.Exec(ctx, `INSERT INTO lifecycle_reconciliation_run(id,workspace_id,work_id,started_at,actor_id,principal_ref,dependency_revision) VALUES($1,$2,$3,$4,$5,$6,$7)`, r.id, w, work, at, p.ActorID, p.Subject, rev); e != nil {
			return e
		}
		if e = appendLifecycleFacts(ctx, tx, "LIFECYCLE_RECONCILIATION", "LifecycleCopyRecoveryStarted", "LIFECYCLE_RECONCILIATION_RUN", r.id, p, rev, at, map[string]any{"work_id": work, "copy_call_id": original, "operation_id": opID}); e != nil {
			return e
		}
		if r.outcome != "" {
			return nil
		}
		e = tx.QueryRow(ctx, `SELECT id FROM lifecycle_copy_decision WHERE operation_id=$1 AND decision IN('ARCHIVED','ONLINE')`, opID).Scan(&r.decision)
		if e == nil {
			r.outcome = "ALREADY_RESOLVED"
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		return nil
	})
	if e != nil || finished {
		return e
	}
	if r.outcome != "" {
		return s.observe(ctx, &r, nil)
	}
	var target domain.Copy
	var materialized, verified bool
	e = o.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var version string
		var discovery *uuid.UUID
		e := tx.QueryRow(ctx, `SELECT c.provider_version,m.discovery_id FROM lifecycle_copy_materialization m JOIN lifecycle_fixture_copy c ON c.id=m.copy_id WHERE m.operation_id=$1`, r.op.ID).Scan(&version, &discovery)
		if errors.Is(e, pgx.ErrNoRows) {
			return nil
		}
		if e != nil {
			return e
		}
		if discovery != nil {
			r.discovery = *discovery
		}
		target = r.op.Materialized(version)
		materialized = true
		return tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_copy_verification WHERE operation_id=$1)`, r.op.ID).Scan(&verified)
	})
	if e != nil {
		return s.observe(ctx, &r, e)
	}
	if !materialized {
		proof, e := s.port.DiscoverCopyTarget(ctx, r.op)
		r.inventory, r.stat = proof.InventoryCallID, proof.StatCallID
		if errors.Is(e, ErrCopyTargetUnknown) {
			r.outcome = "UNKNOWN_COPY_TARGET"
			return s.observe(ctx, &r, nil)
		}
		if e != nil {
			return s.observe(ctx, &r, e)
		}
		bound, e := s.copies.bindRecoveredCopy(ctx, bindRecoveredCommand{CommandHeader: CommandHeader{w, "copy-recover-bind:" + work.String()}, OperationID: r.op.ID, OriginalCallID: original, Proof: proof})
		if e != nil {
			return s.observe(ctx, &r, e)
		}
		r.discovery = bound.ID
		target = proof.Copy
	}
	proofs := recoveredProtection{}
	for _, cp := range []domain.Copy{r.op.Source, target} {
		proof, e := s.port.InspectRetentionAndHold(ctx, cp)
		if e != nil {
			return s.observe(ctx, &r, e)
		}
		proof.ObservedAt = proof.ObservedAt.UTC().Truncate(time.Microsecond)
		if proof.Copy != cp || proof.ObservedAt.IsZero() || proof.ObservedAt.After(o.clock()) || o.clock().Sub(proof.ObservedAt) > time.Minute {
			return s.observe(ctx, &r, ErrStorageContract)
		}
		proofs[cp.ID] = proof
	}
	local := *s.copies
	local.protection = proofs
	if !verified {
		proof, e := s.port.VerifyCopyBytes(ctx, r.op, target)
		if e != nil {
			return s.observe(ctx, &r, e)
		}
		if _, e = local.verifyMaterializedCopy(ctx, VerifyCopyCommand{CommandHeader: CommandHeader{w, "copy-recover-verify:" + work.String()}, OperationID: r.op.ID, Proof: proof}, CapabilityReconcileCopy); e != nil {
			return s.observe(ctx, &r, e)
		}
	}
	decision, e := local.finalizeCopy(ctx, FinalizeCopyCommand{CommandHeader: CommandHeader{w, "copy-recover-finalize:" + work.String()}, OperationID: r.op.ID}, true)
	if e != nil {
		return s.observe(ctx, &r, e)
	}
	r.decision = decision.ID
	switch decision.Decision {
	case "ARCHIVED":
		r.outcome = "COPY_ARCHIVED"
	case "ONLINE":
		r.outcome = "RESTORE_ONLINE"
	case "BLOCKED":
		r.outcome = "BLOCKED_COPY"
	default:
		r.outcome = "UNKNOWN_COPY_LEDGER"
	}
	return s.observe(ctx, &r, nil)
}

func (s *CopyRecoveryService) observe(ctx context.Context, r *copyRecoveryRun, cause error) error {
	o := s.copies.occupancy
	e := o.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rev, e := deliveryfence.Lock(ctx, tx, r.w)
		if e != nil {
			return e
		}
		if r.outcome != "BLOCKED_AUTHORITY" {
			if _, _, authErr := o.authorize(ctx, tx, r.w, CapabilityReconcileCopy, &r.caller); authErr != nil {
				if !recoveryAuthorityError(authErr) {
					return authErr
				}
				r.outcome = "BLOCKED_AUTHORITY"
				cause = authErr
			}
		}
		if cause != nil && r.outcome != "BLOCKED_AUTHORITY" {
			r.outcome = "UNKNOWN_PROVIDER"
		}
		if r.outcome == "" {
			r.outcome = "UNKNOWN_PROVIDER"
		}
		id := uuid.New()
		at := o.clock()
		_, e = tx.Exec(ctx, `INSERT INTO lifecycle_reconciliation_result(id,workspace_id,work_id,run_id,outcome,terminal,inventory_call_id,stat_call_id,copy_discovery_id,copy_decision_id,observed_at,dependency_revision) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, id, r.w, r.work, r.id, r.outcome, r.outcome != "UNKNOWN_PROVIDER", nullableID(r.inventory), nullableID(r.stat), nullableID(r.discovery), nullableID(r.decision), at, rev)
		if e != nil {
			return e
		}
		return appendLifecycleFacts(ctx, tx, "LIFECYCLE_RECONCILIATION", "LifecycleCopyRecoveryObserved", "LIFECYCLE_RECONCILIATION_RESULT", id, r.caller, rev, at, map[string]any{"work_id": r.work, "copy_call_id": r.original, "operation_id": r.op.ID, "outcome": r.outcome, "original_observation_unchanged": true})
	})
	if e != nil {
		return e
	}
	if r.outcome == "UNKNOWN_PROVIDER" {
		return fmt.Errorf("copy recovery uncertain: %w", cause)
	}
	return nil
}
