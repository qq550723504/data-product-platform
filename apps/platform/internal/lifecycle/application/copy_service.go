package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"strings"
	"time"
)

const (
	CapabilityCopyCustody      = "LIFECYCLE_COPY_CUSTODY"
	CapabilityRestoreAdmission = "LIFECYCLE_RESTORE_ADMISSION"
	StorageArchiveCopy         = "ARCHIVE_COPY"
	StorageRestoreCopy         = "RESTORE_COPY"
	StorageVerifyCopy          = "VERIFY_COPY_BYTES"
)

type CopyTargetSlot struct {
	Slot, Action, Namespace, Key string
	NamespaceID                  uuid.UUID
}
type RestoreLedger interface {
	Latest(context.Context) (domain.RestoreCheckpoint, error)
}
type CopyProtectionGate interface {
	CurrentCopyProtection(context.Context, pgx.Tx, domain.Copy, time.Time) (CopyProtection, error)
}
type CustodyCopyPort interface {
	CreateCopy(context.Context, domain.CopyOperation) (domain.Copy, uuid.UUID, error)
	VerifyCopyBytes(context.Context, domain.CopyOperation, domain.Copy) (CopyBytesProof, error)
}
type CopyBytesProof struct {
	Copy       domain.Copy
	CallID     uuid.UUID
	SHA256     string
	Bytes      int64
	ObservedAt time.Time
}
type CopyService struct {
	occupancy  *OccupancyService
	slots      map[string]CopyTargetSlot
	protection CopyProtectionGate
	ledger     RestoreLedger
	epoch      uuid.UUID
}

func NewCopyService(o *OccupancyService, slots []CopyTargetSlot, p CopyProtectionGate, l RestoreLedger, epoch uuid.UUID) *CopyService {
	if o == nil || !o.configured() {
		return nil
	}
	m := map[string]CopyTargetSlot{}
	for _, s := range slots {
		if !validText(s.Slot, 64) || !validText(s.Namespace, 255) || !validText(s.Key, 1024) || s.NamespaceID == uuid.Nil || (s.Action != domain.ArchiveCopy && s.Action != domain.RestoreCopy) || m[s.Slot].Slot != "" {
			return nil
		}
		m[s.Slot] = s
	}
	if len(m) == 0 || len(m) > 32 || p == nil || epoch == uuid.Nil {
		return nil
	}
	return &CopyService{o, m, p, l, epoch}
}

type BindCopyPolicyCommand struct {
	CommandHeader
	DatasetVersionID       uuid.UUID
	UseUntil, RestoreUntil time.Time
}

func (s *CopyService) BindFixtureCopyPolicy(ctx context.Context, c BindCopyPolicyCommand) (FactResult, error) {
	if s == nil || c.DatasetVersionID == uuid.Nil {
		return FactResult{}, ErrStorageScope
	}
	o := s.occupancy
	c.UseUntil = c.UseUntil.UTC().Truncate(time.Microsecond)
	c.RestoreUntil = c.RestoreUntil.UTC().Truncate(time.Microsecond)
	return o.command(ctx, c.CommandHeader, "LIFECYCLE.BIND_COPY_POLICY", CapabilityCopyCustody, c, func(ctx context.Context, tx pgx.Tx, caller principal.RequestPrincipal, _ int64) (uuid.UUID, error) {
		ids, e := o.repo.FixtureCopyIDsForVersion(ctx, tx, c.WorkspaceID, c.DatasetVersionID)
		if e != nil {
			return uuid.Nil, e
		}
		if _, e = o.repo.LockFixtureCopies(ctx, tx, c.WorkspaceID, ids); e != nil {
			return uuid.Nil, e
		}
		_, at, e := o.authorize(ctx, tx, c.WorkspaceID, CapabilityCopyCustody, &caller)
		if e != nil {
			return uuid.Nil, e
		}
		if !c.UseUntil.After(at) || !c.RestoreUntil.After(at) || c.UseUntil.After(at.Add(time.Hour)) || c.RestoreUntil.After(at.Add(time.Hour)) {
			return uuid.Nil, ErrStorageDenied
		}
		rev, e := deliveryfence.Advance(ctx, tx, c.WorkspaceID)
		if e != nil {
			return uuid.Nil, e
		}
		id := uuid.New()
		_, e = tx.Exec(ctx, `INSERT INTO lifecycle_copy_policy(id,workspace_id,dataset_version_id,basis_ref,use_until,restore_until,actor_id,principal_ref,created_at) VALUES($1,$2,$3,'CONTROLLED_SYNTHETIC_CUSTODY_AND_USE',$4,$5,$6,$7,$8)`, id, c.WorkspaceID, c.DatasetVersionID, c.UseUntil, c.RestoreUntil, caller.ActorID, caller.Subject, at)
		if e != nil {
			return uuid.Nil, e
		}
		return id, appendDispositionFacts(ctx, tx, "LifecycleCopyPolicyBound", "LIFECYCLE_COPY_POLICY", id, caller, rev, at, map[string]any{"dataset_version_id": c.DatasetVersionID})
	})
}

