package application

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	"strings"
	"testing"
	"time"
)

// SQL-only provider fault inputs. These observations are not native acceptance.
type copyRecoveryFixturePort struct {
	journal                  StorageCallJournal
	missing, wrongTag, crash bool
	calls                    int
}

func (p *copyRecoveryFixturePort) call(ctx context.Context, op domain.CopyOperation, c domain.Copy, kind, version string) (uuid.UUID, error) {
	id := uuid.New()
	at := time.Now().UTC()
	start := StorageCallStart{ID: id, Copy: c, Operation: kind, StartedAt: at}
	if kind == StorageRecoverCopyVersions || kind == StorageRecoverCopyExact || kind == StorageVerifyCopy {
		start.CopyOperationID = op.ID
		start.CopyIntent = &op
	}
	if e := p.journal.StartStorageCall(ctx, start); e != nil {
		return uuid.Nil, e
	}
	p.calls++
	at = time.Now().UTC()
	obs := StorageCallObservation{CallID: id, ObservedAt: at, InvokedAt: &at, HTTPStatus: 200, ResponseSHA256: strings.Repeat("b", 64)}
	if kind == StorageRecoverCopyExact {
		obs.CopyResultVersion = version
		obs.CopyOperationTag = op.ID
		size := op.Source.ByteSize
		obs.CopyResultBytes = &size
		if p.wrongTag {
			obs.CopyOperationTag = uuid.New()
		}
	}
	if kind == StorageVerifyCopy {
		obs.ResponseSHA256 = c.ContentSHA256
	}
	return id, p.journal.ObserveStorageCall(ctx, obs)
}
func (p *copyRecoveryFixturePort) DiscoverCopyTarget(ctx context.Context, op domain.CopyOperation) (CopyDiscoveryProof, error) {
	v := CopyDiscoveryProof{Copy: op.Materialized("sql-only-recovered-" + op.ID.String()), OperationTag: op.ID}
	var e error
	v.InventoryCallID, e = p.call(ctx, op, op.Source, StorageRecoverCopyVersions, "")
	if e != nil {
		return v, e
	}
	if p.crash {
		panic("SQL-only interruption after durable native-shape inventory")
	}
	if p.missing {
		return v, ErrCopyTargetUnknown
	}
	v.StatCallID, e = p.call(ctx, op, op.Source, StorageRecoverCopyExact, v.Copy.ProviderVersion)
	v.ObservedAt = time.Now().UTC()
	return v, e
}
func (p *copyRecoveryFixturePort) VerifyCopyBytes(ctx context.Context, op domain.CopyOperation, c domain.Copy) (CopyBytesProof, error) {
	id, e := p.call(ctx, op, c, StorageVerifyCopy, "")
	return CopyBytesProof{Copy: c, CallID: id, SHA256: c.ContentSHA256, Bytes: c.ByteSize, ObservedAt: time.Now().UTC()}, e
}
func (p *copyRecoveryFixturePort) InspectRetentionAndHold(ctx context.Context, c domain.Copy) (CopyProtection, error) {
	for _, kind := range []string{StorageRetention, StorageLegalHold} {
		if _, e := p.call(ctx, domain.CopyOperation{}, c, kind, ""); e != nil {
			return CopyProtection{}, e
		}
	}
	return CopyProtection{Copy: c, RetentionMode: "NONE", ObservedAt: time.Now().UTC()}, nil
}
func unknownCopy(t *testing.T, pool *pgxpool.Pool, s *CopyService, w, source uuid.UUID, slot string) (domain.CopyOperation, uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	op, e := s.PrepareCopy(ctx, PrepareCopyCommand{CommandHeader: CommandHeader{w, "sql-unknown-" + slot}, SourceCopyID: source, TargetSlot: slot, BasisRef: "SQL_ONLY_RESPONSE_LOSS", ExpiresAt: time.Now().Add(5 * time.Minute)})
	if e != nil {
		t.Fatal(e)
	}
	id := uuid.New()
	at := time.Now().UTC()
	j := CopyCallJournal{s, NewPostgresStorageCallJournal(s.occupancy)}
	if e = j.StartStorageCall(ctx, StorageCallStart{ID: id, Copy: op.Source, Operation: op.Action + "_COPY", StartedAt: at, CopyOperationID: op.ID, CopyIntent: &op}); e != nil {
		t.Fatal(e)
	}
	at = time.Now().UTC()
	if e = j.ObserveStorageCall(ctx, StorageCallObservation{CallID: id, ObservedAt: at, InvokedAt: &at, Failure: "TRANSPORT"}); e != nil {
		t.Fatal(e)
	}
	var work uuid.UUID
	if e = pool.QueryRow(ctx, `SELECT id FROM lifecycle_reconciliation_work WHERE copy_call_id=$1`, id).Scan(&work); e != nil {
		t.Fatal(e)
	}
	return op, id, work
}
func copyRecoveryTestService(s *CopyService, pool *pgxpool.Pool, w uuid.UUID) (*CopyRecoveryService, *RecoveryService, *copyRecoveryFixturePort) {
	p := &copyRecoveryFixturePort{journal: CopyRecoveryCallJournal{s, NewCopyReconciliationMetadataJournal(s.occupancy)}}
	c := NewCopyRecoveryService(s, p)
	r := NewRecoveryService(NewExactExecution(NewDispositionService(s.occupancy, nil, nil, nil)), &recoveryInspector{}, transaction.NewManager(pool), nil, []uuid.UUID{w}).WithCopyRecovery(c)
	return c, r, p
}
func TestCopyRecoveryLostResponseAppendOnlyAndDuplicateNoIO(t *testing.T) {
	pool, w, cs, s, _, a := newCopyFixture(t)
	a.caller.Capabilities[CapabilityReconcileCopy] = struct{}{}
	op, call, work := unknownCopy(t, pool, s, w, cs[0].ID, "archive")
	_, r, p := copyRecoveryTestService(s, pool, w)
	delete(a.caller.Capabilities, CapabilityCopyCustody)
	ctx := context.Background()
	if count(t, pool, `SELECT count(*) FROM outbox_event WHERE aggregate_id=$1 AND required_handlers=ARRAY['lifecycle-recovery-queue']::text[]`, work) != 1 {
		t.Fatal("COPY start lacks atomic wake obligation")
	}
	if e := r.RunRecovery(ctx, w, work); e != nil {
		t.Fatal(e)
	}
	before := p.calls
	if e := r.RunRecovery(ctx, w, work); e != nil {
		t.Fatal(e)
	}
	if p.calls != before || before != 7 {
		t.Fatalf("unexpected physical calls: %d then %d", before, p.calls)
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_storage_call WHERE copy_operation_id=$1 AND operation='ARCHIVE_COPY'`, op.ID) != 1 || count(t, pool, `SELECT count(*) FROM lifecycle_storage_observation WHERE call_id=$1 AND failure='TRANSPORT' AND http_status=0 AND copy_result_version IS NULL`, call) != 1 {
		t.Fatal("original uncertainty overwritten or COPY resent")
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_reconciliation_result WHERE work_id=$1 AND outcome='COPY_ARCHIVED' AND terminal`, work) != 1 {
		t.Fatal("archive not reconciled")
	}
	if count(t, pool, `SELECT count(*) FROM cost_allocation WHERE lifecycle_storage_call_id IS NOT NULL`) != 8 {
		t.Fatal("physical invocation costs lost")
	}
	if _, e := pool.Exec(ctx, `UPDATE lifecycle_copy_discovery SET provider_version='rewritten'`); e == nil {
		t.Fatal("discovery mutable")
	}
}
func TestCopyMaterializationACKCannotHideForeignDiscovery(t *testing.T) {
	pool, w, cs, s, _, a := newCopyFixture(t)
	a.caller.Capabilities[CapabilityReconcileCopy] = struct{}{}
	archive, _, work := unknownCopy(t, pool, s, w, cs[0].ID, "archive")
	_, r, _ := copyRecoveryTestService(s, pool, w)
	ctx := context.Background()
	if e := r.RunRecovery(ctx, w, work); e != nil {
		t.Fatal(e)
	}
	// A second operation has a valid ordinary ACK and materialization. An
	// attempted insert must reject the first operation's discovery before its
	// uniqueness check; a successful ACK cannot legitimize foreign history.
	restore := stagedCopy(t, s, w, archive.TargetCopyID, "restore")
	_, e := pool.Exec(ctx, `INSERT INTO lifecycle_copy_materialization(workspace_id,operation_id,copy_id,copy_call_id,observed_at,discovery_id)
	 SELECT m.workspace_id,m.operation_id,m.copy_id,m.copy_call_id,m.observed_at,d.id
	 FROM lifecycle_copy_materialization m CROSS JOIN lifecycle_copy_discovery d
	 WHERE m.operation_id=$1 AND d.operation_id=$2`, restore.ID, archive.ID)
	if e == nil || !strings.Contains(e.Error(), "copy materialization discovery scope mismatch") {
		t.Fatalf("ACK hid foreign discovery, expected scope guard: %v", e)
	}
}

