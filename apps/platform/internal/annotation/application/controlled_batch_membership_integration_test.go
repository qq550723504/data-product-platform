package application

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
	infra "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	"os"
	"strings"
	"testing"
	"time"
)

func TestControlledBatchCompleteCannotAuthorizeOutOfSetResult(t *testing.T) {
	f := newControlledFixture(t)
	ctx := t.Context()
	first, err := f.service.RecordAnnotationResult(ctx, f.command(f.source, "expected-root"))
	if err != nil {
		t.Fatal(err)
	}
	f.complete(t)
	extra := f.source
	extra.ExternalID = "502"
	extra.SubmissionRevision = 2
	extra.ID = extra.Identity()
	cmd := f.command(extra, "out-of-set")
	cmd.ExpectedTaskRevision = 2
	unexpected, err := f.service.RecordAnnotationResult(ctx, cmd)
	if err == nil {
		actor := uuid.New()
		_, reviewErr := f.service.ReviewAnnotation(ctx, ReviewAnnotationCommand{
			WorkspaceID: extra.WorkspaceID, CampaignID: extra.CampaignID, TaskID: extra.TaskID,
			ExpectedTaskRevision: 3, ReviewerRef: actor.String(), Action: d.ReviewAccept, Reason: "out-of-set repro",
			IdempotencyKey: uuid.NewString(), ReviewedResultID: &unexpected.ID, ActorID: &actor,
		})
		snapshot, sealErr := f.service.FinalizeAnnotationSnapshot(ctx, FinalizeAnnotationSnapshotCommand{WorkspaceID: extra.WorkspaceID, CampaignID: extra.CampaignID})
		pre, goldErr := f.service.GoldQualityPreflight(ctx, extra.WorkspaceID, extra.CampaignID)
		t.Fatalf("out-of-set Result admitted after COMPLETE: review=%v seal=%v snapshot=%s/%s Gold blocking=%v err=%v", reviewErr, sealErr, snapshot.ID, snapshot.Status, pre.Blocking, goldErr)
	}
	var n int
	var revision int64
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM annotation_result WHERE campaign_id=$1", extra.CampaignID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("membership=%d err=%v", n, err)
	}
	if err := f.pool.QueryRow(ctx, "SELECT revision FROM annotation_task WHERE id=$1", extra.TaskID).Scan(&revision); err != nil || revision != 2 {
		t.Fatalf("revision=%d err=%v", revision, err)
	}
	replay, err := f.service.RecordAnnotationResult(ctx, f.command(f.source, "expected-root-alias"))
	if err != nil || replay.ID != first.ID {
		t.Fatalf("legal replay=%s err=%v", replay.ID, err)
	}
	f.review(t, first.ID, false)
	snapshot, err := f.service.FinalizeAnnotationSnapshot(ctx, FinalizeAnnotationSnapshotCommand{WorkspaceID: extra.WorkspaceID, CampaignID: extra.CampaignID})
	if err != nil {
		t.Fatal(err)
	}
	if intact, err := f.repo.GetSnapshotIntegrity(ctx, snapshot.ID); err != nil || !intact {
		t.Fatalf("normal integrity=%v err=%v", intact, err)
	}
	if pre, err := f.service.GoldQualityPreflight(ctx, extra.WorkspaceID, extra.CampaignID); err != nil || pre.Blocking {
		t.Fatalf("normal Gold=%v err=%v", pre, err)
	}
}

func secondBatchSource(f *controlledFixture) d.SourceObservation {
	s := f.source
	s.ExternalID = "502"
	s.SubmissionRevision = 2
	s.ID = s.Identity()
	return s
}
func rawBatchResult(s d.SourceObservation) d.Result {
	return d.Result{ID: uuid.New(), WorkspaceID: s.WorkspaceID, CampaignID: s.CampaignID, TaskID: s.TaskID,
		AuthorRef: s.CoreAuthorRef, ProviderBindingRef: s.CampaignBindingID.String(), ExternalTaskID: s.ExternalTaskID,
		ExternalAnnotationID: s.ExternalAnnotationID, ExternalRevision: s.ExternalRevision(), ObservationKey: uuid.NewString(),
		CanonicalPayload: s.CanonicalPayload, CanonicalPayloadSHA256: s.CanonicalPayloadSHA256, NormalizerVersion: s.NormalizerVersion, CreatedAt: time.Now().UTC()}
}
func TestControlledOriginalBindingRejectsOutOfSetDirectDB(t *testing.T) {
	for _, completed := range []bool{false, true} {
		t.Run(map[bool]string{false: "before-complete", true: "after-complete"}[completed], func(t *testing.T) {
			f := newControlledFixture(t)
			ctx := t.Context()
			if _, err := f.service.RecordAnnotationResult(ctx, f.command(f.source, "valid")); err != nil {
				t.Fatal(err)
			}
			if completed {
				f.complete(t)
			}
			s := secondBatchSource(f)
			tx, err := f.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			// Source-only observation remains legal, but binding it to an original
			// Result outside the frozen slots must abort the whole transaction.
			if err := f.repo.StoreSourceTx(ctx, tx, s); err != nil {
				t.Fatal(err)
			}
			r := rawBatchResult(s)
			if err := f.repo.InsertResult(ctx, tx, r); err != nil {
				t.Fatal(err)
			}
			err = f.repo.BindSourceResultTx(ctx, tx, r.ID, s)
			if err == nil || !strings.Contains(err.Error(), "invalid exact SourceResultBinding") {
				t.Fatalf("out-of-set DB binding=%v", err)
			}
			if err := tx.Rollback(ctx); err != nil {
				t.Fatal(err)
			}
			var n int
			if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM annotation_result WHERE campaign_id=$1", s.CampaignID).Scan(&n); err != nil || n != 1 {
				t.Fatalf("DB rollback membership=%d err=%v", n, err)
			}
			if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM annotation_source_observation WHERE id=$1", s.ID).Scan(&n); err != nil || n != 0 {
				t.Fatalf("out-of-set source survived rollback=%d err=%v", n, err)
			}
		})
	}
}