type PrepareCopyCommand struct {
	CommandHeader
	SourceCopyID         uuid.UUID
	TargetSlot, BasisRef string
	ExpiresAt            time.Time
}

func (s *CopyService) PrepareCopy(ctx context.Context, c PrepareCopyCommand) (domain.CopyOperation, error) {
	if s == nil || c.SourceCopyID == uuid.Nil || !validText(c.BasisRef, 255) {
		return domain.CopyOperation{}, ErrStorageScope
	}
	slot, ok := s.slots[c.TargetSlot]
	if !ok {
		return domain.CopyOperation{}, ErrStorageScope
	}
	c.ExpiresAt = c.ExpiresAt.UTC().Truncate(time.Microsecond)
	o := s.occupancy
	r, e := o.command(ctx, c.CommandHeader, "LIFECYCLE.PREPARE_COPY", CapabilityCopyCustody, c, func(ctx context.Context, tx pgx.Tx, caller principal.RequestPrincipal, _ int64) (uuid.UUID, error) {
		cs, e := o.repo.LockFixtureCopies(ctx, tx, c.WorkspaceID, []uuid.UUID{c.SourceCopyID})
		if e != nil {
			return uuid.Nil, e
		}
		src := cs[0]
		_, at, e := o.authorize(ctx, tx, c.WorkspaceID, CapabilityCopyCustody, &caller)
		if e != nil {
			return uuid.Nil, e
		}
		if !c.ExpiresAt.After(at) || c.ExpiresAt.After(at.Add(time.Hour)) {
			return uuid.Nil, ErrStorageDenied
		}
		var policy uuid.UUID
		var deadline time.Time
		e = tx.QueryRow(ctx, `SELECT id,restore_until FROM lifecycle_copy_policy WHERE workspace_id=$1 AND dataset_version_id=$2`, c.WorkspaceID, src.DatasetVersionID).Scan(&policy, &deadline)
		if e != nil {
			return uuid.Nil, e
		}
		if c.ExpiresAt.After(deadline) {
			return uuid.Nil, ErrStorageDenied
		}
		var targetValid bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_fixture_namespace WHERE id=$1 AND workspace_id=$2 AND namespace=$3 AND provider_instance_id=$4 AND incarnation_id=$5)`, slot.NamespaceID, c.WorkspaceID, slot.Namespace, src.ProviderInstanceID, src.IncarnationID).Scan(&targetValid)
		if e != nil || !targetValid {
			return uuid.Nil, ErrStorageScope
		}
		if slot.Action == domain.RestoreCopy {
			var archived bool
			e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_copy_materialization m JOIN lifecycle_copy_decision d ON d.operation_id=m.operation_id WHERE m.copy_id=$1 AND d.decision='ARCHIVED')`, src.ID).Scan(&archived)
			if e != nil || !archived {
				return uuid.Nil, ErrStorageDenied
			}
		}
		if e = s.custodyGate(ctx, tx, src, at); e != nil {
			return uuid.Nil, e
		}
		rev, e := deliveryfence.Advance(ctx, tx, c.WorkspaceID)
		if e != nil {
			return uuid.Nil, e
		}
		id := uuid.New()
		_, e = tx.Exec(ctx, `INSERT INTO lifecycle_copy_operation(id,workspace_id,source_copy_id,target_copy_id,target_namespace_id,target_key,action,policy_id,basis_ref,actor_id,principal_ref,dependency_revision,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, id, c.WorkspaceID, src.ID, uuid.New(), slot.NamespaceID, slot.Key, slot.Action, policy, c.BasisRef, caller.ActorID, caller.Subject, rev, at, c.ExpiresAt)
		if e != nil {
			return uuid.Nil, e
		}
		return id, appendDispositionFacts(ctx, tx, "LifecycleCopyPrepared", "LIFECYCLE_COPY_OPERATION", id, caller, rev, at, map[string]any{"source_copy_id": src.ID, "action": slot.Action})
	})
	if e != nil {
		return domain.CopyOperation{}, e
	}
	return s.GetOperation(ctx, c.WorkspaceID, r.ID)
}
func (s *CopyService) GetOperation(ctx context.Context, w, id uuid.UUID) (domain.CopyOperation, error) {
	var r domain.CopyOperation
	e := s.occupancy.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error { var e error; r, e = s.load(ctx, tx, w, id); return e })
	return r, e
}
func (s *CopyService) load(ctx context.Context, tx pgx.Tx, w, id uuid.UUID) (domain.CopyOperation, error) {
	var r domain.CopyOperation
	var source uuid.UUID
	e := tx.QueryRow(ctx, `SELECT o.id,o.workspace_id,o.source_copy_id,o.target_copy_id,o.target_namespace_id,n.namespace,o.target_key,o.action,o.policy_id,o.basis_ref,o.actor_id,o.principal_ref,o.dependency_revision,o.created_at,o.expires_at FROM lifecycle_copy_operation o JOIN lifecycle_fixture_namespace n ON n.id=o.target_namespace_id WHERE o.workspace_id=$1 AND o.id=$2`, w, id).Scan(&r.ID, &r.WorkspaceID, &source, &r.TargetCopyID, &r.TargetNamespaceID, &r.TargetNamespace, &r.TargetKey, &r.Action, &r.PolicyID, &r.BasisRef, &r.ActorID, &r.PrincipalRef, &r.DependencyRevision, &r.CreatedAt, &r.ExpiresAt)
	if e != nil {
		return r, e
	}
	cs, e := s.occupancy.repo.LoadFixtureCopies(ctx, tx, w, []uuid.UUID{source})
	if e != nil {
		return r, e
	}
	r.Source = cs[0]
	return r, r.Validate()
}
func (s *CopyService) lock(ctx context.Context, tx pgx.Tx, w, id uuid.UUID) (domain.CopyOperation, error) {
	r, e := s.load(ctx, tx, w, id)
	if e != nil {
		return r, e
	}
	ids := []uuid.UUID{r.Source.ID}
	var exists bool
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_fixture_copy WHERE id=$1)`, r.TargetCopyID).Scan(&exists)
	if e != nil {
		return r, e
	}
	if exists {
		ids = append(ids, r.TargetCopyID)
	}
	if _, e = s.occupancy.repo.LockFixtureCopies(ctx, tx, w, ids); e != nil {
		return r, e
	}
	var locked uuid.UUID
	e = tx.QueryRow(ctx, `SELECT id FROM lifecycle_copy_operation WHERE id=$1 AND workspace_id=$2 FOR UPDATE`, id, w).Scan(&locked)
	return r, e
}
func (s *CopyService) custodyGate(ctx context.Context, tx pgx.Tx, c domain.Copy, at time.Time) error {
	occ, e := s.occupancy.repo.CopyOccupancy(ctx, tx, c)
	if e != nil {
		return e
	}
	if occ.ActiveHolds > 0 || occ.ActiveReaders > 0 {
		return ErrStorageDenied
	}
	var pending bool
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_disposition_dispatch d WHERE d.copy_id=$1 AND NOT EXISTS(SELECT 1 FROM lifecycle_dispatch_release r WHERE r.dispatch_id=d.id)) OR EXISTS(SELECT 1 FROM lifecycle_copy_absence_receipt WHERE copy_id=$1)`, c.ID).Scan(&pending)
	if e != nil {
		return e
	}
	if pending {
		return ErrStorageDenied
	}
	p, e := s.protection.CurrentCopyProtection(ctx, tx, c, at)
	if e != nil {
		return e
	}
	return unprotectedFresh(p, c, at)
}
func unprotectedFresh(p CopyProtection, c domain.Copy, at time.Time) error {
	if p.Copy != c || p.ObservedAt.IsZero() || p.ObservedAt.After(at) || at.Sub(p.ObservedAt) > time.Minute {
		return ErrStorageUnknown
	}
	if p.LegalHold || p.RetentionMode != "NONE" || p.RetainUntil != nil {
		return ErrStorageUnsupported
	}
	return nil
}

// The wrapper admits COPY/VERIFY against an existing intent, while reusing the
// original metadata journal and every observation/cost transaction unchanged.
type CopyCallJournal struct {
	Service  *CopyService
	Metadata *PostgresStorageCallJournal
}

func (j CopyCallJournal) StartStorageCall(ctx context.Context, c StorageCallStart) error {
	if c.CopyOperationID == uuid.Nil {
		return j.Metadata.StartStorageCall(ctx, c)
	}
	return j.Service.startCopyCall(ctx, c, false)
}
func (j CopyCallJournal) ObserveStorageCall(ctx context.Context, c StorageCallObservation) error {
	return j.Metadata.ObserveStorageCall(ctx, c)
}
func (s *CopyService) startCopyCall(ctx context.Context, c StorageCallStart, recovery bool) error {
	if c.ID == uuid.Nil || c.Copy.Validate() != nil || c.StartedAt.IsZero() {
		return ErrStorageScope
	}
	o := s.occupancy
	c.StartedAt = c.StartedAt.UTC().Truncate(time.Microsecond)
	return o.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rev, e := deliveryfence.Lock(ctx, tx, c.Copy.WorkspaceID)
		if e != nil {
			return e
		}
		op, e := s.lock(ctx, tx, c.Copy.WorkspaceID, c.CopyOperationID)
		if e != nil {
			return e
		}
		if c.CopyIntent == nil {
			return ErrStorageScope
		}
		intent := *c.CopyIntent
		if !intent.CreatedAt.Equal(op.CreatedAt) || !intent.ExpiresAt.Equal(op.ExpiresAt) {
			return ErrStorageScope
		}
		intent.CreatedAt = op.CreatedAt
		intent.ExpiresAt = op.ExpiresAt
		if intent != op {
			return ErrStorageScope
		}
		capability := CapabilityCopyCustody
		if recovery {
			capability = CapabilityReconcileCopy
		}
		caller, at, e := o.authorize(ctx, tx, op.WorkspaceID, capability, nil)
		if e != nil {
			return e
		}
		if recovery {
			if _, _, e = o.authorize(ctx, tx, op.WorkspaceID, CapabilityRestoreAdmission, &caller); e != nil {
				return e
			}
		}
		if at.Before(op.CreatedAt) || (!recovery && !at.Before(op.ExpiresAt)) || c.StartedAt.Before(op.CreatedAt) || c.StartedAt.After(at) {
			return ErrStorageDenied
		}
		if c.Operation == op.Action+"_COPY" {
			if recovery {
				return ErrStorageUnsupported
			}
			if c.Copy != op.Source {
				return ErrStorageScope
			}
			if e = s.custodyGate(ctx, tx, op.Source, at); e != nil {
				return e
			}
		} else if c.Operation == StorageVerifyCopy {
			var target uuid.UUID
			e = tx.QueryRow(ctx, `SELECT copy_id FROM lifecycle_copy_materialization WHERE operation_id=$1`, op.ID).Scan(&target)
			if e != nil || target != c.Copy.ID {
				return ErrStorageScope
			}
			cs, e := o.repo.LoadFixtureCopies(ctx, tx, op.WorkspaceID, []uuid.UUID{target})
			if e != nil || cs[0] != c.Copy {
				return ErrStorageScope
			}
		} else if recovery && (c.Operation == StorageRecoverCopyVersions || c.Operation == StorageRecoverCopyExact) {
			if c.Copy != op.Source {
				return ErrStorageScope
			}
		} else {
			return ErrStorageUnsupported
		}
		_, e = tx.Exec(ctx, `INSERT INTO lifecycle_storage_call(id,workspace_id,copy_id,operation,started_at,actor_id,principal_ref,dependency_revision,copy_operation_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, c.ID, c.Copy.WorkspaceID, c.Copy.ID, c.Operation, c.StartedAt, caller.ActorID, caller.Subject, rev, op.ID)
		if e != nil {
			return e
		}
		if c.Operation == op.Action+"_COPY" {
			if _, e = appendCopyRecoveryWork(ctx, tx, op, c.ID, "durable-copy-start", caller, rev, at); e != nil {
				return e
			}
		}
		return appendDispositionFacts(ctx, tx, StorageCallStartedEvent, "LIFECYCLE_STORAGE_CALL", c.ID, caller, rev, at, map[string]any{"copy_id": c.Copy.ID, "operation": c.Operation, "copy_operation_id": op.ID})
	})
}

