package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
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
