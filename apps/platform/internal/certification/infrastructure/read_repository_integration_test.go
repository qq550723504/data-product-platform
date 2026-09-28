package infrastructure

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
)

func TestListDatasetCertificationHistoryPageIsBoundedAndStable(t *testing.T) {
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
	versionID := uuid.New()
	qualityID := uuid.New()
	profileID := uuid.New()
	profileRef := "history-page/" + uuid.NewString()

	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset (id, workspace_id, code, name, dataset_type)
		VALUES ($1,$2,$3,'Certification history page fixture','CURATED')
	`, datasetID, workspaceID, "CERT-HISTORY-"+uuid.NewString()); err != nil {
		t.Fatalf("insert dataset: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset_version (
			id, dataset_id, version_no, status, storage_type, storage_uri,
			content_type, checksum_algorithm, checksum_value, metadata, ready_at
		) VALUES ($1,$2,1,'READY','OBJECT_STORAGE','s3://certification-history-page',
			'text/csv','SHA256',repeat('a',64),'{}'::jsonb,now())
	`, versionID, datasetID); err != nil {
		t.Fatalf("insert dataset version: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO quality_result (
			id, workspace_id, dataset_version_id, rule_set_ref, rule_set_version,
			rule_set_content_sha256, rule_set_content, evaluator_name, evaluator_version,
			gate_decision, metrics
		) VALUES ($1,$2,$3,'certification/history.yaml','1.0.0',
			encode(digest(convert_to('certification-history','UTF8'),'sha256'),'hex'),
			'certification-history','test','1','PASS','{"dimensions":{}}'::jsonb)
	`, qualityID, workspaceID, versionID); err != nil {
		t.Fatalf("insert quality result: %v", err)
	}
	var profileHash string
	if err := pool.QueryRow(ctx, `
		INSERT INTO certification_profile (
			id, workspace_id, profile_ref, code, name, version,
			content_sha256, content_snapshot, purpose_mode, action_mode,
			consumer_mode, delivery_mode, quality_gate_required,
			rights_required, compliance_required, contract_required,
			traceability_required, evidence_required
		) VALUES ($1,$2,$3,'history-page','History Page','1',
			encode(digest(convert_to('{}','UTF8'),'sha256'),'hex'),'{}','ANY','ANY',
			'ANY','ANY',false,false,false,false,false,false)
		RETURNING content_sha256
	`, profileID, workspaceID, profileRef).Scan(&profileHash); err != nil {
		t.Fatalf("insert profile: %v", err)
	}

	first := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	second := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	third := uuid.MustParse("00000000-0000-0000-0000-000000000003")
	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	for _, item := range []struct {
		id       uuid.UUID
		issuedAt time.Time
	}{
		{id: first, issuedAt: base},
		{id: second, issuedAt: base.Add(time.Minute)},
		{id: third, issuedAt: base.Add(time.Minute)},
	} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO dataset_certification (
				id, workspace_id, dataset_version_id, quality_assessment_id,
				certification_profile_id, profile_ref, profile_version,
				profile_content_sha256, profile_content_snapshot, decision,
				blockers, reason, issued_at
			) VALUES ($1,$2,$3,$4,$5,$6,'1',$7,'{}','CERTIFIED','[]'::jsonb,'fixture',$8)
		`, item.id, workspaceID, versionID, qualityID, profileID, profileRef, profileHash, item.issuedAt); err != nil {
			t.Fatalf("insert certification %s: %v", item.id, err)
		}
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO certification_disposition (
			id, workspace_id, certification_id, disposition, effective_at, reason
		) VALUES
			($1,$2,$3,'REVOKED',$4,'current page disposition'),
			($5,$2,$6,'REVOKED',$4,'off page disposition')
	`, uuid.New(), workspaceID, third, base.Add(2*time.Minute), uuid.New(), first); err != nil {
		t.Fatalf("insert dispositions: %v", err)
	}

	repo := NewCertificationRepository(pool)
	page, err := repo.ListDatasetCertificationHistoryPage(
		ctx, workspaceID, versionID, base.Add(3*time.Minute), 2, 0, nil, nil,
	)
	if err != nil {
		t.Fatalf("list first history page: %v", err)
	}
	if page.Total != 3 || len(page.Rows) != 2 {
		t.Fatalf("page total/rows = %d/%d, want 3/2", page.Total, len(page.Rows))
	}
	if page.Rows[0].Certification.ID != third || page.Rows[1].Certification.ID != second {
		t.Fatalf("page order = [%s %s], want [%s %s]", page.Rows[0].Certification.ID, page.Rows[1].Certification.ID, third, second)
	}
	if len(page.Dispositions) != 1 || page.Dispositions[0].CertificationID != third {
		t.Fatalf("page dispositions = %#v, want only certification %s", page.Dispositions, third)
	}

	// Simulate a certification that commits after page 1. Runtime writes obtain
	// the next delivery-fence revision before insert, so its older issued_at must
	// not shift the offset boundary of the already anchored history snapshot.
	if _, err := pool.Exec(ctx, `
		INSERT INTO delivery_authorization_fence (workspace_id, revision)
		VALUES ($1,1)
		ON CONFLICT (workspace_id) DO UPDATE SET revision=1
	`, workspaceID); err != nil {
		t.Fatalf("advance fixture history revision: %v", err)
	}
	late := uuid.MustParse("00000000-0000-0000-0000-000000000004")
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset_certification (
			id, workspace_id, dataset_version_id, quality_assessment_id,
			certification_profile_id, profile_ref, profile_version,
			profile_content_sha256, profile_content_snapshot, decision,
			blockers, reason, issued_at, history_revision
		) VALUES ($1,$2,$3,$4,$5,$6,'1',$7,'{}','CERTIFIED','[]'::jsonb,'late commit',$8,1)
	`, late, workspaceID, versionID, qualityID, profileID, profileRef, profileHash, base.Add(2*time.Minute)); err != nil {
		t.Fatalf("insert late certification: %v", err)
	}

	next, err := repo.ListDatasetCertificationHistoryPage(
		ctx, workspaceID, versionID, base.Add(3*time.Minute), 2, 2, &page.AnchorRevision, nil,
	)
	if err != nil {
		t.Fatalf("list second history page: %v", err)
	}
	if next.Total != 3 || len(next.Rows) != 1 || next.Rows[0].Certification.ID != first {
		t.Fatalf("second page = total %d rows %#v, want only %s", next.Total, next.Rows, first)
	}
}