type BindMaterializedCommand struct {
	CommandHeader
	OperationID uuid.UUID
	Copy        domain.Copy
	CopyCallID  uuid.UUID
}

func (s *CopyService) BindMaterializedCopy(ctx context.Context, c BindMaterializedCommand) (FactResult, error) {
	if c.Copy.Validate() != nil || c.CopyCallID == uuid.Nil {
		return FactResult{}, ErrStorageScope
	}
	o := s.occupancy
	return o.command(ctx, c.CommandHeader, "LIFECYCLE.BIND_MATERIALIZED_COPY", CapabilityCopyCustody, c, func(ctx context.Context, tx pgx.Tx, caller principal.RequestPrincipal, _ int64) (uuid.UUID, error) {
		op, e := s.lock(ctx, tx, c.WorkspaceID, c.OperationID)
		if e != nil {
			return uuid.Nil, e
		}
		if op.Materialized(c.Copy.ProviderVersion) != c.Copy {
			return uuid.Nil, ErrStorageScope
		}
		_, at, e := o.authorize(ctx, tx, c.WorkspaceID, CapabilityCopyCustody, &caller)
		if e != nil {
			return uuid.Nil, e
		}
		_, e = tx.Exec(ctx, `INSERT INTO lifecycle_fixture_copy(id,workspace_id,namespace_id,dataset_version_id,object_key,provider_version,content_sha256,byte_size) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, c.Copy.ID, c.WorkspaceID, op.TargetNamespaceID, c.Copy.DatasetVersionID, c.Copy.Key, c.Copy.ProviderVersion, c.Copy.ContentSHA256, c.Copy.ByteSize)
		if e != nil {
			return uuid.Nil, e
		}
		_, e = tx.Exec(ctx, `INSERT INTO lifecycle_copy_materialization(operation_id,workspace_id,copy_id,copy_call_id,observed_at) VALUES($1,$2,$3,$4,$5)`, op.ID, c.WorkspaceID, c.Copy.ID, c.CopyCallID, at)
		if e != nil {
			return uuid.Nil, e
		}
		rev, e := deliveryfence.Advance(ctx, tx, c.WorkspaceID)
		if e != nil {
			return uuid.Nil, e
		}
		return op.ID, appendDispositionFacts(ctx, tx, "LifecycleCopyMaterialized", "LIFECYCLE_COPY_OPERATION", op.ID, caller, rev, at, map[string]any{"target_copy_id": c.Copy.ID})
	})
}

type VerifyCopyCommand struct {
	CommandHeader
	OperationID uuid.UUID
	Proof       CopyBytesProof
}

func (s *CopyService) VerifyMaterializedCopy(ctx context.Context, c VerifyCopyCommand) (FactResult, error) {
	return s.verifyMaterializedCopy(ctx, c, CapabilityCopyCustody)
}
func (s *CopyService) verifyMaterializedCopy(ctx context.Context, c VerifyCopyCommand, capability string) (FactResult, error) {
	o := s.occupancy
	return o.command(ctx, c.CommandHeader, "LIFECYCLE.VERIFY_COPY", capability, c, func(ctx context.Context, tx pgx.Tx, caller principal.RequestPrincipal, _ int64) (uuid.UUID, error) {
		op, e := s.lock(ctx, tx, c.WorkspaceID, c.OperationID)
		if e != nil {
			return uuid.Nil, e
		}
		p := c.Proof
		if p.Copy != op.Materialized(p.Copy.ProviderVersion) || p.SHA256 != op.Source.ContentSHA256 || p.Bytes != op.Source.ByteSize {
			return uuid.Nil, ErrStorageContract
		}
		_, at, e := o.authorize(ctx, tx, c.WorkspaceID, capability, &caller)
		if e != nil {
			return uuid.Nil, e
		}
		if p.ObservedAt.After(at) || p.ObservedAt.Before(op.CreatedAt) {
			return uuid.Nil, ErrStorageContract
		}
		protection, e := s.protection.CurrentCopyProtection(ctx, tx, p.Copy, at)
		if e != nil {
			return uuid.Nil, e
		}
		if e = unprotectedFresh(protection, p.Copy, at); e != nil {
			return uuid.Nil, e
		}
		_, e = tx.Exec(ctx, `INSERT INTO lifecycle_copy_verification(operation_id,workspace_id,copy_id,verify_call_id,content_sha256,byte_size,observed_at,protection_checked_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, op.ID, c.WorkspaceID, p.Copy.ID, p.CallID, p.SHA256, p.Bytes, at, protection.ObservedAt)
		if e != nil {
			return uuid.Nil, e
		}
		rev, e := deliveryfence.Advance(ctx, tx, c.WorkspaceID)
		if e != nil {
			return uuid.Nil, e
		}
		return op.ID, appendDispositionFacts(ctx, tx, "LifecycleCopyVerified", "LIFECYCLE_COPY_OPERATION", op.ID, caller, rev, at, map[string]any{"target_copy_id": p.Copy.ID, "content_sha256": p.SHA256})
	})
}

// Current typed facts are hashed rather than accepting an old backup's flags.
// Identity/rights delivery is outside this fixture-only custody/use policy.
func (s *CopyService) currentState(ctx context.Context, tx pgx.Tx, w, v uuid.UUID) (string, bool, error) {
	var status, hash string
	var size int64
	var use, restore time.Time
	var restrictions, dispositions, holds string
	e := tx.QueryRow(ctx, `SELECT v.status,v.checksum_value,v.byte_size,p.use_until,p.restore_until,
 COALESCE((SELECT string_agg(r.id::text,',' ORDER BY r.id) FROM lifecycle_content_use_restriction r WHERE r.workspace_id=$1 AND r.dataset_version_id=v.id),''),
 COALESCE((SELECT string_agg(a.id::text,',' ORDER BY a.id) FROM lifecycle_disposition_approval a JOIN lifecycle_disposition_plan_target t ON t.plan_id=a.plan_id JOIN lifecycle_fixture_copy c ON c.id=t.copy_id WHERE c.dataset_version_id=v.id AND NOT EXISTS(SELECT 1 FROM lifecycle_approval_revocation r WHERE r.approval_id=a.id)),''),
 COALESCE((SELECT string_agg(h.id::text,',' ORDER BY h.id) FROM lifecycle_content_hold h JOIN lifecycle_fixture_copy c ON c.id=h.copy_id WHERE c.dataset_version_id=v.id AND NOT EXISTS(SELECT 1 FROM lifecycle_content_hold_release r WHERE r.hold_id=h.id)),'')
 FROM dataset_version v JOIN dataset d ON d.id=v.dataset_id JOIN lifecycle_copy_policy p ON p.dataset_version_id=v.id AND p.workspace_id=d.workspace_id WHERE d.workspace_id=$1 AND v.id=$2`, w, v).Scan(&status, &hash, &size, &use, &restore, &restrictions, &dispositions, &holds)
	if e != nil {
		return "", false, e
	}
	b, _ := json.Marshal([]any{w, v, status, hash, size, use.UTC(), restore.UTC(), restrictions, dispositions, holds})
	sum := sha256.Sum256(b)
	now := s.occupancy.now().UTC()
	allowed := status == "READY" && restrictions == "" && dispositions == "" && holds == "" && now.Before(use) && now.Before(restore)
	return hex.EncodeToString(sum[:]), allowed, nil
}

type ExportRestoreCheckpointCommand struct {
	CommandHeader
	DatasetVersionID uuid.UUID
}

// The publisher must retain this command key until its independent append has
// succeeded. Replaying the command recovers the original immutable cut even if
// the process stopped after the Core commit, without allocating a chain gap.
func (s *CopyService) ExportRestoreCheckpoint(ctx context.Context, cmd ExportRestoreCheckpointCommand) (domain.RestoreCheckpoint, error) {
	var c domain.RestoreCheckpoint
	if s == nil || s.occupancy == nil || cmd.DatasetVersionID == uuid.Nil || s.epoch == uuid.Nil {
		return c, domain.ErrRestoreLedger
	}
	w, v := cmd.WorkspaceID, cmd.DatasetVersionID
	o := s.occupancy
	body := struct {
		Command ExportRestoreCheckpointCommand
		Epoch   uuid.UUID
	}{cmd, s.epoch}
	r, e := o.command(ctx, cmd.CommandHeader, "LIFECYCLE.EXPORT_RESTORE_CHECKPOINT", CapabilityRestoreAdmission, body, func(ctx context.Context, tx pgx.Tx, caller principal.RequestPrincipal, rev int64) (uuid.UUID, error) {
		ids, e := o.repo.FixtureCopyIDsForVersion(ctx, tx, w, v)
		if e != nil {
			return uuid.Nil, e
		}
		if _, e = o.repo.LockFixtureCopies(ctx, tx, w, ids); e != nil {
			return uuid.Nil, e
		}
		caller, at, e := o.authorize(ctx, tx, w, CapabilityRestoreAdmission, &caller)
		if e != nil {
			return uuid.Nil, e
		}
		state, _, e := s.currentState(ctx, tx, w, v)
		if e != nil {
			return uuid.Nil, e
		}
		var n int64
		previous := strings.Repeat("0", 64)
		e = tx.QueryRow(ctx, `SELECT sequence,content_sha256 FROM lifecycle_restore_checkpoint WHERE workspace_id=$1 AND dataset_version_id=$2 ORDER BY sequence DESC LIMIT 1`, w, v).Scan(&n, &previous)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return uuid.Nil, e
		}
		c = domain.RestoreCheckpoint{ID: uuid.New(), WorkspaceID: w, DatasetVersionID: v, Epoch: s.epoch, Sequence: n + 1, DependencyRevision: rev, StateSHA256: state, PreviousSHA256: previous, CreatedAt: at.UTC().Truncate(time.Microsecond), ExpiresAt: at.Add(time.Minute).UTC().Truncate(time.Microsecond)}
		c.ContentSHA256 = c.Hash()
		_, e = tx.Exec(ctx, `INSERT INTO lifecycle_restore_checkpoint(id,workspace_id,dataset_version_id,epoch,sequence,dependency_revision,state_sha256,previous_sha256,content_sha256,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, c.ID, w, v, c.Epoch, c.Sequence, rev, state, previous, c.ContentSHA256, c.CreatedAt, c.ExpiresAt)
		if e != nil {
			return uuid.Nil, e
		}
		return c.ID, appendDispositionFacts(ctx, tx, "LifecycleRestoreCheckpointExported", "LIFECYCLE_RESTORE_CHECKPOINT", c.ID, caller, rev, at, map[string]any{"sequence": c.Sequence, "dataset_version_id": v})
	})
	if e != nil {
		return domain.RestoreCheckpoint{}, e
	}
	e = o.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id,workspace_id,dataset_version_id,epoch,sequence,dependency_revision,state_sha256,previous_sha256,content_sha256,created_at,expires_at FROM lifecycle_restore_checkpoint WHERE id=$1 AND workspace_id=$2 AND dataset_version_id=$3 AND epoch=$4`, r.ID, w, v, s.epoch).Scan(&c.ID, &c.WorkspaceID, &c.DatasetVersionID, &c.Epoch, &c.Sequence, &c.DependencyRevision, &c.StateSHA256, &c.PreviousSHA256, &c.ContentSHA256, &c.CreatedAt, &c.ExpiresAt)
	})
	c.CreatedAt, c.ExpiresAt = c.CreatedAt.UTC(), c.ExpiresAt.UTC()
	return c, e
}