// Privileged corruption simulation is scoped to this random synthetic fixture;
// ordinary receipt writes are immutable. It exercises stale/tampered evidence
// at every downstream gate without weakening its production constraints.
func corruptCompleteReceipt(t *testing.T, f *controlledFixture) {
	t.Helper()
	ctx := t.Context()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SET LOCAL session_replication_role=replica"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "UPDATE annotation_submission_batch_receipt SET source_ids=$2 WHERE campaign_id=$1 AND outcome='COMPLETE'", f.source.CampaignID, []uuid.UUID{uuid.New()}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}
func batchAcceptCommand(f *controlledFixture, resultID uuid.UUID) ReviewAnnotationCommand {
	actor := uuid.New()
	return ReviewAnnotationCommand{WorkspaceID: f.source.WorkspaceID, CampaignID: f.source.CampaignID,
		TaskID: f.source.TaskID, ExpectedTaskRevision: 2, ReviewerRef: actor.String(), Action: d.ReviewAccept,
		Reason: "exact batch regression", IdempotencyKey: uuid.NewString(), ReviewedResultID: &resultID, ActorID: &actor}
}
func TestControlledReceiptMembershipRecheckedAtReviewSealAndGold(t *testing.T) {
	for _, phase := range []string{"review", "seal", "read-gold"} {
		t.Run(phase, func(t *testing.T) {
			f := newControlledFixture(t)
			ctx := t.Context()
			root, err := f.service.RecordAnnotationResult(ctx, f.command(f.source, "root"))
			if err != nil {
				t.Fatal(err)
			}
			f.complete(t)
			cmd := batchAcceptCommand(f, root.ID)
			fp, err := reviewFingerprint(cmd)
			if err != nil {
				t.Fatal(err)
			}
			attempt, err := f.service.ensureReviewAttempt(ctx, cmd, fp)
			if err != nil {
				t.Fatal(err)
			}
			var snapshot d.Snapshot
			if phase != "review" {
				if _, err := f.service.ReviewAnnotation(ctx, cmd); err != nil {
					t.Fatal(err)
				}
			}
			if phase == "read-gold" {
				snapshot, err = f.service.FinalizeAnnotationSnapshot(ctx, FinalizeAnnotationSnapshotCommand{WorkspaceID: f.source.WorkspaceID, CampaignID: f.source.CampaignID})
				if err != nil {
					t.Fatal(err)
				}
			}
			corruptCompleteReceipt(t, f)
			switch phase {
			case "review":
				tx, err := f.pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				decision := d.ReviewDecision{ID: uuid.New(), WorkspaceID: cmd.WorkspaceID, CampaignID: cmd.CampaignID,
					TaskID: cmd.TaskID, ReviewAttemptID: attempt.ID, ReviewedResultID: &root.ID, SelectedResultID: &root.ID,
					ReviewerRef: cmd.ReviewerRef, Outcome: d.ReviewAccept, Reason: cmd.Reason, ExpectedTaskRevision: 2, CreatedAt: time.Now().UTC()}
				err = f.repo.InsertReviewDecision(ctx, tx, decision)
				_ = tx.Rollback(ctx)
				if err == nil || !strings.Contains(err.Error(), "does not cover exact current membership") {
					t.Fatalf("direct DB review=%v", err)
				}
				if _, err := f.service.ReviewAnnotation(ctx, cmd); !errors.Is(err, d.ErrSourceIntegrity) {
					t.Fatalf("Command review stale receipt=%v", err)
				}
			case "seal":
				if _, err := f.service.FinalizeAnnotationSnapshot(ctx, FinalizeAnnotationSnapshotCommand{WorkspaceID: f.source.WorkspaceID, CampaignID: f.source.CampaignID}); !errors.Is(err, d.ErrSourceIntegrity) {
					t.Fatalf("Command seal stale receipt=%v", err)
				}
				manifest := []byte(`{}`)
				_, err := f.pool.Exec(ctx, `INSERT INTO annotation_snapshot(id,workspace_id,campaign_id,manifest,manifest_hash_payload,root_hash,expected_task_count,expected_result_count,expected_decision_count,expected_output_count) VALUES($1,$2,$3,$4,$5,$6,1,1,1,1)`, uuid.New(), f.source.WorkspaceID, f.source.CampaignID, manifest, manifest, d.SourceDigest(manifest))
				if err == nil || !strings.Contains(err.Error(), "does not cover exact current membership") {
					t.Fatalf("direct DB snapshot=%v", err)
				}
			case "read-gold":
				if intact, err := f.repo.GetSnapshotIntegrity(ctx, snapshot.ID); err == nil && intact {
					t.Fatal("tampered COMPLETE passed read integrity")
				}
				if pre, err := f.service.GoldQualityPreflight(ctx, f.source.WorkspaceID, f.source.CampaignID); err == nil && !pre.Blocking {
					t.Fatal("tampered COMPLETE passed Gold")
				}
			}
		})
	}
}

func TestControlledOutOfSetWriterFirstCannotRaceReviewOrSeal(t *testing.T) {
	for _, phase := range []string{"review", "seal"} {
		t.Run(phase, func(t *testing.T) {
			f := newControlledFixture(t)
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			root, err := f.service.RecordAnnotationResult(ctx, f.command(f.source, "root"))
			if err != nil {
				t.Fatal(err)
			}
			f.complete(t)
			reviewCmd := batchAcceptCommand(f, root.ID)
			fp, err := reviewFingerprint(reviewCmd)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.service.ensureReviewAttempt(ctx, reviewCmd, fp); err != nil {
				t.Fatal(err)
			}
			if phase == "seal" {
				if _, err := f.service.ReviewAnnotation(ctx, reviewCmd); err != nil {
					t.Fatal(err)
				}
			}
			cfg, err := pgxpool.ParseConfig(os.Getenv("TEST_POSTGRES_DSN"))
			if err != nil {
				t.Fatal(err)
			}
			name := "dpp294-membership-" + uuid.NewString()
			cfg.ConnConfig.RuntimeParams["application_name"] = name
			cfg.MaxConns = 2
			writerPool, err := pgxpool.NewWithConfig(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer writerPool.Close()
			writer := NewService(transaction.NewManager(writerPool), infra.NewRepository(writerPool), f.guard)
			actionCfg, err := pgxpool.ParseConfig(os.Getenv("TEST_POSTGRES_DSN"))
			if err != nil {
				t.Fatal(err)
			}
			actionName := name + "-action"
			actionCfg.ConnConfig.RuntimeParams["application_name"] = actionName
			actionCfg.MaxConns = 2
			actionPool, err := pgxpool.NewWithConfig(ctx, actionCfg)
			if err != nil {
				t.Fatal(err)
			}
			defer actionPool.Close()
			actionService := NewService(transaction.NewManager(actionPool), infra.NewRepository(actionPool), f.guard)
			hold, err := f.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer hold.Rollback(context.Background())
			if _, err := f.repo.LockCampaignTx(ctx, hold, f.source.CampaignID); err != nil {
				t.Fatal(err)
			}
			record := f.command(secondBatchSource(f), "writer-first-outset")
			record.ExpectedTaskRevision = 2
			recorded := make(chan error, 1)
			go func() { _, err := writer.RecordAnnotationResult(ctx, record); recorded <- err }()
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
					t.Fatal("writer did not reach parent fence")
				}
				time.Sleep(10 * time.Millisecond)
			}
			progressed := make(chan error, 1)
			go func() {
				if phase == "review" {
					_, err := actionService.ReviewAnnotation(ctx, reviewCmd)
					progressed <- err
				} else {
					_, err := actionService.FinalizeAnnotationSnapshot(ctx, FinalizeAnnotationSnapshotCommand{WorkspaceID: f.source.WorkspaceID, CampaignID: f.source.CampaignID})
					progressed <- err
				}
			}()
			// Observe both independent transactions waiting on the same parent
			// before releasing it, with the writer queued first.
			deadline = time.Now().Add(5 * time.Second)
			for {
				var waiting bool
				if err := f.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name=$1 AND wait_event_type='Lock')", actionName).Scan(&waiting); err != nil {
					t.Fatal(err)
				}
				if waiting {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("review/seal did not reach shared parent fence")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if err := hold.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err := <-recorded; !errors.Is(err, d.ErrSourceIntegrity) {
				t.Fatalf("writer-first outside membership=%v", err)
			}
			if err := <-progressed; err != nil {
				t.Fatalf("%s blocked by rejected source=%v", phase, err)
			}
			snapshot, err := f.service.FinalizeAnnotationSnapshot(ctx, FinalizeAnnotationSnapshotCommand{WorkspaceID: f.source.WorkspaceID, CampaignID: f.source.CampaignID})
			if err != nil {
				t.Fatal(err)
			}
			var manifest struct{ SourceClosure []infra.SourceClosure }
			if err := json.Unmarshal(snapshot.Manifest, &manifest); err != nil || len(manifest.SourceClosure) != 1 {
				t.Fatalf("raced closure=%+v err=%v", manifest, err)
			}
		})
	}
}
