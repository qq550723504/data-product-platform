package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
)

func TestCheckpointPublicationResumesCommittedCutWithoutNewSequence(t *testing.T) {
	pool, w, copies, s, ledger, authority := newCopyFixture(t)
	ctx := context.Background()
	cmd := ExportRestoreCheckpointCommand{CommandHeader: CommandHeader{w, "publication-1"}, DatasetVersionID: copies[0].DatasetVersionID}
	cut, e := s.ExportRestoreCheckpoint(ctx, cmd)
	if e != nil {
		t.Fatal(e)
	}
	// The Core commit succeeds, but the independent directory is unavailable.
	ledger.Path = filepath.Join(t.TempDir(), "temporarily-unavailable", "ledger.jsonl")
	if e = ledger.Append(cut); e == nil {
		t.Fatal("expected publication failure")
	}
	if e = os.MkdirAll(filepath.Dir(ledger.Path), 0700); e != nil {
		t.Fatal(e)
	}
	recovered, e := s.ExportRestoreCheckpoint(ctx, cmd)
	if e != nil || recovered.ID != cut.ID || recovered.Sequence != 1 || recovered.ContentSHA256 != cut.ContentSHA256 || recovered.ExpiresAt != cut.ExpiresAt {
		t.Fatal("replay must recover the committed cut without renewing it", recovered, e)
	}
	for i := 0; i < 2; i++ {
		if e = ledger.Append(recovered); e != nil {
			t.Fatal(e)
		}
	}
	latest, e := ledger.Latest(ctx)
	if e != nil || latest.ID != cut.ID {
		t.Fatal(latest, e)
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_restore_checkpoint WHERE workspace_id=$1`, w) != 1 || count(t, pool, `SELECT count(*) FROM outbox_event WHERE aggregate_id=$1 AND event_type='LifecycleRestoreCheckpointExported'`, cut.ID) != 1 {
		t.Fatal("publication retry duplicated Core facts/events")
	}
	conflict := cmd
	conflict.DatasetVersionID = uuid.New()
	if _, e = s.ExportRestoreCheckpoint(ctx, conflict); e != ErrIdempotencyConflict {
		t.Fatal("changed export scope must conflict", e)
	}
	authority.denied.Store(true)
	if _, e = s.ExportRestoreCheckpoint(ctx, cmd); !errors.Is(e, principal.ErrUnauthenticated) {
		t.Fatal("checkpoint replay bypassed current publisher authority", e)
	}
}

func TestConcurrentCheckpointKeysHaveRecoverablePublicationOrder(t *testing.T) {
	pool, w, copies, s, ledger, _ := newCopyFixture(t)
	ctx := context.Background()
	commands := []ExportRestoreCheckpointCommand{
		{CommandHeader: CommandHeader{w, "publication-a"}, DatasetVersionID: copies[0].DatasetVersionID},
		{CommandHeader: CommandHeader{w, "publication-b"}, DatasetVersionID: copies[0].DatasetVersionID},
	}
	cuts := make([]domain.RestoreCheckpoint, 2)
	errors := make([]error, 2)
	var done sync.WaitGroup
	for i := range commands {
		done.Add(1)
		go func(i int) {
			defer done.Done()
			cuts[i], errors[i] = s.ExportRestoreCheckpoint(ctx, commands[i])
		}(i)
	}
	done.Wait()
	for _, e := range errors {
		if e != nil {
			t.Fatal(e)
		}
	}
	first, second := 0, 1
	if cuts[first].Sequence != 1 {
		first, second = second, first
	}
	if cuts[second].Sequence != 2 || ledger.Append(cuts[second]) == nil {
		t.Fatal("a later cut cannot jump the unpublished predecessor")
	}
	if e := ledger.Append(cuts[first]); e != nil {
		t.Fatal(e)
	}
	replayed, e := s.ExportRestoreCheckpoint(ctx, commands[second])
	if e != nil || replayed.ID != cuts[second].ID || ledger.Append(replayed) != nil {
		t.Fatal("same-key retry must fill the original second sequence", replayed, e)
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_restore_checkpoint WHERE workspace_id=$1`, w) != 2 {
		t.Fatal("retry allocated another sequence")
	}
}