type FinalizeCopyCommand struct {
	CommandHeader
	OperationID uuid.UUID
}
type CopyDecision struct {
	ID               uuid.UUID
	Decision, Reason string
	TargetCopyID     uuid.UUID
}

// Only this closed fixture policy is supported; ONLINE is availability, not a
// production delivery/rights/certification permission. Every read gets a new pin.
func (s *CopyService) EvaluateFixtureUse(ctx context.Context, tx pgx.Tx, r UseGateRequest) (domain.UseDecision, error) {
	_, allowed, e := s.currentState(ctx, tx, r.Copy.WorkspaceID, r.Copy.DatasetVersionID)
	if e != nil {
		return domain.UseDecision{}, e
	}
	if !allowed {
		return domain.UseDecision{}, domain.ErrUseBlocked
	}
	var until time.Time
	if e = tx.QueryRow(ctx, `SELECT use_until FROM lifecycle_copy_policy WHERE workspace_id=$1 AND dataset_version_id=$2`, r.Copy.WorkspaceID, r.Copy.DatasetVersionID).Scan(&until); e != nil {
		return domain.UseDecision{}, e
	}
	// The output intent freezes the actual producer inputs before publication.
	// A derived fixture copy cannot outlive a currently restricted/expired input.
	rows, e := tx.Query(ctx, `WITH RECURSIVE deps(id,path,cycle) AS (
 SELECT $1::uuid,ARRAY[$1::uuid],false
 UNION ALL
 SELECT src.id,d.path||src.id,src.id=ANY(d.path) FROM deps d
 JOIN lifecycle_processing_output_intent i ON i.dataset_version_id=d.id
 CROSS JOIN LATERAL (
 SELECT x.dataset_version_id AS id FROM execution_input x WHERE x.execution_id=i.execution_id
 UNION SELECT c.dataset_version_id FROM lifecycle_fixture_copy c WHERE c.id=i.source_copy_id AND i.entity_match_job_id IS NOT NULL
 ) src WHERE NOT d.cycle AND cardinality(d.path)<18
) SELECT DISTINCT id,cycle,cardinality(path)>=18 FROM deps`, r.Copy.DatasetVersionID)
	if e != nil {
		return domain.UseDecision{}, e
	}
	var sources []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		var cycle, deep bool
		if e = rows.Scan(&id, &cycle, &deep); e != nil {
			rows.Close()
			return domain.UseDecision{}, e
		}
		if cycle || deep {
			rows.Close()
			return domain.UseDecision{}, domain.ErrUseBlocked
		}
		sources = append(sources, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return domain.UseDecision{}, e
	}
	for _, id := range sources {
		if id == r.Copy.DatasetVersionID {
			continue
		}
		_, allowed, e := s.currentState(ctx, tx, r.Copy.WorkspaceID, id)
		if e != nil {
			return domain.UseDecision{}, e
		}
		if !allowed {
			return domain.UseDecision{}, domain.ErrUseBlocked
		}
		var cap time.Time
		if e = tx.QueryRow(ctx, `SELECT use_until FROM lifecycle_copy_policy WHERE workspace_id=$1 AND dataset_version_id=$2`, r.Copy.WorkspaceID, id).Scan(&cap); e != nil {
			return domain.UseDecision{}, e
		}
		if cap.Before(until) {
			until = cap
		}
	}
	return domain.UseDecision{WorkspaceID: r.Copy.WorkspaceID, CopyID: r.Copy.ID, ReaderID: r.ReaderID, ActorID: r.Caller.ActorID, CheckedAt: r.AsOf, Decision: domain.Satisfied, WindowMode: domain.UseDeadline, UseUntil: &until}, nil
}

