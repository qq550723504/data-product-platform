package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/infrastructure"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type copyTestProtection struct{}

func (copyTestProtection) CurrentCopyProtection(_ context.Context, _ pgx.Tx, c domain.Copy, at time.Time) (CopyProtection, error) {
	return CopyProtection{Copy: c, RetentionMode: "NONE", ObservedAt: at.Add(-time.Millisecond)}, nil
}
func newCopyFixture(t *testing.T) (*pgxpool.Pool, uuid.UUID, []domain.Copy, *CopyService, *infrastructure.FileRestoreLedger, *occupancyAuthority) {
	t.Helper()
	pool := planDatabase(t)
	w, cs, o, a, _, _, _ := occupancyFixture(t, pool)
	for _, cap := range []string{CapabilityCopyCustody, CapabilityRestoreAdmission, CapabilityInspectStorage} {
		a.caller.Capabilities[cap] = struct{}{}
	}
	var ns uuid.UUID
	if e := pool.QueryRow(context.Background(), `SELECT namespace_id FROM lifecycle_fixture_copy WHERE id=$1`, cs[0].ID).Scan(&ns); e != nil {
		t.Fatal(e)
	}
	epoch := uuid.New()
	ledger := &infrastructure.FileRestoreLedger{Path: filepath.Join(t.TempDir(), "independent.jsonl"), WorkspaceID: w, DatasetVersionID: cs[0].DatasetVersionID, Epoch: epoch}
	slots := []CopyTargetSlot{{Slot: "archive", Action: domain.ArchiveCopy, Namespace: cs[0].Namespace, NamespaceID: ns, Key: "archive/copy.csv"}}
	for _, name := range []string{"restore", "stale", "missing", "gap", "late"} {
		slots = append(slots, CopyTargetSlot{Slot: name, Action: domain.RestoreCopy, Namespace: cs[0].Namespace, NamespaceID: ns, Key: "quarantine/" + name + ".csv"})
	}
	s := NewCopyService(o, slots, copyTestProtection{}, ledger, epoch)
	_, e := s.BindFixtureCopyPolicy(context.Background(), BindCopyPolicyCommand{CommandHeader: CommandHeader{w, "policy"}, DatasetVersionID: cs[0].DatasetVersionID, UseUntil: time.Now().Add(15 * time.Minute), RestoreUntil: time.Now().Add(15 * time.Minute)})
	if e != nil {
		t.Fatal(e)
	}
	return pool, w, cs, s, ledger, a
}
func stagedCopy(t *testing.T, s *CopyService, w, source uuid.UUID, slot string) domain.CopyOperation {
	t.Helper()
	ctx := context.Background()
	o, e := s.PrepareCopy(ctx, PrepareCopyCommand{CommandHeader: CommandHeader{w, "prepare-" + slot}, SourceCopyID: source, TargetSlot: slot, BasisRef: "controlled-synthetic", ExpiresAt: time.Now().Add(5 * time.Minute)})
	if e != nil {
		t.Fatal(e)
	}
	j := CopyCallJournal{s, NewPostgresStorageCallJournal(s.occupancy)}
	call := StorageCallStart{ID: uuid.New(), Copy: o.Source, Operation: o.Action + "_COPY", StartedAt: time.Now(), CopyOperationID: o.ID, CopyIntent: &o}
	if e = j.StartStorageCall(ctx, call); e != nil {
		t.Fatal(e)
	}
	at := time.Now()
	if e = j.ObserveStorageCall(ctx, StorageCallObservation{CallID: call.ID, ObservedAt: at, InvokedAt: &at, HTTPStatus: 200, ResponseSHA256: strings.Repeat("b", 64), CopyResultVersion: "synthetic-copy-" + slot}); e != nil {
		t.Fatal(e)
	}
	c := o.Materialized("synthetic-copy-" + slot)
	if _, e = s.BindMaterializedCopy(ctx, BindMaterializedCommand{CommandHeader: CommandHeader{w, "bind-" + slot}, OperationID: o.ID, Copy: c, CopyCallID: call.ID}); e != nil {
		t.Fatal(e)
	}
	verify := StorageCallStart{ID: uuid.New(), Copy: c, Operation: StorageVerifyCopy, StartedAt: time.Now(), CopyOperationID: o.ID, CopyIntent: &o}
	if e = j.StartStorageCall(ctx, verify); e != nil {
		t.Fatal(e)
	}
	at = time.Now()
	if e = j.ObserveStorageCall(ctx, StorageCallObservation{CallID: verify.ID, ObservedAt: at, InvokedAt: &at, HTTPStatus: 200, ResponseSHA256: c.ContentSHA256}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.VerifyMaterializedCopy(ctx, VerifyCopyCommand{CommandHeader: CommandHeader{w, "verify-" + slot}, OperationID: o.ID, Proof: CopyBytesProof{Copy: c, CallID: verify.ID, SHA256: c.ContentSHA256, Bytes: c.ByteSize, ObservedAt: time.Now()}}); e != nil {
		t.Fatal(e)
	}
	return o
}
func TestArchiveRestoreCurrentWitnessAndRestrictions(t *testing.T) {
	pool, w, cs, s, ledger, _ := newCopyFixture(t)
	ctx := context.Background()
	archive := stagedCopy(t, s, w, cs[0].ID, "archive")
	d, e := s.FinalizeCopy(ctx, FinalizeCopyCommand{CommandHeader: CommandHeader{w, "archive-final"}, OperationID: archive.ID})
	if e != nil || d.Decision != "ARCHIVED" {
		t.Fatal(d, e)
	}
	if _, e = s.occupancy.AdmitFixtureReader(ctx, AdmitFixtureReaderCommand{CommandHeader: CommandHeader{w, "verified-archive-read"}, CopyID: archive.TargetCopyID, ReaderID: uuid.New()}); !errors.Is(e, domain.ErrUseBlocked) {
		t.Fatal("verified ARCHIVED bytes bypassed restore admission", e)
	}
	restore := stagedCopy(t, s, w, archive.TargetCopyID, "restore")
	cut, e := s.ExportRestoreCheckpoint(ctx, ExportRestoreCheckpointCommand{CommandHeader: CommandHeader{w, "cut-1"}, DatasetVersionID: cs[0].DatasetVersionID})
	if e != nil {
		t.Fatal(e)
	}
	if e = ledger.Append(cut); e != nil {
		t.Fatal(e)
	}
	d, e = s.FinalizeCopy(ctx, FinalizeCopyCommand{CommandHeader: CommandHeader{w, "restore-final"}, OperationID: restore.ID})
	if e != nil || d.Decision != "ONLINE" {
		t.Fatal(d, e)
	}
	if count(t, pool, `SELECT count(*) FROM dataset_version WHERE id=$1 AND checksum_value=repeat('a',64) AND storage_uri='s3://no-provider-configured/sql-only.csv'`, cs[0].DatasetVersionID) != 1 {
		t.Fatal("identity changed")
	}
	if count(t, pool, `SELECT count(*) FROM cost_allocation WHERE lifecycle_storage_call_id IS NOT NULL`) != 4 {
		t.Fatal("physical copy/verify costs missing")
	}
	stale := stagedCopy(t, s, w, archive.TargetCopyID, "stale")
	_, e = s.occupancy.RestrictContentUse(ctx, RestrictContentUseCommand{CommandHeader: CommandHeader{w, "new-restriction"}, DatasetVersionID: cs[0].DatasetVersionID, BasisRef: "new-current-fact-after-old-archive-cut"})
	if e != nil {
		t.Fatal(e)
	}
	d, e = s.FinalizeCopy(ctx, FinalizeCopyCommand{CommandHeader: CommandHeader{w, "stale-final"}, OperationID: stale.ID})
	if e != nil || d.Decision != "UNKNOWN" {
		t.Fatal("old witness admitted", d, e)
	}
	cut, e = s.ExportRestoreCheckpoint(ctx, ExportRestoreCheckpointCommand{CommandHeader: CommandHeader{w, "cut-2"}, DatasetVersionID: cs[0].DatasetVersionID})
	if e != nil {
		t.Fatal(e)
	}
	if e = ledger.Append(cut); e != nil {
		t.Fatal(e)
	}
	d, e = s.FinalizeCopy(ctx, FinalizeCopyCommand{CommandHeader: CommandHeader{w, "current-final"}, OperationID: stale.ID})
	if e != nil || d.Decision != "BLOCKED" {
		t.Fatal("old content reopened despite current prohibition", d, e)
	}
	for _, table := range []string{"lifecycle_copy_operation", "lifecycle_copy_materialization", "lifecycle_copy_verification", "lifecycle_restore_checkpoint", "lifecycle_copy_decision"} {
		if _, e = pool.Exec(ctx, `UPDATE `+table+` SET workspace_id=workspace_id`); e == nil {
			t.Fatal("mutable history", table)
		}
		if _, e = pool.Exec(ctx, `TRUNCATE `+table+` CASCADE`); e == nil {
			t.Fatal("history truncate", table)
		}
	}
}
func TestCopyClaimRacePendingAndNoUnverifiedAdmission(t *testing.T) {
	pool, w, cs, s, _, a := newCopyFixture(t)
	ctx := context.Background()
	o, e := s.PrepareCopy(ctx, PrepareCopyCommand{CommandHeader: CommandHeader{w, "pending"}, SourceCopyID: cs[0].ID, TargetSlot: "archive", BasisRef: "synthetic", ExpiresAt: time.Now().Add(time.Minute)})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.occupancy.AdmitFixtureReader(ctx, AdmitFixtureReaderCommand{CommandHeader: CommandHeader{w, "pending-read"}, CopyID: cs[0].ID, ReaderID: uuid.New()}); e == nil {
		t.Fatal("pending copy omitted from reader fence")
	}
	j := CopyCallJournal{s, NewPostgresStorageCallJournal(s.occupancy)}
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errors <- j.StartStorageCall(ctx, StorageCallStart{ID: uuid.New(), Copy: cs[0], Operation: StorageArchiveCopy, StartedAt: time.Now(), CopyOperationID: o.ID, CopyIntent: &o})
		}()
	}
	wg.Wait()
	close(errors)
	success := 0
	for e := range errors {
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatal("double COPY send", success)
	}
	d, e := s.FinalizeCopy(ctx, FinalizeCopyCommand{CommandHeader: CommandHeader{w, "no-verify-final"}, OperationID: o.ID})
	if e != nil || d.Decision != "UNKNOWN" {
		t.Fatal("unverified opened", d, e)
	}
	a.denied.Store(true)
	var id uuid.UUID
	if e = pool.QueryRow(ctx, `SELECT id FROM lifecycle_storage_call WHERE copy_operation_id=$1`, o.ID).Scan(&id); e != nil {
		t.Fatal(e)
	}
	at := time.Now()
	if e = j.ObserveStorageCall(ctx, StorageCallObservation{CallID: id, ObservedAt: at, InvokedAt: &at, HTTPStatus: 503, ResponseSHA256: strings.Repeat("c", 64)}); e != nil {
		t.Fatal("revocation erased incurred cost", e)
	}
	if count(t, pool, `SELECT count(*) FROM cost_allocation WHERE lifecycle_storage_call_id=$1`, id) != 1 {
		t.Fatal("failed COPY cost missing")
	}
}

