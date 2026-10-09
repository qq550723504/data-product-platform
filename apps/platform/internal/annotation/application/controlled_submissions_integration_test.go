package application

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
	infra "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type controlledTestGuard struct {
	revoked atomic.Bool
	calls   atomic.Int32
}

func (g *controlledTestGuard) Preflight(context.Context, d.Campaign, []d.Task) (ActivationProof, error) {
	return ActivationProof{}, nil
}
func (g *controlledTestGuard) ValidateActivationTx(context.Context, pgx.Tx, d.Campaign, ActivationProof) error {
	return nil
}
func (g *controlledTestGuard) ValidateEngineSendTx(context.Context, pgx.Tx, d.Campaign) error {
	g.calls.Add(1)
	if g.revoked.Load() {
		return ErrActivationGuardRejected
	}
	return nil
}

type controlledFixture struct {
	pool      *pgxpool.Pool
	repo      *infra.Repository
	service   *Service
	guard     *controlledTestGuard
	source    d.SourceObservation
	operation d.EngineOperation
}

func newControlledFixture(t *testing.T) *controlledFixture {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := t.Context()
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	router, err := routing.NewRouter(false)
	if err != nil {
		t.Fatal(err)
	}
	outbox.ConfigureAppendObligation(router)
	fx := seedGoldPreflightBaseFixture(t, ctx, pool, false)
	repo := infra.NewRepository(pool)
	guard := &controlledTestGuard{}
	manager := transaction.NewManager(pool)
	svc := NewService(manager, repo, guard)
	campaign, err := repo.GetCampaign(ctx, fx.campaignID)
	if err != nil {
		t.Fatal(err)
	}
	task, err := repo.GetTask(ctx, fx.taskID)
	if err != nil {
		t.Fatal(err)
	}
	binding := d.EngineCampaignBinding{ID: uuid.New(), WorkspaceID: fx.workspaceID, CampaignID: fx.campaignID, Provider: "label-studio", ProviderInstance: "synthetic-" + uuid.NewString(), ExternalProjectID: "41", RequestID: uuid.NewString(), ConfigSHA256: strings.Repeat("e", 64), CreatedAt: time.Now().UTC(),
		SourceContract: d.SourceContract{AdmissionProtocol: d.ControlledSubmissionProtocol, ConnectionID: uuid.New(), ProviderIncarnation: uuid.NewString(), SourceCommit: strings.Repeat("1", 40), EngineVersion: "synthetic-294", ImageDigest: "sha256:" + strings.Repeat("2", 64), NormalizerVersion: "label-studio-single-label-v1"}}
	tb := d.EngineTaskBinding{ID: uuid.New(), WorkspaceID: fx.workspaceID, CampaignBindingID: binding.ID, CampaignID: fx.campaignID, TaskID: fx.taskID, ExternalTaskID: "101", CreatedAt: time.Now().UTC()}
	ab := d.EngineActorBinding{ID: uuid.New(), WorkspaceID: fx.workspaceID, Provider: binding.Provider, ProviderInstance: binding.ProviderInstance, ExternalActorRef: "7", CoreActorRef: "annotator", CreatedAt: time.Now().UTC()}
	if err := manager.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := repo.InsertEngineCampaignBinding(ctx, tx, binding); err != nil {
			return err
		}
		if _, err := repo.InsertEngineTaskBinding(ctx, tx, tb); err != nil {
			return err
		}
		_, err := repo.InsertEngineActorBinding(ctx, tx, ab)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"immutable":"synthetic body","quality":1.0}`)
	payload := []byte(`{"label":"A"}`)
	f := d.SourceFingerprint{WorkspaceID: fx.workspaceID, ConnectionID: binding.ConnectionID, ProviderInstance: binding.ProviderInstance, ProviderIncarnation: binding.ProviderIncarnation, SourceKind: d.ImmutableSubmissionSource, ExternalID: "501", AssignmentID: "301", SubmissionRevision: 1,
		ExternalProjectID: "41", ExternalTaskID: "101", ExternalAnnotationID: "201", ExternalAuthorRef: "7", CoreAuthorRef: "annotator", CampaignID: fx.campaignID, TaskID: fx.taskID, InputVersionID: campaign.InputDatasetVersionID, InputSHA256: campaign.InputChecksumSHA256,
		SourceItemRef: task.SourceItemRef, SourceSHA256: task.SourceContentSHA256, TaskTextSHA256: task.TaskTextSHA256, CampaignBindingID: binding.ID, TaskBindingID: tb.ID, ActorBindingID: ab.ID, ConfigSHA256: binding.ConfigSHA256,
		SchemaSHA256: campaign.Schema.ContentSHA256, TaxonomySHA256: campaign.Taxonomy.ContentSHA256, RubricSHA256: campaign.Rubric.ContentSHA256, RendererSHA256: campaign.Renderer.ContentSHA256, ReviewPolicySHA256: campaign.ReviewPolicy.ContentSHA256,
		NormalizerVersion: binding.NormalizerVersion, SnapshotSHA256: d.SourceDigest(raw), CanonicalPayloadSHA256: d.SourceDigest(payload)}
	f.MappingSHA256 = f.MappingHash()
	source := d.SourceObservation{ID: f.Identity(), SourceFingerprint: f, Snapshot: raw, CanonicalPayload: payload, AssignmentObservation: d.AssignmentObservation{Version: 9, ObservedAt: time.Now().UTC()}}
	if err := source.Validate(); err != nil {
		t.Fatal(err)
	}
	mf := engineTasksManifest{Kind: d.EngineOperationSubmitTasks, WorkspaceID: fx.workspaceID.String(), CampaignID: fx.campaignID.String(), Binding: engineBinding{SourceContract: binding.SourceContract, Provider: binding.Provider, ProviderInstance: binding.ProviderInstance, ExternalProjectID: "41", RequestID: binding.RequestID, ConfigSHA256: binding.ConfigSHA256}, Tasks: []EngineTask{{TaskID: task.ID, SourceItemRef: task.SourceItemRef, SourceSHA256: task.SourceContentSHA256, TaskText: "synthetic", TaskTextSHA256: task.TaskTextSHA256, CorrelationKey: "synthetic-task"}}}
	body, _ := json.Marshal(mf)
	op := d.EngineOperation{ID: uuid.New(), WorkspaceID: fx.workspaceID, CampaignID: fx.campaignID, Provider: binding.Provider, ProviderInstanceRef: binding.ProviderInstance, OperationKind: d.EngineOperationSubmitTasks, RequestID: uuid.NewString(), RequestFingerprint: hashBytes(body), PayloadManifest: body, PayloadManifestHash: body, PayloadManifestSHA256: hashBytes(body), Status: d.EngineOperationMatched, Revision: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := manager.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := repo.InsertEngineOperation(ctx, tx, op)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	prepare := PrepareSubmissionBatchCommand{WorkspaceID: fx.workspaceID, CampaignID: fx.campaignID, Expected: []d.SubmissionExpectation{{TaskID: fx.taskID, AssignmentID: "301", Revision: 1}}, PageBudget: 2, Quiescent: true}
	if err := svc.PrepareControlledSubmissionBatch(ctx, prepare); err != nil {
		t.Fatal(err)
	}
	if err := svc.PrepareControlledSubmissionBatch(ctx, prepare); err != nil {
		t.Fatalf("batch replay: %v", err)
	}
	return &controlledFixture{pool: pool, repo: repo, service: svc, guard: guard, source: source, operation: op}
}
func (f *controlledFixture) command(s d.SourceObservation, key string) RecordResultCommand {
	return RecordResultCommand{WorkspaceID: s.WorkspaceID, CampaignID: s.CampaignID, TaskID: s.TaskID, ExpectedTaskRevision: 1, AuthorRef: s.CoreAuthorRef, ProviderBindingRef: s.CampaignBindingID.String(), ExternalTaskID: s.ExternalTaskID, ExternalAnnotationID: s.ExternalAnnotationID, ExternalRevision: s.ExternalRevision(), ObservationKey: key, CanonicalPayload: s.CanonicalPayload, CanonicalPayloadSHA256: s.CanonicalPayloadSHA256, NormalizerVersion: s.NormalizerVersion, Source: &s}
}
func (f *controlledFixture) complete(t *testing.T) {
	t.Helper()
	ctx := t.Context()
	r := NewEngineResultReconciler(f.service.tx, f.repo, nil, f.service)
	c, err := f.repo.GetCampaign(ctx, f.source.CampaignID)
	if err != nil {
		t.Fatal(err)
	}
	scan, err := r.batchReceipt(ctx, c, false, uuid.Nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.batchReceipt(ctx, c, true, scan, []d.SourceObservation{f.source}); err != nil {
		t.Fatal(err)
	}
}
func (f *controlledFixture) review(t *testing.T, resultID uuid.UUID, correct bool) ReviewAnnotationResult {
	t.Helper()
	actor := uuid.New()
	action := d.ReviewAccept
	var payload []byte
	if correct {
		action = d.ReviewCorrect
		payload = []byte(`{"label":"B"}`)
	}
	review, err := f.service.ReviewAnnotation(t.Context(), ReviewAnnotationCommand{WorkspaceID: f.source.WorkspaceID, CampaignID: f.source.CampaignID, TaskID: f.source.TaskID, ExpectedTaskRevision: 2, ReviewerRef: actor.String(), Action: action, Reason: "synthetic verified", IdempotencyKey: uuid.NewString(), ReviewedResultID: &resultID, CorrectedPayload: payload, CorrectedPayloadHash: func() string {
		if correct {
			return d.SourceDigest(payload)
		}
		return ""
	}(), ActorID: &actor})
	if err != nil {
		t.Fatal(err)
	}
	return review
}
func TestControlledResultReplayChecksFullSourceAndCurrentAuthority(t *testing.T) {
	f := newControlledFixture(t)
	ctx := t.Context()
	cmd := f.command(f.source, "first")
	result, err := f.service.RecordAnnotationResult(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"first", "alias", "alias"} {
		cmd.ObservationKey = key
		replay, err := f.service.RecordAnnotationResult(ctx, cmd)
		if err != nil || replay.ID != result.ID {
			t.Fatalf("replay=%+v err=%v", replay, err)
		}
	}
	for _, change := range []func(*d.SourceObservation){func(s *d.SourceObservation) {
		s.Snapshot = []byte(`{"immutable":"changed same label"}`)
		s.SnapshotSHA256 = d.SourceDigest(s.Snapshot)
	}, func(s *d.SourceObservation) { s.ExternalAuthorRef = "8" }, func(s *d.SourceObservation) {
		s.ConfigSHA256 = strings.Repeat("a", 64)
		s.MappingSHA256 = s.MappingHash()
	}} {
		changed := f.source
		change(&changed)
		if changed.ID != f.source.ID {
			t.Fatal("fingerprint dimensions changed source key")
		}
		if _, err := f.service.RecordAnnotationResult(ctx, f.command(changed, "alias")); err == nil {
			t.Fatal("changed same-source fact accepted")
		}
	}
	denied := f.command(f.source, "first")
	f.guard.revoked.Store(true)
	if _, err := f.service.RecordAnnotationResult(ctx, denied); !errors.Is(err, ErrActivationGuardRejected) {
		t.Fatalf("revoked replay=%v", err)
	}
	f.guard.revoked.Store(false)
	denied.WorkspaceID = uuid.New()
	if _, err := f.service.RecordAnnotationResult(ctx, denied); err == nil {
		t.Fatal("cross-workspace replay accepted")
	}
	var count int
	var revision int64
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM annotation_result WHERE campaign_id=$1", f.source.CampaignID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("results=%d %v", count, err)
	}
	if err := f.pool.QueryRow(ctx, "SELECT revision FROM annotation_task WHERE id=$1", f.source.TaskID).Scan(&revision); err != nil || revision != 2 {
		t.Fatalf("revision=%d %v", revision, err)
	}
}

func TestControlledResultRollbackAndDeferredBindingRequirement(t *testing.T) {
	f := newControlledFixture(t)
	ctx := t.Context()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	fn := "dpp294_fail_" + suffix
	trigger := fn
	sql := "CREATE FUNCTION " + fn + "() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.workspace_id='" + f.source.WorkspaceID.String() + "'::uuid THEN RAISE EXCEPTION 'synthetic rollback probe'; END IF; RETURN NEW; END $$"
	goldPreflightExec(t, ctx, f.pool, sql)
	goldPreflightExec(t, ctx, f.pool, "CREATE TRIGGER "+trigger+" BEFORE INSERT ON audit_event FOR EACH ROW EXECUTE FUNCTION "+fn+"()")
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), "DROP TRIGGER "+trigger+" ON audit_event")
		_, _ = f.pool.Exec(context.Background(), "DROP FUNCTION "+fn+"()")
	})
	if _, err := f.service.RecordAnnotationResult(ctx, f.command(f.source, "rollback")); err == nil {
		t.Fatal("injected audit failure accepted")
	}
	for _, table := range []string{"annotation_result", "annotation_source_observation"} {
		var n int
		if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE campaign_id=$1", f.source.CampaignID).Scan(&n); err != nil || n != 0 {
			t.Fatalf("%s remained %d %v", table, n, err)
		}
	}
	var n int
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM annotation_source_result_binding b JOIN annotation_result r ON r.id=b.result_id WHERE r.campaign_id=$1", f.source.CampaignID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("bindings=%d %v", n, err)
	}
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	result := d.Result{ID: uuid.New(), WorkspaceID: f.source.WorkspaceID, CampaignID: f.source.CampaignID, TaskID: f.source.TaskID, AuthorRef: f.source.CoreAuthorRef, ProviderBindingRef: f.source.CampaignBindingID.String(), ExternalTaskID: "101", ExternalAnnotationID: "201", ExternalRevision: f.source.ExternalRevision(), ObservationKey: "missing-binding", CanonicalPayload: f.source.CanonicalPayload, CanonicalPayloadSHA256: f.source.CanonicalPayloadSHA256, NormalizerVersion: f.source.NormalizerVersion, CreatedAt: time.Now().UTC()}
	if err := f.repo.InsertResult(ctx, tx, result); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("Result without same-transaction binding committed")
	}
}
func TestControlledCorrectionSealIntegrityAndLateObservation(t *testing.T) {
	f := newControlledFixture(t)
	ctx := t.Context()
	result, err := f.service.RecordAnnotationResult(ctx, f.command(f.source, "source"))
	if err != nil {
		t.Fatal(err)
	}
	actor := uuid.New()
	_, err = f.service.ReviewAnnotation(ctx, ReviewAnnotationCommand{WorkspaceID: f.source.WorkspaceID, CampaignID: f.source.CampaignID, TaskID: f.source.TaskID, ExpectedTaskRevision: 2, ReviewerRef: actor.String(), Action: d.ReviewAccept, Reason: "unresolved", IdempotencyKey: uuid.NewString(), ReviewedResultID: &result.ID, ActorID: &actor})
	if err == nil {
		t.Fatal("review accepted unresolved batch")
	}
	f.complete(t)
	f.review(t, result.ID, true)
	release := make(chan struct{})
	type sealResult struct {
		s d.Snapshot
		e error
	}
	sealed := make(chan sealResult, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-release
			s, e := f.service.FinalizeAnnotationSnapshot(ctx, FinalizeAnnotationSnapshotCommand{WorkspaceID: f.source.WorkspaceID, CampaignID: f.source.CampaignID})
			sealed <- sealResult{s, e}
		}()
	}
	close(release)
	a, b := <-sealed, <-sealed
	if a.e != nil || b.e != nil || a.s.ID != b.s.ID {
		t.Fatalf("double finalizer %v/%v %s/%s", a.e, b.e, a.s.ID, b.s.ID)
	}
	snapshot := a.s
	var manifest struct{ SourceClosure []infra.SourceClosure }
	if err := json.Unmarshal(snapshot.Manifest, &manifest); err != nil || len(manifest.SourceClosure) != 2 {
		t.Fatalf("correction source closure %+v %v", manifest, err)
	}
	for _, c := range manifest.SourceClosure {
		if c.SourceID != f.source.ID || c.OriginalResultID != result.ID {
			t.Fatalf("correction made another provider source: %+v", c)
		}
	}
	if pre, err := f.service.GoldQualityPreflight(ctx, f.source.WorkspaceID, f.source.CampaignID); err != nil || pre.Blocking {
		t.Fatalf("Gold source preflight %v %v", pre, err)
	}
	r := NewEngineResultReconciler(f.service.tx, f.repo, nil, f.service)
	attempt, err := r.startFetchAttempt(ctx, f.operation)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.sourceReceipt(ctx, f.source, attempt.ID, "LATE"); err != nil {
		t.Fatal(err)
	}
	if err := r.sourceReceipt(ctx, f.source, attempt.ID, "LATE"); err != nil {
		t.Fatal(err)
	}
	changed := f.source
	changed.Snapshot = []byte(`{"late":"same label changed body"}`)
	changed.SnapshotSHA256 = d.SourceDigest(changed.Snapshot)
	if err := r.sourceReceipt(ctx, changed, attempt.ID, "LATE"); !errors.Is(err, d.ErrSourceConflict) {
		t.Fatalf("late conflict=%v", err)
	}
	var late, conflicts int
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FILTER(WHERE disposition='LATE'),count(*) FILTER(WHERE disposition='CONFLICT') FROM annotation_source_receipt WHERE campaign_id=$1", f.source.CampaignID).Scan(&late, &conflicts); err != nil || late != 1 || conflicts != 1 {
		t.Fatalf("receipts %d/%d %v", late, conflicts, err)
	}
	stable, err := f.service.FinalizeAnnotationSnapshot(ctx, FinalizeAnnotationSnapshotCommand{WorkspaceID: f.source.WorkspaceID, CampaignID: f.source.CampaignID})
	if err != nil || stable.RootHash != snapshot.RootHash {
		t.Fatalf("late root changed %v", err)
	}
	// A privileged corruption simulation affects only this random synthetic fixture.
	conn, err := f.pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		conn.Release()
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL session_replication_role=replica"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "DELETE FROM annotation_source_result_binding WHERE result_id=$1", result.ID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	conn.Release()
	if intact, err := f.repo.GetSnapshotIntegrity(ctx, snapshot.ID); err == nil && intact {
		t.Fatal("missing source binding passed read integrity")
	}
	if _, err := f.service.FinalizeAnnotationSnapshot(ctx, FinalizeAnnotationSnapshotCommand{WorkspaceID: f.source.WorkspaceID, CampaignID: f.source.CampaignID}); err == nil {
		t.Fatal("broken source replay returned finalized snapshot")
	}
	if pre, err := f.service.GoldQualityPreflight(ctx, f.source.WorkspaceID, f.source.CampaignID); err == nil && !pre.Blocking {
		t.Fatal("Gold accepted source corruption")
	}
	var root string
	if err := f.pool.QueryRow(ctx, "SELECT root_hash FROM annotation_snapshot WHERE id=$1", snapshot.ID).Scan(&root); err != nil || root != snapshot.RootHash {
		t.Fatalf("old FINALIZED root overwritten %v", err)
	}
}
func TestControlledConcurrentSameSourceConflictAndBatchFence(t *testing.T) {
	f := newControlledFixture(t)
	ctx := t.Context()
	firstTx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer firstTx.Rollback(ctx)
	if _, err := f.repo.LockCampaignTx(ctx, firstTx, f.source.CampaignID); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.StoreSourceTx(ctx, firstTx, f.source); err != nil {
		t.Fatal(err)
	}
	changed := f.source
	changed.Snapshot = []byte(`{"parallel":"different body"}`)
	changed.SnapshotSHA256 = d.SourceDigest(changed.Snapshot)
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		tx, err := f.pool.Begin(ctx)
		if err != nil {
			done <- err
			return
		}
		defer tx.Rollback(ctx)
		close(started)
		done <- f.repo.StoreSourceTx(ctx, tx, changed)
	}()
	<-started
	select {
	case err := <-done:
		t.Fatalf("writer crossed held parent fence %v", err)
	case <-time.After(40 * time.Millisecond):
	}
	if err := firstTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, d.ErrSourceConflict) {
			t.Fatalf("same source conflict %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("conflict writer hung")
	}
	if _, err := f.service.RecordAnnotationResult(ctx, f.command(f.source, "bind")); err != nil {
		t.Fatal(err)
	}
	c, err := f.repo.GetCampaign(ctx, f.source.CampaignID)
	if err != nil {
		t.Fatal(err)
	}
	r := NewEngineResultReconciler(f.service.tx, f.repo, nil, f.service)
	stale, err := r.batchReceipt(ctx, c, false, uuid.Nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := r.batchReceipt(ctx, c, false, uuid.Nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.batchReceipt(ctx, c, true, stale, []d.SourceObservation{f.source}); err == nil {
		t.Fatal("stale scan overwrote newer unresolved state")
	}
	if _, err := r.batchReceipt(ctx, c, true, fresh, []d.SourceObservation{f.source}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, "UPDATE annotation_source_observation SET snapshot=$2 WHERE id=$1", f.source.ID, []byte(`{"tampered":true}`)); err == nil {
		t.Fatal("source mutation succeeded")
	}
}

type controlledBatchPort struct {
	AnnotationEnginePort
	source   d.SourceObservation
	scenario string
	calls    int
	guard    *controlledTestGuard
}

func (p *controlledBatchPort) Provider() string    { return "label-studio" }
func (p *controlledBatchPort) InstanceRef() string { return p.source.ProviderInstance }
func (p *controlledBatchPort) FetchResults(ctx context.Context, req EngineLookupRequest, cursor EngineResultCursor) (EngineResultPage, error) {
	observer := EngineInvocationFromContext(ctx)
	if observer == nil {
		return EngineResultPage{}, ErrActivationGuardRequired
	}
	attempt, err := observer.Start(ctx)
	if err != nil {
		return EngineResultPage{}, err
	}
	p.calls++
	s := p.source
	if p.scenario == "drift" && p.calls > 1 {
		s.Snapshot = []byte(`{"drift":"same label"}`)
		s.SnapshotSHA256 = d.SourceDigest(s.Snapshot)
	}
	o := EngineResultObservation{TaskID: s.TaskID, ExternalTaskID: s.ExternalTaskID, ExternalAnnotationID: s.ExternalAnnotationID, ExternalRevision: s.ExternalRevision(), ExternalAuthorRef: s.ExternalAuthorRef, CanonicalPayload: s.CanonicalPayload, CanonicalPayloadSHA256: s.CanonicalPayloadSHA256, NormalizerVersion: s.NormalizerVersion, ProviderSubmitted: true, Source: &EngineImmutableSource{AssignmentObservation: d.AssignmentObservation{Version: int64(p.calls + 9), ObservedAt: time.Now().UTC()}, PhysicalAttemptID: attempt.ID, ExternalID: s.ExternalID, AssignmentID: s.AssignmentID, Revision: s.SubmissionRevision, Snapshot: s.Snapshot, SnapshotSHA256: s.SnapshotSHA256, ExternalProjectID: s.ExternalProjectID}}
	page := EngineResultPage{Results: []EngineResultObservation{o}}
	if p.scenario == "missing" {
		page.Results = nil
	}
	if p.scenario == "quarantine" {
		page.Results[0].Quarantined = true
	}
	if p.scenario == "budget" {
		page.Results = nil
		page.NextCursor = &EngineResultCursor{Offset: cursor.Offset + 100}
	}
	if p.scenario == "revoke" {
		p.guard.revoked.Store(true)
	}
	if err := observer.Finish(ctx, attempt, nil); err != nil {
		return EngineResultPage{}, err
	}
	return page, nil
}
func TestControlledFiniteBatchCompletenessAndDrift(t *testing.T) {
	for _, scenario := range []string{"complete", "missing", "drift", "quarantine", "budget", "revoke"} {
		t.Run(scenario, func(t *testing.T) {
			f := newControlledFixture(t)
			engine := &controlledBatchPort{source: f.source, scenario: scenario, guard: f.guard}
			r := NewEngineResultReconciler(f.service.tx, f.repo, engine, f.service)
			err := r.ReconcileCampaign(t.Context(), f.source.CampaignID)
			if scenario == "complete" && err != nil {
				t.Fatal(err)
			}
			if scenario == "complete" {
				var n int
				var first, last int64
				if err := f.pool.QueryRow(t.Context(), "SELECT count(*),min(observed_assignment_version),max(observed_assignment_version) FROM annotation_source_receipt WHERE campaign_id=$1 AND assignment_version_semantics='OBSERVED_CURRENT' AND observed_at IS NOT NULL", f.source.CampaignID).Scan(&n, &first, &last); err != nil || n != 2 || first != 10 || last != 11 {
					t.Fatalf("current token receipts=%d/%d/%d err=%v", n, first, last, err)
				}
			}
			if scenario != "complete" && err == nil {
				t.Fatal("incomplete/unauthorized batch accepted")
			}
			var outcome string
			if err := f.pool.QueryRow(t.Context(), "SELECT outcome FROM annotation_submission_batch_receipt WHERE campaign_id=$1 ORDER BY sequence DESC LIMIT 1", f.source.CampaignID).Scan(&outcome); err != nil {
				t.Fatal(err)
			}
			want := "UNRESOLVED"
			if scenario == "complete" {
				want = "COMPLETE"
			}
			if outcome != want {
				t.Fatalf("outcome=%s want %s", outcome, want)
			}
			var results, attempts, costs int
			if err := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM annotation_result WHERE campaign_id=$1", f.source.CampaignID).Scan(&results); err != nil {
				t.Fatal(err)
			}
			if scenario != "complete" && results != 0 {
				t.Fatalf("failed batch admitted %d Results", results)
			}
			if err := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM annotation_engine_attempt WHERE operation_id=$1", f.operation.ID).Scan(&attempts); err != nil {
				t.Fatal(err)
			}
			if err := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM cost_allocation WHERE annotation_engine_attempt_id IN(SELECT id FROM annotation_engine_attempt WHERE operation_id=$1)", f.operation.ID).Scan(&costs); err != nil {
				t.Fatal(err)
			}
			if attempts != engine.calls || costs != engine.calls {
				t.Fatalf("physical calls/attempts/costs=%d/%d/%d", engine.calls, attempts, costs)
			}
			if scenario == "complete" {
				var id uuid.UUID
				if err := f.pool.QueryRow(t.Context(), "SELECT id FROM annotation_result WHERE campaign_id=$1", f.source.CampaignID).Scan(&id); err != nil {
					t.Fatal(err)
				}
				f.review(t, id, false)
				snapshot, err := f.service.FinalizeAnnotationSnapshot(t.Context(), FinalizeAnnotationSnapshotCommand{WorkspaceID: f.source.WorkspaceID, CampaignID: f.source.CampaignID})
				if err != nil {
					t.Fatal(err)
				}
				if err := r.ReconcileCampaign(t.Context(), f.source.CampaignID); err != nil {
					t.Fatalf("SEALED late collection: %v", err)
				}
				stable, err := f.service.FinalizeAnnotationSnapshot(t.Context(), FinalizeAnnotationSnapshotCommand{WorkspaceID: f.source.WorkspaceID, CampaignID: f.source.CampaignID})
				if err != nil || stable.RootHash != snapshot.RootHash {
					t.Fatalf("late enumeration changed snapshot %v", err)
				}
			}
		})
	}
}
func TestControlledSourceBodyCorruptionBlocksGold(t *testing.T) {
	f := newControlledFixture(t)
	ctx := t.Context()
	result, err := f.service.RecordAnnotationResult(ctx, f.command(f.source, "body-integrity"))
	if err != nil {
		t.Fatal(err)
	}
	f.complete(t)
	f.review(t, result.ID, false)
	snapshot, err := f.service.FinalizeAnnotationSnapshot(ctx, FinalizeAnnotationSnapshotCommand{WorkspaceID: f.source.WorkspaceID, CampaignID: f.source.CampaignID})
	if err != nil {
		t.Fatal(err)
	}
	damaged := f.source
	damaged.Snapshot = []byte(`{"privileged":"body replacement"}`)
	damaged.SnapshotSHA256 = d.SourceDigest(damaged.Snapshot)
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SET LOCAL session_replication_role=replica"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "UPDATE annotation_source_observation SET snapshot=$2,snapshot_sha256=$3,fingerprint_payload=$4,fingerprint_sha256=$5 WHERE id=$1", damaged.ID, damaged.Snapshot, damaged.SnapshotSHA256, damaged.Payload(), damaged.Hash()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if intact, err := f.repo.GetSnapshotIntegrity(ctx, snapshot.ID); err == nil && intact {
		t.Fatal("changed source body passed integrity")
	}
	if pre, err := f.service.GoldQualityPreflight(ctx, f.source.WorkspaceID, f.source.CampaignID); err == nil && !pre.Blocking {
		t.Fatal("Gold accepted body replacement")
	}
}

func TestControlledReviewFirstRejectsWaitingResultThenSeals(t *testing.T) {
	f := newControlledFixture(t)
	ctx := t.Context()
	result, err := f.service.RecordAnnotationResult(ctx, f.command(f.source, "race-root"))
	if err != nil {
		t.Fatal(err)
	}
	f.complete(t)
	actor := uuid.New()
	cmd := ReviewAnnotationCommand{WorkspaceID: f.source.WorkspaceID, CampaignID: f.source.CampaignID, TaskID: f.source.TaskID, ExpectedTaskRevision: 2, ReviewerRef: actor.String(), Action: d.ReviewAccept, Reason: "review wins parent fence", IdempotencyKey: uuid.NewString(), ReviewedResultID: &result.ID, ActorID: &actor}
	fingerprint, err := reviewFingerprint(cmd)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ensureReviewAttempt(ctx, cmd, fingerprint); err != nil {
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(os.Getenv("TEST_POSTGRES_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	name := "dpp294-review-" + uuid.NewString()
	cfg.ConnConfig.RuntimeParams["application_name"] = name
	cfg.MaxConns = 2
	reviewPool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reviewPool.Close()
	reviewSvc := NewService(transaction.NewManager(reviewPool), infra.NewRepository(reviewPool), f.guard)
	hold, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer hold.Rollback(ctx)
	if _, err := f.repo.LockCampaignTx(ctx, hold, f.source.CampaignID); err != nil {
		t.Fatal(err)
	}
	reviewed := make(chan error, 1)
	go func() { _, err := reviewSvc.ReviewAnnotation(ctx, cmd); reviewed <- err }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting bool
		if err := f.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name=$1 AND wait_event_type='Lock')", name).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("review did not reach parent lock barrier")
		}
		time.Sleep(10 * time.Millisecond)
	}
	changed := f.source
	changed.ExternalID = "502"
	changed.SubmissionRevision = 2
	changed.ID = changed.Identity()
	late := make(chan error, 1)
	go func() {
		_, err := f.service.RecordAnnotationResult(ctx, f.command(changed, "waiting-late"))
		late <- err
	}()
	if err := hold.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-reviewed; err != nil {
		t.Fatal(err)
	}
	if err := <-late; err == nil {
		t.Fatal("waiting source result crossed completed review")
	}
	snapshot, err := f.service.FinalizeAnnotationSnapshot(ctx, FinalizeAnnotationSnapshotCommand{WorkspaceID: f.source.WorkspaceID, CampaignID: f.source.CampaignID})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Status != d.SnapshotFinalized {
		t.Fatal("review-first campaign did not seal")
	}
	var n int
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM annotation_result WHERE campaign_id=$1", f.source.CampaignID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("late candidate changed membership %d %v", n, err)
	}
}
