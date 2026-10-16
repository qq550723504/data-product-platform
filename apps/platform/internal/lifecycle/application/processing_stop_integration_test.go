package application

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	workflowdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	"testing"
)

func TestProcessingStopFullMembershipAtomicRollbackAndReplay(t *testing.T) {
	pool := planDatabase(t)
	w, cs, o, _, _, verifier, _ := occupancyFixture(t, pool)
	ctx := context.Background()
	// The seed has two copies of one version. Add an actual second immutable
	// input version so the test exercises an atomic multi-pin batch.
	var dataset, namespace uuid.UUID
	if e := pool.QueryRow(ctx, `SELECT v.dataset_id,c.namespace_id FROM lifecycle_fixture_copy c JOIN dataset_version v ON v.id=c.dataset_version_id WHERE c.id=$1`, cs[1].ID).Scan(&dataset, &namespace); e != nil {
		t.Fatal(e)
	}
	cs[1].ID = uuid.New()
	cs[1].DatasetVersionID = uuid.New()
	cs[1].Key += "-batch-two"
	if _, e := pool.Exec(ctx, `INSERT INTO dataset_version(id,dataset_id,version_no,status,storage_type,storage_uri,checksum_algorithm,checksum_value,byte_size) VALUES($1,$2,2,'READY','OBJECT_STORAGE','s3://no-provider-configured/batch-two.csv','SHA256',$3,$4)`, cs[1].DatasetVersionID, dataset, cs[1].ContentSHA256, cs[1].ByteSize); e != nil {
		t.Fatal(e)
	}
	if _, e := pool.Exec(ctx, `INSERT INTO lifecycle_fixture_copy(id,workspace_id,namespace_id,dataset_version_id,object_key,provider_version,content_sha256,byte_size) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, cs[1].ID, w, namespace, cs[1].DatasetVersionID, cs[1].Key, cs[1].ProviderVersion, cs[1].ContentSHA256, cs[1].ByteSize); e != nil {
		t.Fatal(e)
	}
	base, svc := subjectExecution(t, pool, w, cs[0], o)
	ex, e := svc.Create(ctx, workflowapp.CreateExecutionCommand{WorkspaceID: w, WorkflowVersionID: base.WorkflowVersionID, OutputDatasetID: base.OutputDatasetID, TargetPeriod: "2026-10", Inputs: []workflowdomain.InputBinding{{Name: "first", DatasetVersionID: cs[0].DatasetVersionID}, {Name: "second", DatasetVersionID: cs[1].DatasetVersionID}}, ActorID: &o.authority.(*occupancyAuthority).caller.ActorID, IdempotencyKey: "batch-two"})
	if e != nil {
		t.Fatal(e)
	}
	ex, e = svc.StartWithReferenceCheck(ctx, ex.ID, "native:"+ex.ID.String(), "sql-only")
	if e != nil {
		t.Fatal(e)
	}
	o.termination = nil
	reader := NewFixtureReaderServiceWithRecovery(o, &readerTestPort{}, verifier)
	pins := NewFixtureSubjectPins(reader, map[uuid.UUID]uuid.UUID{cs[0].DatasetVersionID: cs[0].ID, cs[1].DatasetVersionID: cs[1].ID})
	e = pins.ExecuteProcessing(readerContext(o), w, ex.ID, func(x context.Context) error { pins.MarkProcessingOutputUnknown(x); return nil })
	if !errors.Is(e, domain.ErrTerminationUnknown) {
		t.Fatal(e)
	}
	rows, e := pool.Query(ctx, `SELECT id,generation FROM lifecycle_reader_pin WHERE execution_id=$1 ORDER BY id`, ex.ID)
	if e != nil {
		t.Fatal(e)
	}
	var stops []TerminateFixtureReaderCommand
	for rows.Next() {
		s := TerminateFixtureReaderCommand{CommandHeader: CommandHeader{WorkspaceID: w}, Outcome: domain.ReaderCancelled}
		if e = rows.Scan(&s.PinID, &s.Generation); e != nil {
			t.Fatal(e)
		}
		stops = append(stops, s)
	}
	rows.Close()
	if len(stops) != 2 {
		t.Fatal(stops)
	}
	cmd := TerminateProcessingReadersCommand{CommandHeader: CommandHeader{w, "batch-atomic"}, ExecutionID: ex.ID, Stops: stops}
	partial := cmd
	partial.Stops = stops[:1]
	if _, e = o.TerminateProcessingReaders(ctx, partial); !errors.Is(e, domain.ErrTerminationUnknown) {
		t.Fatal("partial accepted", e)
	}
	invalid := cmd
	invalid.Stops = append([]TerminateFixtureReaderCommand(nil), stops...)
	invalid.Stops[1].Generation++
	if _, e = o.TerminateProcessingReaders(ctx, invalid); !errors.Is(e, domain.ErrTerminationUnknown) {
		t.Fatal("foreign generation accepted", e)
	}
	verifier.hook = func(p *domain.ReaderTermination) {
		if p.PinID == stops[1].PinID {
			p.Generation++
		}
	}
	if _, e = o.TerminateProcessingReaders(ctx, cmd); !errors.Is(e, domain.ErrTerminationUnknown) {
		t.Fatal("invalid proof accepted", e)
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_reader_termination`) != 0 {
		t.Fatal("partial transaction survived rollback")
	}
	verifier.hook = nil
	result, e := o.TerminateProcessingReaders(ctx, cmd)
	if e != nil {
		t.Fatal(e)
	}
	calls := verifier.calls.Load()
	again, e := o.TerminateProcessingReaders(ctx, cmd)
	if e != nil || !again.Replayed || again.ID != result.ID || verifier.calls.Load() != calls || count(t, pool, `SELECT count(*) FROM lifecycle_reader_termination`) != 2 {
		t.Fatal("replay changed facts", e)
	}
}
