package application

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/certification/domain"
	certificationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
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