func TestCopyRecoveryMissingAmbiguousAndCrashNeverResend(t *testing.T) {
	for _, scenario := range []string{"missing", "wrong-tag", "crash"} {
		t.Run(scenario, func(t *testing.T) {
			pool, w, cs, s, _, a := newCopyFixture(t)
			a.caller.Capabilities[CapabilityReconcileCopy] = struct{}{}
			op, _, work := unknownCopy(t, pool, s, w, cs[0].ID, "archive")
			_, r, p := copyRecoveryTestService(s, pool, w)
			p.missing = scenario == "missing"
			p.wrongTag = scenario == "wrong-tag"
			p.crash = scenario == "crash"
			ctx := context.Background()
			if p.crash {
				func() {
					defer func() {
						if recover() == nil {
							t.Fatal("fault not reached")
						}
					}()
					_ = r.RunRecovery(ctx, w, work)
				}()
				p.crash = false
				if e := r.RunRecovery(ctx, w, work); e != nil {
					t.Fatal(e)
				}
			} else {
				e := r.RunRecovery(ctx, w, work)
				if scenario == "wrong-tag" && e == nil {
					t.Fatal("unbacked tagged HEAD bound")
				}
				if scenario == "missing" && !errors.Is(e, ErrCopyTargetUnknown) {
					t.Fatal(e)
				}
			}
			if count(t, pool, `SELECT count(*) FROM lifecycle_storage_call WHERE copy_operation_id=$1 AND operation='ARCHIVE_COPY'`, op.ID) != 1 {
				t.Fatal("recovery resent COPY")
			}
			if scenario != "crash" && count(t, pool, `SELECT count(*) FROM lifecycle_copy_materialization WHERE operation_id=$1`, op.ID) != 0 {
				t.Fatal("uncertain target materialized")
			}
		})
	}
}
func TestCopyRecoveryCurrentAuthorityStopsBeforeNativeIO(t *testing.T) {
	pool, w, cs, s, _, a := newCopyFixture(t)
	a.caller.Capabilities[CapabilityReconcileCopy] = struct{}{}
	_, _, work := unknownCopy(t, pool, s, w, cs[0].ID, "archive")
	_, r, p := copyRecoveryTestService(s, pool, w)
	delete(a.caller.Capabilities, CapabilityRestoreAdmission)
	if e := r.RunRecovery(context.Background(), w, work); e != nil {
		t.Fatal(e)
	}
	if p.calls != 0 || count(t, pool, `SELECT count(*) FROM lifecycle_reconciliation_result WHERE work_id=$1 AND outcome='BLOCKED_AUTHORITY'`, work) != 1 {
		t.Fatal("revoked restore admission granted I/O")
	}
}
func TestCopyRecoveryRestoreNeedsNewIndependentCutAndCurrentRestrictions(t *testing.T) {
	for _, restricted := range []bool{false, true} {
		t.Run(map[bool]string{false: "allowed", true: "restricted"}[restricted], func(t *testing.T) {
			pool, w, cs, s, ledger, a := newCopyFixture(t)
			a.caller.Capabilities[CapabilityReconcileCopy] = struct{}{}
			archive := stagedCopy(t, s, w, cs[0].ID, "archive")
			if d, e := s.FinalizeCopy(context.Background(), FinalizeCopyCommand{CommandHeader: CommandHeader{w, "archive-final"}, OperationID: archive.ID}); e != nil || d.Decision != "ARCHIVED" {
				t.Fatalf("archive prerequisite: %v %+v", e, d)
			}
			op, call, work := unknownCopy(t, pool, s, w, archive.TargetCopyID, "restore")
			cut, e := s.ExportRestoreCheckpoint(context.Background(), ExportRestoreCheckpointCommand{CommandHeader: CommandHeader{w, "cut-1"}, DatasetVersionID: cs[0].DatasetVersionID})
			if e != nil {
				t.Fatal(e)
			}
			if e = ledger.Append(cut); e != nil {
				t.Fatal(e)
			}
			c, r, p := copyRecoveryTestService(s, pool, w)
			if e = r.RunRecovery(context.Background(), w, work); e != nil {
				t.Fatal(e)
			}
			if count(t, pool, `SELECT count(*) FROM lifecycle_reconciliation_result WHERE work_id=$1 AND outcome='UNKNOWN_COPY_LEDGER'`, work) != 1 {
				t.Fatal("stale pre-materialization cut admitted restore")
			}
			if restricted {
				if _, e = s.occupancy.RestrictContentUse(context.Background(), RestrictContentUseCommand{CommandHeader: CommandHeader{w, "new-current-restriction"}, DatasetVersionID: cs[0].DatasetVersionID, BasisRef: "new-fact-after-original-copy"}); e != nil {
					t.Fatal(e)
				}
			}
			cut, e = s.ExportRestoreCheckpoint(context.Background(), ExportRestoreCheckpointCommand{CommandHeader: CommandHeader{w, "cut-2"}, DatasetVersionID: cs[0].DatasetVersionID})
			if e != nil {
				t.Fatal(e)
			}
			if e = ledger.Append(cut); e != nil {
				t.Fatal(e)
			}
			req, e := c.RequestRecovery(context.Background(), RequestCopyRecoveryCommand{CommandHeader: CommandHeader{w, "fresh-witness-request"}, CopyCallID: call, BasisRef: "independent-cut-now-current"})
			if e != nil {
				t.Fatal(e)
			}
			before := p.calls
			if e = r.RunRecovery(context.Background(), w, req.ID); e != nil {
				t.Fatal(e)
			}
			expected := "RESTORE_ONLINE"
			if restricted {
				expected = "BLOCKED_COPY"
			}
			if count(t, pool, `SELECT count(*) FROM lifecycle_reconciliation_result WHERE work_id=$1 AND outcome=$2`, req.ID, expected) != 1 {
				t.Fatal("fresh current facts not enforced")
			}
			if p.calls-before != 4 {
				t.Fatal("already materialized/verified restore should only refresh protection")
			}
			if count(t, pool, `SELECT count(*) FROM lifecycle_storage_call WHERE copy_operation_id=$1 AND operation='RESTORE_COPY'`, op.ID) != 1 {
				t.Fatal("restore resent")
			}
		})
	}
}