func TestRestoreCheckpointEpochChainsRemainIndependent(t *testing.T) {
	pool, w, copies, s, oldLedger, _ := newCopyFixture(t)
	ctx := context.Background()
	archive := stagedCopy(t, s, w, copies[0].ID, "archive")
	if d, e := s.FinalizeCopy(ctx, FinalizeCopyCommand{CommandHeader: CommandHeader{w, "epoch-archive-final"}, OperationID: archive.ID}); e != nil || d.Decision != "ARCHIVED" {
		t.Fatal(d, e)
	}
	restore := stagedCopy(t, s, w, archive.TargetCopyID, "restore")
	oldCommand := ExportRestoreCheckpointCommand{CommandHeader: CommandHeader{w, "old-epoch-first"}, DatasetVersionID: copies[0].DatasetVersionID}
	for _, key := range []string{oldCommand.IdempotencyKey, "old-epoch-second", "old-epoch-third"} {
		cut, e := s.ExportRestoreCheckpoint(ctx, ExportRestoreCheckpointCommand{CommandHeader: CommandHeader{w, key}, DatasetVersionID: copies[0].DatasetVersionID})
		if e != nil || oldLedger.Append(cut) != nil {
			t.Fatal("old epoch publication", cut, e)
		}
	}
	oldBytes, e := os.ReadFile(oldLedger.Path)
	if e != nil {
		t.Fatal(e)
	}
	var oldFacts string
	if e = pool.QueryRow(ctx, `SELECT string_agg(to_jsonb(c)::text, E'\n' ORDER BY sequence) FROM lifecycle_restore_checkpoint c WHERE workspace_id=$1 AND dataset_version_id=$2 AND epoch=$3`, w, copies[0].DatasetVersionID, s.epoch).Scan(&oldFacts); e != nil {
		t.Fatal(e)
	}

	// A new configured witness uses a distinct file and epoch for the same asset.
	// It must not append to, renumber or reinterpret the old immutable chain.
	next := *s
	next.epoch = uuid.New()
	ledger := &infrastructure.FileRestoreLedger{Path: filepath.Join(t.TempDir(), "new-epoch.jsonl"), WorkspaceID: w, DatasetVersionID: copies[0].DatasetVersionID, Epoch: next.epoch}
	next.ledger = ledger
	if _, e = next.ExportRestoreCheckpoint(ctx, oldCommand); !errors.Is(e, ErrIdempotencyConflict) {
		t.Fatal("same publication key silently changed epoch", e)
	}
	cmd := ExportRestoreCheckpointCommand{CommandHeader: CommandHeader{w, "new-epoch-first"}, DatasetVersionID: copies[0].DatasetVersionID}
	first, e := next.ExportRestoreCheckpoint(ctx, cmd)
	if e != nil || first.Sequence != 1 || first.PreviousSHA256 != strings.Repeat("0", 64) {
		t.Fatal("new epoch must start an independent sequence/hash chain", first, e)
	}
	if e = ledger.Append(first); e != nil {
		t.Fatal("new epoch could not publish its first cut", e)
	}
	if e = ledger.Append(first); e != nil {
		t.Fatal("same-cut publication was not idempotent", e)
	}

	wrongWitness := next
	wrongWitness.ledger = oldLedger
	if d, e := wrongWitness.FinalizeCopy(ctx, FinalizeCopyCommand{CommandHeader: CommandHeader{w, "old-epoch-witness-denied"}, OperationID: restore.ID}); e != nil || d.Decision != "UNKNOWN" {
		t.Fatal("another epoch witness admitted restore", d, e)
	}
	secondCommand := ExportRestoreCheckpointCommand{CommandHeader: CommandHeader{w, "new-epoch-second"}, DatasetVersionID: copies[0].DatasetVersionID}
	second, e := next.ExportRestoreCheckpoint(ctx, secondCommand)
	if e != nil || second.Sequence != 2 || second.PreviousSHA256 != first.ContentSHA256 || ledger.Append(second) != nil {
		t.Fatal("new epoch failed to continue its own chain", second, e)
	}
	// The previous epoch has a larger sequence (3). Finalization must select the
	// latest cut of the configured epoch (2), not the largest asset-wide number.
	decision, e := next.FinalizeCopy(ctx, FinalizeCopyCommand{CommandHeader: CommandHeader{w, "new-epoch-restore-final"}, OperationID: restore.ID})
	if e != nil || decision.Decision != "ONLINE" {
		t.Fatal("current new-epoch witness was rejected", decision, e)
	}
	var checkpoint uuid.UUID
	if e = pool.QueryRow(ctx, `SELECT checkpoint_id FROM lifecycle_copy_decision WHERE id=$1`, decision.ID).Scan(&checkpoint); e != nil || checkpoint != second.ID {
		t.Fatal("decision did not bind the exact configured-epoch cut", checkpoint, e)
	}
	replayed, e := next.ExportRestoreCheckpoint(ctx, secondCommand)
	if e != nil || replayed != second || ledger.Append(replayed) != nil {
		t.Fatal("new-epoch same-key replay changed the cut", replayed, e)
	}
	_, e = pool.Exec(ctx, `INSERT INTO lifecycle_restore_checkpoint(id,workspace_id,dataset_version_id,epoch,sequence,dependency_revision,state_sha256,previous_sha256,content_sha256,created_at,expires_at) SELECT $1,workspace_id,dataset_version_id,epoch,sequence,dependency_revision,state_sha256,previous_sha256,content_sha256,created_at,expires_at FROM lifecycle_restore_checkpoint WHERE id=$2`, uuid.New(), first.ID)
	var pgErr *pgconn.PgError
	if !errors.As(e, &pgErr) || pgErr.Code != "23505" {
		t.Fatal("duplicate sequence within the same epoch was accepted", e)
	}
	var after string
	if e = pool.QueryRow(ctx, `SELECT string_agg(to_jsonb(c)::text, E'\n' ORDER BY sequence) FROM lifecycle_restore_checkpoint c WHERE workspace_id=$1 AND dataset_version_id=$2 AND epoch=$3`, w, copies[0].DatasetVersionID, s.epoch).Scan(&after); e != nil || after != oldFacts {
		t.Fatal("old Core epoch facts changed", e)
	}
	afterBytes, e := os.ReadFile(oldLedger.Path)
	if e != nil || string(afterBytes) != string(oldBytes) {
		t.Fatal("old independent witness bytes changed", e)
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_restore_checkpoint WHERE workspace_id=$1`, w) != 5 || count(t, pool, `SELECT count(*) FROM lifecycle_copy_decision WHERE operation_id=$1 AND decision='UNKNOWN'`, restore.ID) != 1 {
		t.Fatal("replay duplicated cuts or removed the denied historical decision")
	}
}