func TestCopyForgedIntentAndBytesCannotMaterialize(t *testing.T) {
	pool, w, cs, s, _, _ := newCopyFixture(t)
	ctx := context.Background()
	op, e := s.PrepareCopy(ctx, PrepareCopyCommand{CommandHeader: CommandHeader{w, "intent"}, SourceCopyID: cs[0].ID, TargetSlot: "archive", BasisRef: "synthetic", ExpiresAt: time.Now().Add(time.Minute)})
	if e != nil {
		t.Fatal(e)
	}
	j := CopyCallJournal{s, NewPostgresStorageCallJournal(s.occupancy)}
	forged := op
	forged.TargetKey = "foreign/key"
	if e = j.StartStorageCall(ctx, StorageCallStart{ID: uuid.New(), Copy: op.Source, Operation: StorageArchiveCopy, StartedAt: time.Now(), CopyOperationID: op.ID, CopyIntent: &forged}); !errors.Is(e, ErrStorageScope) {
		t.Fatal("forged intent accepted", e)
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_storage_call WHERE copy_operation_id=$1`, op.ID) != 0 {
		t.Fatal("forged send start survived rollback")
	}
	call := uuid.New()
	if e = j.StartStorageCall(ctx, StorageCallStart{ID: call, Copy: op.Source, Operation: StorageArchiveCopy, StartedAt: time.Now(), CopyOperationID: op.ID, CopyIntent: &op}); e != nil {
		t.Fatal(e)
	}
	at := time.Now()
	if e = j.ObserveStorageCall(ctx, StorageCallObservation{CallID: call, ObservedAt: at, InvokedAt: &at, HTTPStatus: 200, ResponseSHA256: strings.Repeat("b", 64), CopyResultVersion: "real-native-version"}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.BindMaterializedCopy(ctx, BindMaterializedCommand{CommandHeader: CommandHeader{w, "wrong-version"}, OperationID: op.ID, Copy: op.Materialized("invented-version"), CopyCallID: call}); e == nil {
		t.Fatal("unobserved version accepted")
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_fixture_copy WHERE id=$1`, op.TargetCopyID) != 0 {
		t.Fatal("unbacked registry row survived rollback")
	}
	target := op.Materialized("real-native-version")
	if _, e = s.BindMaterializedCopy(ctx, BindMaterializedCommand{CommandHeader: CommandHeader{w, "bind"}, OperationID: op.ID, Copy: target, CopyCallID: call}); e != nil {
		t.Fatal(e)
	}
	verify := uuid.New()
	if e = j.StartStorageCall(ctx, StorageCallStart{ID: verify, Copy: target, Operation: StorageVerifyCopy, StartedAt: time.Now(), CopyOperationID: op.ID, CopyIntent: &op}); e != nil {
		t.Fatal(e)
	}
	at = time.Now()
	if e = j.ObserveStorageCall(ctx, StorageCallObservation{CallID: verify, ObservedAt: at, InvokedAt: &at, HTTPStatus: 200, ResponseSHA256: strings.Repeat("c", 64)}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.VerifyMaterializedCopy(ctx, VerifyCopyCommand{CommandHeader: CommandHeader{w, "false-hash"}, OperationID: op.ID, Proof: CopyBytesProof{Copy: target, CallID: verify, SHA256: target.ContentSHA256, Bytes: target.ByteSize, ObservedAt: time.Now()}}); e == nil {
		t.Fatal("invented matching hash over corrupt GET admitted")
	}
	if _, e = s.occupancy.AdmitFixtureReader(ctx, AdmitFixtureReaderCommand{CommandHeader: CommandHeader{w, "quarantine-bypass"}, CopyID: target.ID, ReaderID: uuid.New()}); !errors.Is(e, domain.ErrUseBlocked) {
		t.Fatal("raw registry bypassed quarantine", e)
	}
}

func TestRestoreMissingGapAndWrongContextStayQuarantined(t *testing.T) {
	for _, fault := range []string{"missing", "gap", "foreign", "old-core"} {
		t.Run(fault, func(t *testing.T) {
			pool, w, cs, s, ledger, _ := newCopyFixture(t)
			ctx := context.Background()
			archive := stagedCopy(t, s, w, cs[0].ID, "archive")
			if _, e := s.FinalizeCopy(ctx, FinalizeCopyCommand{CommandHeader: CommandHeader{w, "archive-final"}, OperationID: archive.ID}); e != nil {
				t.Fatal(e)
			}
			restore := stagedCopy(t, s, w, archive.TargetCopyID, "restore")
			cut, e := s.ExportRestoreCheckpoint(ctx, ExportRestoreCheckpointCommand{CommandHeader: CommandHeader{w, "cut-3"}, DatasetVersionID: cs[0].DatasetVersionID})
			if e != nil {
				t.Fatal(e)
			}
			switch fault {
			case "missing":
			case "gap":
				cut.Sequence = 2
				cut.ContentSHA256 = cut.Hash()
				b, _ := json.Marshal(cut)
				if e = os.WriteFile(ledger.Path, append(b, '\n'), 0600); e != nil {
					t.Fatal(e)
				}
			case "foreign":
				cut.WorkspaceID = uuid.New()
				cut.ContentSHA256 = cut.Hash()
				b, _ := json.Marshal(cut)
				if e = os.WriteFile(ledger.Path, append(b, '\n'), 0600); e != nil {
					t.Fatal(e)
				}
			case "old-core":
				if e = ledger.Append(cut); e != nil {
					t.Fatal(e)
				}
				if _, e = s.ExportRestoreCheckpoint(ctx, ExportRestoreCheckpointCommand{CommandHeader: CommandHeader{w, "cut-4"}, DatasetVersionID: cs[0].DatasetVersionID}); e != nil {
					t.Fatal(e)
				}
			}
			d, e := s.FinalizeCopy(ctx, FinalizeCopyCommand{CommandHeader: CommandHeader{w, "restore-final"}, OperationID: restore.ID})
			if e != nil || d.Decision != "UNKNOWN" {
				t.Fatal("unsafe recovery opened", d, e)
			}
			if count(t, pool, `SELECT count(*) FROM lifecycle_copy_decision WHERE operation_id=$1 AND decision='ONLINE'`, restore.ID) != 0 {
				t.Fatal("ONLINE persisted")
			}
		})
	}
}

func TestRestoreFinalizeAndRestrictionSerializeBothOrders(t *testing.T) {
	for _, first := range []bool{true, false} {
		t.Run(fmt.Sprintf("finalize-first=%t", first), func(t *testing.T) {
			pool, w, cs, s, ledger, a := newCopyFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			archive := stagedCopy(t, s, w, cs[0].ID, "archive")
			if _, e := s.FinalizeCopy(ctx, FinalizeCopyCommand{CommandHeader: CommandHeader{w, "archive-final"}, OperationID: archive.ID}); e != nil {
				t.Fatal(e)
			}
			restore := stagedCopy(t, s, w, archive.TargetCopyID, "restore")
			cut, e := s.ExportRestoreCheckpoint(ctx, ExportRestoreCheckpointCommand{CommandHeader: CommandHeader{w, "cut-5"}, DatasetVersionID: cs[0].DatasetVersionID})
			if e != nil {
				t.Fatal(e)
			}
			if e = ledger.Append(cut); e != nil {
				t.Fatal(e)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			var calls atomic.Int64
			cap := CapabilityRestoreAdmission
			if !first {
				cap = CapabilityRestrictUse
			}
			a.hook = func(_ context.Context, _ pgx.Tx, c string) error {
				if c == cap && calls.Add(1) == 2 {
					close(entered)
					select {
					case <-release:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				return nil
			}
			decisions := make(chan CopyDecision, 1)
			finalErrors := make(chan error, 1)
			restricted := make(chan error, 1)
			finalize := func() {
				d, e := s.FinalizeCopy(ctx, FinalizeCopyCommand{CommandHeader: CommandHeader{w, "race-finalize"}, OperationID: restore.ID})
				decisions <- d
				finalErrors <- e
			}
			restrict := func() {
				_, e := s.occupancy.RestrictContentUse(ctx, RestrictContentUseCommand{CommandHeader: CommandHeader{w, "race-restrict"}, DatasetVersionID: cs[0].DatasetVersionID, BasisRef: "current"})
				restricted <- e
			}
			if first {
				go finalize()
			} else {
				go restrict()
			}
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal("first command failed to reach fence")
			}
			if first {
				go restrict()
			} else {
				go finalize()
			}
			waitForOccupancyFence(t, pool)
			close(release)
			if e = <-restricted; e != nil {
				t.Fatal(e)
			}
			d := <-decisions
			if e = <-finalErrors; e != nil {
				t.Fatal(e)
			}
			want := "ONLINE"
			if !first {
				want = "UNKNOWN"
			}
			if d.Decision != want {
				t.Fatal(d, want)
			}
			if _, e = s.occupancy.AdmitFixtureReader(ctx, AdmitFixtureReaderCommand{CommandHeader: CommandHeader{w, "late-reader"}, CopyID: restore.TargetCopyID, ReaderID: uuid.New()}); !errors.Is(e, domain.ErrUseBlocked) {
				t.Fatal("late restriction failed to stop reader", e)
			}
		})
	}
}

func TestCopyPreparationAndReaderSerializeBothOrders(t *testing.T) {
	for _, first := range []bool{true, false} {
		t.Run(fmt.Sprintf("copy-first=%t", first), func(t *testing.T) {
			pool, w, cs, s, _, a := newCopyFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			entered, release := make(chan struct{}), make(chan struct{})
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			var calls atomic.Int64
			cap := CapabilityCopyCustody
			if !first {
				cap = CapabilityAdmitReader
			}
			a.hook = func(_ context.Context, _ pgx.Tx, c string) error {
				if c == cap && calls.Add(1) == 2 {
					close(entered)
					select {
					case <-release:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				return nil
			}
			copyDone, readDone := make(chan error, 1), make(chan error, 1)
			copy := func() {
				_, e := s.PrepareCopy(ctx, PrepareCopyCommand{CommandHeader: CommandHeader{w, "racing-copy"}, SourceCopyID: cs[0].ID, TargetSlot: "archive", BasisRef: "synthetic", ExpiresAt: time.Now().Add(time.Minute)})
				copyDone <- e
			}
			read := func() {
				_, e := s.occupancy.AdmitFixtureReader(ctx, AdmitFixtureReaderCommand{CommandHeader: CommandHeader{w, "racing-reader"}, CopyID: cs[0].ID, ReaderID: uuid.New()})
				readDone <- e
			}
			if first {
				go copy()
			} else {
				go read()
			}
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal("first command did not enter")
			}
			if first {
				go read()
			} else {
				go copy()
			}
			waitForOccupancyFence(t, pool)
			close(release)
			ce, re := <-copyDone, <-readDone
			if first {
				if ce != nil || !errors.Is(re, domain.ErrUseBlocked) {
					t.Fatal(ce, re)
				}
			} else {
				if re != nil || !errors.Is(ce, ErrStorageDenied) {
					t.Fatal(ce, re)
				}
			}
		})
	}
}