func (s *CopyService) FinalizeCopy(ctx context.Context, c FinalizeCopyCommand) (CopyDecision, error) {
	return s.finalizeCopy(ctx, c, false)
}
func (s *CopyService) finalizeCopy(ctx context.Context, c FinalizeCopyCommand, recovery bool) (CopyDecision, error) {
	// Independent I/O happens before the short Core transaction, never under locks.
	var cut domain.RestoreCheckpoint
	ledgerErr := domain.ErrRestoreLedger
	if s.ledger != nil {
		cut, ledgerErr = s.ledger.Latest(ctx)
	}
	o := s.occupancy
	r, e := o.command(ctx, c.CommandHeader, "LIFECYCLE.FINALIZE_COPY", CapabilityRestoreAdmission, c, func(ctx context.Context, tx pgx.Tx, caller principal.RequestPrincipal, _ int64) (uuid.UUID, error) {
		op, e := s.lock(ctx, tx, c.WorkspaceID, c.OperationID)
		if e != nil {
			return uuid.Nil, e
		}
		caller, at, e := o.authorize(ctx, tx, c.WorkspaceID, CapabilityRestoreAdmission, &caller)
		if e != nil {
			return uuid.Nil, e
		}
		if recovery {
			if _, _, e = o.authorize(ctx, tx, c.WorkspaceID, CapabilityReconcileCopy, &caller); e != nil {
				return uuid.Nil, e
			}
		}
		var verified bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_copy_verification WHERE operation_id=$1)`, op.ID).Scan(&verified)
		if e != nil {
			return uuid.Nil, e
		}
		decision, reason := "UNKNOWN", "COPY_UNVERIFIED"
		var checkpoint *uuid.UUID
		rev, e := deliveryfence.Lock(ctx, tx, c.WorkspaceID)
		if e != nil {
			return uuid.Nil, e
		}
		if verified && at.Before(op.ExpiresAt) {
			if op.Action == domain.ArchiveCopy {
				decision, reason = "ARCHIVED", "VERIFIED_SOURCE_RETAINED"
			} else {
				decision, reason = "UNKNOWN", "INDEPENDENT_LEDGER_UNAVAILABLE"
				state, allowed, e := s.currentState(ctx, tx, c.WorkspaceID, op.Source.DatasetVersionID)
				if e != nil {
					return uuid.Nil, e
				}
				var id uuid.UUID
				var root string
				var seq, revision int64
				e = tx.QueryRow(ctx, `SELECT id,sequence,dependency_revision,content_sha256 FROM lifecycle_restore_checkpoint WHERE workspace_id=$1 AND dataset_version_id=$2 ORDER BY sequence DESC LIMIT 1`, c.WorkspaceID, op.Source.DatasetVersionID).Scan(&id, &seq, &revision, &root)
				if ledgerErr == nil && e == nil && cut.Validate(at) == nil && cut.Epoch == s.epoch && cut.WorkspaceID == c.WorkspaceID && cut.DatasetVersionID == op.Source.DatasetVersionID && cut.ID == id && cut.Sequence == seq && cut.DependencyRevision == revision && revision == rev && cut.ContentSHA256 == root && cut.StateSHA256 == state {
					checkpoint = &id
					if allowed {
						decision, reason = "ONLINE", "CURRENT_LEDGER_AND_POLICY_ALLOWED"
					} else {
						decision, reason = "BLOCKED", "CURRENT_USE_OR_RESTORE_PROHIBITED"
					}
				} else if ledgerErr == nil {
					reason = "LEDGER_GAP_STALE_OR_CONTEXT_MISMATCH"
				}
			}
		} else if !at.Before(op.ExpiresAt) {
			decision, reason = "BLOCKED", "COPY_AUTHORITY_WINDOW_EXPIRED"
		}
		if decision == "ARCHIVED" || decision == "ONLINE" {
			var version string
			if e = tx.QueryRow(ctx, `SELECT c.provider_version FROM lifecycle_fixture_copy c WHERE c.id=$1`, op.TargetCopyID).Scan(&version); e != nil {
				return uuid.Nil, e
			}
			protection, checkErr := s.protection.CurrentCopyProtection(ctx, tx, op.Materialized(version), at)
			if checkErr != nil || unprotectedFresh(protection, op.Materialized(version), at) != nil || s.custodyGate(ctx, tx, op.Source, at) != nil {
				decision, reason = "BLOCKED", "CURRENT_COPY_PROTECTION_OR_OCCUPANCY"
			}
		}
		rev, e = deliveryfence.Advance(ctx, tx, c.WorkspaceID)
		if e != nil {
			return uuid.Nil, e
		}
		id := uuid.New()
		_, e = tx.Exec(ctx, `INSERT INTO lifecycle_copy_decision(id,workspace_id,operation_id,checkpoint_id,decision,reason,actor_id,principal_ref,dependency_revision,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id, c.WorkspaceID, op.ID, checkpoint, decision, reason, caller.ActorID, caller.Subject, rev, at)
		if e != nil {
			return uuid.Nil, e
		}
		return id, appendDispositionFacts(ctx, tx, "LifecycleCopyAdmissionEvaluated", "LIFECYCLE_COPY_DECISION", id, caller, rev, at, map[string]any{"operation_id": op.ID, "decision": decision, "reason": reason})
	})
	if e != nil {
		return CopyDecision{}, e
	}
	var d CopyDecision
	d.ID = r.ID
	e = o.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT d.decision,d.reason,o.target_copy_id FROM lifecycle_copy_decision d JOIN lifecycle_copy_operation o ON o.id=d.operation_id WHERE d.id=$1 AND d.workspace_id=$2`, r.ID, c.WorkspaceID).Scan(&d.Decision, &d.Reason, &d.TargetCopyID)
	})
	return d, e
}
