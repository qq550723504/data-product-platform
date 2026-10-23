package infrastructure_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/migration"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	qualityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/infrastructure"
	qualityhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/transport/http"
)

func TestQualityReportIsBoundedAndWorkspaceScoped(t *testing.T) {
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

	if err := migration.NewRunner(pool, migrationsDir(t)).Up(ctx); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	workspaceID := uuid.New()
	foreignWorkspaceID := uuid.New()
	datasetID := uuid.New()
	versionID := uuid.New()
	assessmentID := uuid.New()
	actorID := uuid.New()
	createdAt := time.Date(2026, 9, 21, 3, 0, 0, 0, time.UTC)
	ruleContent := "apiVersion: quality/v1\nkind: QualityRuleSet\nmetadata:\n  version: 1.0.0\n"
	ruleHash := sha256.Sum256([]byte(ruleContent))
	metrics, err := json.Marshal(map[string]any{
		"R-1": map[string]any{
			"affectedCount": 999999,
			"sample":        []any{map[string]any{"row": 999999}},
		},
		"dimensions": map[string]any{
			"COMPLETENESS": map[string]any{
				"dimension":      "COMPLETENESS",
				"status":         "PASS",
				"ruleCount":      3,
				"evaluatedCount": 3,
				"failedCount":    0,
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal metrics: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin fixture transaction: %v", err)
	}
	defer tx.Rollback(ctx) // The historical rows intentionally remain when commit succeeds.

	if _, err := tx.Exec(ctx, `
		INSERT INTO dataset (id, workspace_id, code, name, dataset_type, metadata, created_at, created_by, updated_at, updated_by)
		VALUES ($1,$2,$3,$4,'CURATED','{}'::jsonb,$5,$6,$5,$6)
	`, datasetID, workspaceID, "QR-"+assessmentID.String(), "Quality Report Fixture", createdAt, actorID); err != nil {
		t.Fatalf("insert dataset: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO dataset_version (id, dataset_id, version_no, status, storage_uri, checksum_value, metadata, created_at, created_by, ready_at)
		VALUES ($1,$2,1,'READY','s3://quality-report-fixture/data.csv','fixture-checksum','{}'::jsonb,$3,$4,$3)
	`, versionID, datasetID, createdAt, actorID); err != nil {
		t.Fatalf("insert dataset version: %v", err)
	}
	observed, err := json.Marshal(map[string]any{
		"observedValue": 0,
		"threshold":     1,
		"affectedCount": 2,
		"sample":        []any{map[string]any{"row": 7, "field": "company_id"}},
	})
	if err != nil {
		t.Fatalf("marshal finding observation: %v", err)
	}
	for _, finding := range []struct {
		id       uuid.UUID
		ruleID   string
		severity string
		status   string
	}{
		{id: uuid.New(), ruleID: "R-1", severity: "WARNING", status: "PASS"},
		{id: uuid.New(), ruleID: "R-2", severity: "CRITICAL", status: "FAIL"},
		{id: uuid.New(), ruleID: "R-3", severity: "WARNING", status: "SKIPPED"},
	} {
		if _, err := tx.Exec(ctx, `
			INSERT INTO quality_finding (id, result_id, rule_id, dimension, severity, status, observed, message, created_at)
			VALUES ($1,$2,$3,'COMPLETENESS',$4,$5,$6,'fixture finding',$7)
		`, finding.id, assessmentID, finding.ruleID, finding.severity, finding.status, observed, createdAt); err != nil {
			t.Fatalf("insert finding %s: %v", finding.ruleID, err)
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO quality_result (
			id, workspace_id, dataset_version_id, rule_set_ref, rule_set_version,
			rule_set_content_sha256, rule_set_content, evaluator_name, evaluator_version,
			gate_decision, metrics, created_at, created_by
		) VALUES ($1,$2,$3,'fixture/quality.yaml','1.0.0',$4,$5,'fixture-evaluator','1', 'FAIL',$6,$7,$8)
	`, assessmentID, workspaceID, versionID, hex.EncodeToString(ruleHash[:]), ruleContent, metrics, createdAt, actorID); err != nil {
		t.Fatalf("insert quality result: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_event (id, workspace_id, actor_type, actor_id, action, object_type, object_id, metadata, occurred_at)
		VALUES ($1,$2,'USER',$3,'QUALITY_CHECK_COMPLETED','QUALITY_RESULT',$4,'{}'::jsonb,$5)
	`, uuid.New(), workspaceID, actorID, assessmentID, createdAt); err != nil {
		t.Fatalf("insert audit event: %v", err)
	}
	if _, err := evidence.Append(ctx, tx, evidence.Record{
		WorkspaceID:  workspaceID,
		EvidenceType: "QUALITY_RESULT",
		Title:        "Quality report fixture evidence",
		SourceType:   "QUALITY_RESULT",
		SourceID:     &assessmentID,
		Metadata:     map[string]any{"fixture": true},
		CreatedAt:    createdAt,
		CreatedBy:    &actorID,
	}, evidence.Relation{ObjectType: "QUALITY_RESULT", ObjectID: assessmentID, RelationType: "EVIDENCE_FOR"}); err != nil {
		t.Fatalf("insert evidence: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit fixture: %v", err)
	}
	// An object reference alone must not pull another workspace's attachments
	// into either the detail endpoint or the bounded report.
	foreignTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer foreignTx.Rollback(ctx)
	if _, err := evidence.Append(ctx, foreignTx, evidence.Record{WorkspaceID: foreignWorkspaceID, EvidenceType: "QUALITY_RESULT", Title: "foreign-secret", SourceType: "QUALITY_RESULT", SourceID: &assessmentID, Metadata: map[string]any{"secret": "foreign-secret"}, CreatedAt: createdAt, CreatedBy: &actorID}, evidence.Relation{ObjectType: "QUALITY_RESULT", ObjectID: assessmentID, RelationType: "EVIDENCE_FOR"}); err != nil {
		t.Fatal(err)
	}
	if _, err := foreignTx.Exec(ctx, `INSERT INTO audit_event (id,workspace_id,actor_type,action,object_type,object_id,metadata,occurred_at) VALUES ($1,$2,'SYSTEM','FOREIGN_SECRET','QUALITY_RESULT',$3,'{"secret":"foreign-secret"}',$4)`, uuid.New(), foreignWorkspaceID, assessmentID, createdAt); err != nil {
		t.Fatal(err)
	}
	if err := foreignTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	repo := qualityinfra.NewPostgresRepository(pool)
	assessment, page, err := repo.GetReport(ctx, workspaceID, assessmentID, 1, 1)
	if err != nil {
		t.Fatalf("get quality report: %v", err)
	}
	if assessment.DatasetVersionID != versionID || assessment.RuleSetVersion != "1.0.0" {
		t.Fatalf("assessment provenance = %#v, want dataset version and rule version", assessment)
	}
	if assessment.RuleSetContent != "" {
		t.Fatal("bounded report query loaded the complete rule-set content")
	}
	if got := assessment.DimensionSummaries["COMPLETENESS"].Status; got != "PASS" {
		t.Fatalf("dimension status = %s, want persisted PASS snapshot", got)
	}
	if page.Total != 3 || len(page.Items) != 1 || page.Items[0].RuleID != "R-2" {
		t.Fatalf("finding page = total %d items %#v, want stable second item R-2", page.Total, page.Items)
	}

	if _, _, err := repo.GetReport(ctx, foreignWorkspaceID, assessmentID, 25, 0); !errors.Is(err, qualityinfra.ErrNotFound) {
		t.Fatalf("foreign workspace error = %v, want ErrNotFound", err)
	}

	mux := http.NewServeMux()
	resolver, err := principal.NewStaticResolver(true, "quality-report-secret", "quality-test", actorID.String(), []string{workspaceID.String(), foreignWorkspaceID.String()}, []string{principal.CapabilityHumanDecision})
	if err != nil {
		t.Fatal(err)
	}
	handler := qualityhttp.NewHandler(nil, repo, evidence.NewQueryRepository(pool))
	handler.Principal = resolver
	handler.Register(mux)
	server := httptest.NewServer(mux)
	defer server.Close()
	get := func(path string) (*http.Response, error) {
		req, err := http.NewRequest(http.MethodGet, server.URL+path, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer quality-report-secret")
		return server.Client().Do(req)
	}
	response, err := get("/api/v1/quality-assessments/" + assessmentID.String() + "/report?workspaceId=" + workspaceID.String() + "&limit=1&offset=1")
	if err != nil {
		t.Fatalf("request quality report: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("quality report status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	var report struct {
		Findings struct {
			Items []map[string]any `json:"items"`
			Page  struct {
				Total int `json:"total"`
			} `json:"page"`
		} `json:"findings"`
		Metrics     map[string]any   `json:"metrics"`
		Evidence    []map[string]any `json:"evidence"`
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	if err := json.NewDecoder(response.Body).Decode(&report); err != nil {
		t.Fatalf("decode quality report response: %v", err)
	}
	if report.Findings.Page.Total != 3 || len(report.Findings.Items) != 1 || report.Findings.Items[0]["ruleId"] != "R-2" {
		t.Fatalf("HTTP finding page = %#v, want total 3 and R-2", report.Findings)
	}
	if len(report.Evidence) != 1 || len(report.AuditEvents) != 1 {
		t.Fatalf("HTTP governance refs = evidence %d audit %d, want one each", len(report.Evidence), len(report.AuditEvents))
	}
	if _, ok := report.Metrics["R-1"]; ok {
		t.Fatal("HTTP report exposed unbounded per-rule metrics")
	}
	if _, ok := report.Evidence[0]["metadata"]; ok {
		t.Fatal("HTTP report exposed evidence metadata")
	}

	foreignResponse, err := get("/api/v1/quality-assessments/" + assessmentID.String() + "/report?workspaceId=" + foreignWorkspaceID.String())
	if err != nil {
		t.Fatalf("request foreign quality report: %v", err)
	}
	defer foreignResponse.Body.Close()
	if foreignResponse.StatusCode != http.StatusNotFound {
		t.Fatalf("foreign HTTP report status = %d, want %d", foreignResponse.StatusCode, http.StatusNotFound)
	}

	attemptID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO quality_assessment_attempt
		(id,workspace_id,dataset_version_id,rule_set_ref,engine_name,engine_version,started_at,lease_expires_at,created_by)
		VALUES ($1,$2,$3,'fixture/quality.yaml','fixture-evaluator','1',$4,$5,$6)`, attemptID, workspaceID, versionID, createdAt, createdAt.Add(time.Hour), actorID); err != nil {
		t.Fatal(err)
	}
	// All reads are authenticated, scoped and read only, including the old alias.
	var before string
	snapshot := `SELECT jsonb_build_array(
		(SELECT jsonb_agg(to_jsonb(q) ORDER BY id) FROM quality_result q WHERE workspace_id=$1),
		(SELECT jsonb_agg(to_jsonb(a) ORDER BY id) FROM quality_assessment_attempt a WHERE workspace_id=$1),
		(SELECT jsonb_agg(to_jsonb(c) ORDER BY id) FROM cost_event c WHERE workspace_id=$1)
	)::text`
	if err := pool.QueryRow(ctx, snapshot, workspaceID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/quality-results/" + assessmentID.String(),
		"/quality-assessments/" + assessmentID.String(),
		"/quality-assessments/" + assessmentID.String() + "/report",
		"/quality-assessment-attempts/" + attemptID.String(),
		"/dataset-versions/" + versionID.String() + "/quality-assessments",
		"/dataset-versions/" + versionID.String() + "/quality-assessments/latest",
	} {
		for _, scope := range []struct {
			ws     uuid.UUID
			status int
		}{{workspaceID, 200}, {foreignWorkspaceID, 404}} {
			res, err := get("/api/v1" + path + "?workspaceId=" + scope.ws.String())
			if err != nil {
				t.Fatal(err)
			}
			var payload any
			if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			res.Body.Close()
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "foreign-secret") || strings.Contains(string(encoded), "FOREIGN_SECRET") {
				t.Fatal("foreign attachment leaked")
			}
			if res.StatusCode != scope.status {
				t.Fatalf("%s: status=%d want=%d", path, res.StatusCode, scope.status)
			}
		}
	}
	// An authorized workspace with another workspace's version must not reach
	// either Command. The handler intentionally has a nil application service.
	for _, command := range []string{"quality-checks", "gold-quality-checks"} {
		req := httptest.NewRequest("POST", "/api/v1/dataset-versions/"+versionID.String()+"/"+command, strings.NewReader(`{"workspaceId":"`+foreignWorkspaceID.String()+`","assessmentAttemptId":"`+uuid.NewString()+`"}`))
		req.Header.Set("Authorization", "Bearer quality-report-secret")
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		if res.Code != 404 {
			t.Fatalf("foreign version %s: %d %s", command, res.Code, res.Body)
		}
	}
	var after string
	if err := pool.QueryRow(ctx, snapshot, workspaceID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("read or rejected command changed Quality/attempt/cost facts")
	}
	var assessments, attempts int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM quality_result WHERE workspace_id=$1), (SELECT count(*) FROM quality_assessment_attempt WHERE workspace_id=$1)`, workspaceID).Scan(&assessments, &attempts); err != nil {
		t.Fatal(err)
	}
	if assessments != 1 || attempts != 1 {
		t.Fatalf("GET mutated facts: assessments=%d attempts=%d", assessments, attempts)
	}
}

func migrationsDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test source path")
	}
	return filepath.Join(filepath.Clean(filepath.Join(filepath.Dir(file), "../../../../../")), "migrations")
}
