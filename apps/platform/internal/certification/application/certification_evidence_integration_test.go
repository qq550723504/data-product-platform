package application

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/certification/domain"
	certificationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

type staticEvidenceResolver struct {
	input domain.EvaluationInput
}

func (r staticEvidenceResolver) Resolve(context.Context, EvaluateDatasetCertificationCommand, domain.ProfileSnapshot) (domain.EvaluationInput, error) {
	return r.input, nil
}

func TestCertificationAndDispositionAppendDecisionEvidence(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}

	ctx := context.Background()
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer pool.Close()

	workspaceID := uuid.New()
	datasetID := uuid.New()
	datasetVersionID := uuid.New()
	qualityID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset (id, workspace_id, code, name, dataset_type)
		VALUES ($1,$2,$3,'Certification evidence fixture','CURATED')
	`, datasetID, workspaceID, "CERT-EVIDENCE-"+uuid.NewString()); err != nil {
		t.Fatalf("insert dataset: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset_version (
			id, dataset_id, version_no, status, storage_type, storage_uri,
			content_type, checksum_algorithm, checksum_value, metadata, ready_at
		) VALUES ($1,$2,1,'READY','OBJECT_STORAGE','s3://certification-evidence-fixture',
			'text/csv','SHA256',repeat('c',64),'{}'::jsonb,now())
	`, datasetVersionID, datasetID); err != nil {
		t.Fatalf("insert dataset version: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO quality_result (
			id, workspace_id, dataset_version_id, rule_set_ref, rule_set_version,
			rule_set_content_sha256, rule_set_content, evaluator_name, evaluator_version,
			gate_decision, metrics
		) VALUES ($1,$2,$3,'certification/evidence.yaml','1.0.0',
			encode(digest(convert_to('certification-evidence','UTF8'),'sha256'),'hex'),
			'certification-evidence','test-evaluator','1','PASS','{"dimensions":{}}'::jsonb)
	`, qualityID, workspaceID, datasetVersionID); err != nil {
		t.Fatalf("insert quality assessment: %v", err)
	}

	profile, err := (domain.CertificationProfile{
		ProfileRef: "certification/evidence",
		Code:       "CERT-EVIDENCE",
		Name:       "Certification evidence profile",
		Version:    "1.0.0",
		Purpose:    domain.Applicability{Mode: domain.ApplicabilityAny},
		Actions:    domain.Applicability{Mode: domain.ApplicabilityAny},
		Consumers:  domain.Applicability{Mode: domain.ApplicabilityAny},
		Delivery:   domain.Applicability{Mode: domain.ApplicabilityAny},
	}).SnapshotForWorkspace(workspaceID, nil)
	if err != nil {
		t.Fatalf("snapshot certification profile: %v", err)
	}

	txManager := transaction.NewManager(pool)
	profileRepo := certificationinfra.NewProfileRepository(pool)
	if err := txManager.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return profileRepo.InsertProfile(ctx, tx, profile)
	}); err != nil {
		t.Fatalf("insert certification profile: %v", err)
	}

	certificationRepo := certificationinfra.NewCertificationRepository(pool)
	service := NewCertificationService(txManager, profileRepo, certificationRepo, staticEvidenceResolver{input: domain.EvaluationInput{
		WorkspaceID:          workspaceID,
		DatasetVersionID:     datasetVersionID,
		DatasetVersionStatus: "READY",
		Quality: domain.QualityAssessmentEvidence{
			ID:               qualityID,
			WorkspaceID:      workspaceID,
			DatasetVersionID: datasetVersionID,
			GateDecision:     domain.EvidencePass,
		},
	}})

	certification, err := service.Evaluate(ctx, EvaluateDatasetCertificationCommand{
		WorkspaceID:      workspaceID,
		DatasetVersionID: datasetVersionID,
		ProfileID:        profile.ID,
		IdempotencyKey:   "certification-evidence-1",
	})
	if err != nil {
		t.Fatalf("evaluate certification: %v", err)
	}
	assertEvidenceRelation(t, pool, "DATASET_CERTIFICATION", certification.ID, "DECISION_EVIDENCE")

	if _, err := service.ChangeDisposition(ctx, ChangeCertificationDispositionCommand{
		WorkspaceID:     workspaceID,
		CertificationID: certification.ID,
		Disposition:     domain.DispositionRevoked,
		Reason:          "fixture revocation",
		EffectiveAt:     certification.IssuedAt,
		IdempotencyKey:  "certification-disposition-evidence-1",
	}); err != nil {
		t.Fatalf("change certification disposition: %v", err)
	}
	var dispositionID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM certification_disposition WHERE certification_id=$1`, certification.ID).Scan(&dispositionID); err != nil {
		t.Fatalf("load certification disposition: %v", err)
	}
	assertEvidenceRelation(t, pool, "CERTIFICATION_DISPOSITION", dispositionID, "DISPOSITION_EVIDENCE")
}

