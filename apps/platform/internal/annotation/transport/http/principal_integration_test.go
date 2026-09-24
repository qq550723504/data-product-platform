package annotationhttp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	annotationapp "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/application"
	annotationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	platformprincipal "github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

type reviewIntegrationFixture struct {
	workspaceID uuid.UUID
	campaignID  uuid.UUID
	taskID      uuid.UUID
	resultID    uuid.UUID
}

func TestAnnotationReviewHTTPPersistsTrustedHumanDecisionFacts(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	router, err := routing.NewRouter(false)
	if err != nil {
		t.Fatalf("routing: %v", err)
	}
	outbox.ConfigureAppendObligation(router)

	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer pool.Close()

	repo := annotationinfra.NewRepository(pool)
	service := annotationapp.NewService(transaction.NewManager(pool), repo, nil)
	trustedActor := uuid.New()

	for _, tc := range []struct {
		name      string
		action    string
		corrected string
	}{
		{name: "accept", action: "ACCEPT"},
		{name: "reject", action: "REJECT"},
		{name: "correct", action: "CORRECT", corrected: `{"label":"B"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := seedReviewIntegrationFixture(t, ctx, pool)
			resolver, err := platformprincipal.NewStaticResolver(
				true, "review-secret", "reviewer:integration", trustedActor.String(),
				[]string{fx.workspaceID.String()}, []string{platformprincipal.CapabilityHumanDecision},
			)
			if err != nil {
				t.Fatalf("resolver: %v", err)
			}
			mux := http.NewServeMux()
			NewHandler(service, resolver).Register(mux)

			body := fmt.Sprintf(`{"expectedTaskRevision":2,"action":%q,"reason":"verified integration"`, tc.action)
			if tc.action != "REJECT" {
				body += fmt.Sprintf(`,"reviewedResultId":%q`, fx.resultID.String())
			}
			if tc.corrected != "" {
				body += `,"correctedPayload":` + tc.corrected
			}
			body += "}"

			recorder := postAnnotationReviewIntegration(
				mux, fx, "review-secret", "review-"+tc.name+"-"+uuid.NewString(), body, uuid.New(),
			)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}

			var decisionID uuid.UUID
			var outcome, reviewerRef string
			var reviewedResultID, selectedResultID *uuid.UUID
			if err := pool.QueryRow(ctx, `
				SELECT id, outcome, reviewer_ref, reviewed_result_id, selected_result_id
				FROM annotation_review_decision WHERE task_id=$1
			`, fx.taskID).Scan(&decisionID, &outcome, &reviewerRef, &reviewedResultID, &selectedResultID); err != nil {
				t.Fatalf("read decision: %v", err)
			}
			if outcome != tc.action || reviewerRef != trustedActor.String() {
				t.Fatalf("decision outcome/reviewer=%s/%s", outcome, reviewerRef)
			}
			if tc.action == "ACCEPT" {
				if reviewedResultID == nil || selectedResultID == nil || *reviewedResultID != fx.resultID || *selectedResultID != fx.resultID {
					t.Fatalf("accept selection reviewed=%v selected=%v", reviewedResultID, selectedResultID)
				}
			}
			if tc.action == "REJECT" && selectedResultID != nil {
				t.Fatalf("reject selected result=%v, want nil", selectedResultID)
			}
			if tc.action == "CORRECT" {
				if reviewedResultID == nil || *reviewedResultID != fx.resultID || selectedResultID == nil || *selectedResultID == fx.resultID {
					t.Fatalf("correct selection reviewed=%v selected=%v", reviewedResultID, selectedResultID)
				}
				var correctedFrom uuid.UUID
				var createdBy uuid.UUID
				var authorRef string
				if err := pool.QueryRow(ctx, `
					SELECT corrected_from_result_id, created_by, author_ref
					FROM annotation_result WHERE id=$1
				`, *selectedResultID).Scan(&correctedFrom, &createdBy, &authorRef); err != nil {
					t.Fatalf("read correction: %v", err)
				}
				if correctedFrom != fx.resultID || createdBy != trustedActor || authorRef != trustedActor.String() {
					t.Fatalf("correction provenance from=%s createdBy=%s author=%s", correctedFrom, createdBy, authorRef)
				}
			}

			assertAnnotationReviewCount(t, ctx, pool, 1, `
				SELECT count(*) FROM audit_event
				WHERE object_type='ANNOTATION_REVIEW_DECISION' AND object_id=$1
				  AND action='ANNOTATION_REVIEWED' AND actor_id=$2
			`, decisionID, trustedActor)
			assertAnnotationReviewCount(t, ctx, pool, 1, `
				SELECT count(*) FROM evidence
				WHERE evidence_type='ANNOTATION_REVIEW_DECISION' AND created_by=$1
				  AND metadata->>'decisionId'=$2
			`, trustedActor, decisionID.String())
			assertAnnotationReviewCount(t, ctx, pool, 1, `
				SELECT count(*) FROM outbox_event
				WHERE aggregate_type='ANNOTATION_TASK' AND aggregate_id=$1
				  AND event_type='AnnotationReviewed'
			`, fx.taskID)
			assertAnnotationReviewCount(t, ctx, pool, 1, `
				SELECT count(*) FROM annotation_task
				WHERE id=$1 AND status='REVIEWED' AND current_decision_id=$2
			`, fx.taskID, decisionID)
		})
	}
}

func TestAnnotationReviewHTTPRejectsUntrustedAndStaleWithoutDecision(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	router, err := routing.NewRouter(false)
	if err != nil {
		t.Fatalf("routing: %v", err)
	}
	outbox.ConfigureAppendObligation(router)
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer pool.Close()

	repo := annotationinfra.NewRepository(pool)
	service := annotationapp.NewService(transaction.NewManager(pool), repo, nil)
	fx := seedReviewIntegrationFixture(t, ctx, pool)
	actor := uuid.New()
	resolver, err := platformprincipal.NewStaticResolver(
		true, "review-secret", "reviewer:integration", actor.String(),
		[]string{fx.workspaceID.String()}, []string{platformprincipal.CapabilityHumanDecision},
	)
	if err != nil {
		t.Fatalf("resolver: %v", err)
	}
	mux := http.NewServeMux()
	NewHandler(service, resolver).Register(mux)
	valid := fmt.Sprintf(
		`{"expectedTaskRevision":2,"action":"ACCEPT","reason":"verified","reviewedResultId":%q}`,
		fx.resultID.String(),
	)

	t.Run("unauthenticated", func(t *testing.T) {
		rec := postAnnotationReviewIntegration(mux, fx, "", "unauth-"+uuid.NewString(), valid, uuid.New())
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		assertAnnotationReviewCount(t, ctx, pool, 0, "SELECT count(*) FROM annotation_review_attempt WHERE task_id=$1", fx.taskID)
	})

	t.Run("cross workspace", func(t *testing.T) {
		foreignResolver, err := platformprincipal.NewStaticResolver(
			true, "review-secret", "reviewer:foreign", uuid.NewString(),
			[]string{uuid.NewString()}, []string{platformprincipal.CapabilityHumanDecision},
		)
		if err != nil {
			t.Fatalf("foreign resolver: %v", err)
		}
		foreignMux := http.NewServeMux()
		NewHandler(service, foreignResolver).Register(foreignMux)
		rec := postAnnotationReviewIntegration(foreignMux, fx, "review-secret", "foreign-"+uuid.NewString(), valid, uuid.New())
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		assertAnnotationReviewCount(t, ctx, pool, 0, "SELECT count(*) FROM annotation_review_attempt WHERE task_id=$1", fx.taskID)
	})

	t.Run("stale", func(t *testing.T) {
		body := fmt.Sprintf(
			`{"expectedTaskRevision":1,"action":"ACCEPT","reason":"stale","reviewedResultId":%q}`,
			fx.resultID.String(),
		)
		rec := postAnnotationReviewIntegration(mux, fx, "review-secret", "stale-"+uuid.NewString(), body, uuid.New())
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "ANNOTATION_REVIEW_STALE") {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		assertAnnotationReviewCount(t, ctx, pool, 0, "SELECT count(*) FROM annotation_review_decision WHERE task_id=$1", fx.taskID)
		assertAnnotationReviewCount(t, ctx, pool, 1, `
			SELECT count(*) FROM annotation_review_attempt a
			JOIN annotation_review_attempt_outcome o ON o.attempt_id=a.id
			WHERE a.task_id=$1 AND o.outcome='STALE_CONFLICT'
		`, fx.taskID)
	})
}

func seedReviewIntegrationFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) reviewIntegrationFixture {
	t.Helper()
	workspaceID := uuid.New()
	resourceID := uuid.New()
	datasetID := uuid.New()
	versionID := uuid.New()
	qualityID := uuid.New()
	profileID := uuid.New()
	certificationID := uuid.New()
	campaignID := uuid.New()
	taskID := uuid.New()
	resultID := uuid.New()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	inputChecksum := strings.Repeat("1", 64)
	specContent := `{"kind":"single-label-v1","labels":["A","B"]}`
	specHash := annotationReviewSHA256([]byte(specContent))

	mustExecReviewFixture(t, ctx, pool, `
		INSERT INTO data_resource(id, workspace_id, code, name, resource_type, lifecycle_status)
		VALUES ($1,$2,$3,'annotation contribution','OTHER','READY')
	`, resourceID, workspaceID, "ANN-"+suffix)
	mustExecReviewFixture(t, ctx, pool, `
		INSERT INTO dataset(id, workspace_id, code, name, dataset_type)
		VALUES ($1,$2,$3,'annotation input','CURATED')
	`, datasetID, workspaceID, "DS-"+suffix)
	mustExecReviewFixture(t, ctx, pool, `
		INSERT INTO dataset_version(
			id, dataset_id, version_no, status, storage_type, storage_uri,
			content_type, row_count, checksum_algorithm, checksum_value, ready_at
		) VALUES ($1,$2,1,'READY','OBJECT','test://annotation-review','application/json',1,'SHA256',$3,now())
	`, versionID, datasetID, inputChecksum)

	ruleContent := "annotation-review-rules"
	mustExecReviewFixture(t, ctx, pool, `
		INSERT INTO quality_result(
			id, workspace_id, dataset_version_id, rule_set_ref, rule_set_version,
			gate_decision, metrics, rule_set_content_sha256, rule_set_content,
			evaluator_name, evaluator_version
		) VALUES ($1,$2,$3,'review-rules','1','PASS',$4,$5,$6,'fixture','1')
	`, qualityID, workspaceID, versionID, []byte(`{"dimensions":{}}`), annotationReviewSHA256([]byte(ruleContent)), ruleContent)

	profileContent := "annotation-review-profile"
	profileHash := annotationReviewSHA256([]byte(profileContent))
	mustExecReviewFixture(t, ctx, pool, `
		INSERT INTO certification_profile(
			id, workspace_id, profile_ref, code, name, version, content_sha256, content_snapshot,
			purpose_mode, action_mode, consumer_mode, delivery_mode,
			quality_gate_required, rights_required, compliance_required, contract_required,
			traceability_required, evidence_required, membership_state
		) VALUES ($1,$2,'review-profile',$3,'review profile','1',$4,$5,
		          'ANY','ANY','ANY','ANY',true,false,false,false,false,false,'DRAFT')
	`, profileID, workspaceID, "PROFILE-"+suffix, profileHash, profileContent)
	mustExecReviewFixture(t, ctx, pool, "UPDATE certification_profile SET membership_state='FINALIZED' WHERE id=$1", profileID)
	mustExecReviewFixture(t, ctx, pool, `
		INSERT INTO dataset_certification(
			id, workspace_id, dataset_version_id, quality_assessment_id,
			certification_profile_id, profile_ref, profile_version,
			profile_content_sha256, profile_content_snapshot,
			decision, blockers, reason, issued_at
		) VALUES ($1,$2,$3,$4,$5,'review-profile','1',$6,$7,'CERTIFIED','[]'::jsonb,'fixture',now())
	`, certificationID, workspaceID, versionID, qualityID, profileID, profileHash, profileContent)

	mustExecReviewFixture(t, ctx, pool, `
		INSERT INTO annotation_campaign(
			id, workspace_id, input_dataset_version_id, input_certification_id,
			annotation_contribution_resource_id, purpose, action,
			schema_ref, schema_version, schema_content_sha256, schema_content_snapshot,
			taxonomy_ref, taxonomy_version, taxonomy_content_sha256, taxonomy_content_snapshot,
			rubric_ref, rubric_version, rubric_content_sha256, rubric_content_snapshot,
			renderer_ref, renderer_version, renderer_content_sha256, renderer_content_snapshot,
			review_policy_ref, review_policy_version, review_policy_content_sha256, review_policy_content_snapshot
		) VALUES (
			$1,$2,$3,$4,$5,'gold-pilot','PROCESS',
			'schema','1',$6,$7,'taxonomy','1',$6,$7,'rubric','1',$6,$7,
			'renderer','1',$6,$7,'review','1',$6,$7
		)
	`, campaignID, workspaceID, versionID, certificationID, resourceID, specHash, specContent)
	mustExecReviewFixture(t, ctx, pool, `
		INSERT INTO annotation_task(
			id, workspace_id, campaign_id, source_item_ref, source_content_sha256,
			task_text_sha256, primary_annotator_ref
		) VALUES ($1,$2,$3,'row:1',$4,$5,'annotator')
	`, taskID, workspaceID, campaignID, strings.Repeat("a", 64), strings.Repeat("b", 64))
	mustExecReviewFixture(t, ctx, pool, `
		UPDATE annotation_campaign
		   SET status='ACTIVE', revision=2, expected_task_count=1,
		       task_manifest_hash=$2, input_checksum_sha256=$3, activated_at=now()
		 WHERE id=$1
	`, campaignID, strings.Repeat("c", 64), inputChecksum)

	payload := []byte(`{"label":"A"}`)
	mustExecReviewFixture(t, ctx, pool, `
		INSERT INTO annotation_result(
			id, workspace_id, campaign_id, task_id, author_ref,
			provider_binding_ref, external_task_id, external_annotation_id, external_revision,
			observation_key, canonical_payload, canonical_payload_sha256, normalizer_version
		) VALUES ($1,$2,$3,$4,'annotator','fixture-provider','review-task','review-annotation','1',$5,$6,$7,'fixture-v1')
	`, resultID, workspaceID, campaignID, taskID, "review:"+uuid.NewString(), payload, annotationReviewSHA256(payload))
	mustExecReviewFixture(t, ctx, pool, "UPDATE annotation_task SET status='REVIEWABLE', revision=2 WHERE id=$1", taskID)

	return reviewIntegrationFixture{workspaceID: workspaceID, campaignID: campaignID, taskID: taskID, resultID: resultID}
}

func mustExecReviewFixture(t *testing.T, ctx context.Context, execer *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := execer.Exec(ctx, query, args...); err != nil {
		t.Fatalf("review fixture SQL: %v", err)
	}
}

func postAnnotationReviewIntegration(
	handler http.Handler,
	fx reviewIntegrationFixture,
	token, key, body string,
	forgedActor uuid.UUID,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/workspaces/"+fx.workspaceID.String()+"/annotation-campaigns/"+fx.campaignID.String()+"/tasks/"+fx.taskID.String()+"/review",
		bytes.NewBufferString(body),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", key)
	request.Header.Set("X-Actor-ID", forgedActor.String())
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func assertAnnotationReviewCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want int, query string, args ...any) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx, query, args...).Scan(&got); err != nil {
		t.Fatalf("count review facts: %v", err)
	}
	if got != want {
		t.Fatalf("count=%d want=%d query=%s", got, want, query)
	}
}

func annotationReviewSHA256(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