func TestCertificationDispositionSerializesWithDeliveryAuthorizationFence(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer pool.Close()

	workspaceID := uuid.New()
	datasetID := uuid.New()
	datasetVersionID := uuid.New()
	qualityID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset (id, workspace_id, code, name, dataset_type)
		VALUES ($1,$2,$3,'Certification fence fixture','CURATED')
	`, datasetID, workspaceID, "CERT-FENCE-"+uuid.NewString()); err != nil {
		t.Fatalf("insert dataset: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset_version (
			id, dataset_id, version_no, status, storage_type, storage_uri,
			content_type, checksum_algorithm, checksum_value, metadata, ready_at
		) VALUES ($1,$2,1,'READY','OBJECT_STORAGE','s3://certification-fence-fixture',
			'text/csv','SHA256',repeat('d',64),'{}'::jsonb,now())
	`, datasetVersionID, datasetID); err != nil {
		t.Fatalf("insert dataset version: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO quality_result (
			id, workspace_id, dataset_version_id, rule_set_ref, rule_set_version,
			rule_set_content_sha256, rule_set_content, evaluator_name, evaluator_version,
			gate_decision, metrics
		) VALUES ($1,$2,$3,'certification/fence.yaml','1.0.0',
			encode(digest(convert_to('certification-fence','UTF8'),'sha256'),'hex'),
			'certification-fence','test-evaluator','1','PASS','{"dimensions":{}}'::jsonb)
	`, qualityID, workspaceID, datasetVersionID); err != nil {
		t.Fatalf("insert quality assessment: %v", err)
	}

	profile, err := (domain.CertificationProfile{
		ProfileRef: "certification/fence",
		Code:       "CERT-FENCE",
		Name:       "Certification fence profile",
		Version:    "1.0.0",
		Purpose:    domain.Applicability{Mode: domain.ApplicabilityAny},
		Actions:    domain.Applicability{Mode: domain.ApplicabilityAny},
		Consumers:  domain.Applicability{Mode: domain.ApplicabilityAny},
		Delivery:   domain.Applicability{Mode: domain.ApplicabilityAny},
	}).SnapshotForWorkspace(workspaceID, nil)
	if err != nil {
		t.Fatalf("snapshot certification profile: %v", err)
	}
	txManager := transaction.NewManager(pool)
	profileRepo := certificationinfra.NewProfileRepository(pool)
	if err := txManager.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return profileRepo.InsertProfile(ctx, tx, profile)
	}); err != nil {
		t.Fatalf("insert certification profile: %v", err)
	}
	certificationRepo := certificationinfra.NewCertificationRepository(pool)
	service := NewCertificationService(txManager, profileRepo, certificationRepo, staticEvidenceResolver{input: domain.EvaluationInput{
		WorkspaceID:          workspaceID,
		DatasetVersionID:     datasetVersionID,
		DatasetVersionStatus: "READY",
		Quality: domain.QualityAssessmentEvidence{
			ID: qualityID, WorkspaceID: workspaceID, DatasetVersionID: datasetVersionID, GateDecision: domain.EvidencePass,
		},
	}})
	certification, err := service.Evaluate(ctx, EvaluateDatasetCertificationCommand{
		WorkspaceID: workspaceID, DatasetVersionID: datasetVersionID, ProfileID: profile.ID,
		IdempotencyKey: "certification-fence-evaluate-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("evaluate certification: %v", err)
	}

	var revisionAfterCertification int64
	if err := pool.QueryRow(ctx, `SELECT revision FROM delivery_authorization_fence WHERE workspace_id=$1`, workspaceID).Scan(&revisionAfterCertification); err != nil {
		t.Fatalf("read fence after certification: %v", err)
	}
	if revisionAfterCertification < 1 {
		t.Fatalf("fence revision after new certification = %d, want >= 1", revisionAfterCertification)
	}

	lockTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin delivery fence holder: %v", err)
	}
	defer func() { _ = lockTx.Rollback(context.Background()) }()
	lockedRevision, err := deliveryfence.Lock(ctx, lockTx, workspaceID)
	if err != nil {
		t.Fatalf("lock delivery authorization fence: %v", err)
	}
	if lockedRevision != revisionAfterCertification {
		t.Fatalf("locked revision = %d, want %d", lockedRevision, revisionAfterCertification)
	}

	done := make(chan error, 1)
	go func() {
		_, changeErr := service.ChangeDisposition(context.Background(), ChangeCertificationDispositionCommand{
			WorkspaceID: workspaceID, CertificationID: certification.ID,
			Disposition: domain.DispositionRevoked, Reason: "concurrent revocation",
			EffectiveAt: certification.IssuedAt,
			IdempotencyKey: "certification-fence-disposition-" + uuid.NewString(),
		})
		done <- changeErr
	}()

	select {
	case err := <-done:
		t.Fatalf("certification disposition crossed a held delivery fence: %v", err)
	case <-time.After(200 * time.Millisecond):
		// Expected: the disposition transaction is blocked on the same row lock.
	}

	if err := lockTx.Commit(ctx); err != nil {
		t.Fatalf("commit delivery fence holder: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("change certification disposition after fence release: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("certification disposition did not complete after delivery fence release")
	}

	var revisionAfterDisposition int64
	if err := pool.QueryRow(ctx, `SELECT revision FROM delivery_authorization_fence WHERE workspace_id=$1`, workspaceID).Scan(&revisionAfterDisposition); err != nil {
		t.Fatalf("read fence after disposition: %v", err)
	}
	if revisionAfterDisposition != revisionAfterCertification+1 {
		t.Fatalf("fence revision after disposition = %d, want %d", revisionAfterDisposition, revisionAfterCertification+1)
	}
}

func assertEvidenceRelation(t *testing.T, pool *pgxpool.Pool, objectType string, objectID uuid.UUID, relationType string) {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*)
		FROM evidence_relation er
		JOIN evidence e ON e.id=er.evidence_id
		WHERE er.object_type=$1 AND er.object_id=$2 AND er.relation_type=$3
	`, objectType, objectID, relationType).Scan(&count); err != nil {
		t.Fatalf("count %s evidence: %v", objectType, err)
	}
	if count != 1 {
		t.Fatalf("%s evidence relation count = %d, want 1", objectType, count)
	}
}
