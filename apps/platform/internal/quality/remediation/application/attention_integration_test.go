package application_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/remediation/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/remediation/infrastructure"
	remediationhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/remediation/transport/http"
)

// Reuses the existing C1 owner's real admission fixtures and in-process adapters.
// No provider, retained runtime, or new business state machine is involved.
func verifyAttentionProjection(t *testing.T, ctx context.Context, pool *pgxpool.Pool, service *application.Service, command application.ExecuteCommand, sourceVersion uuid.UUID, run func(uuid.UUID) error) {
	t.Helper()
	ws, actor := command.WorkspaceID, command.ActorID
	repo := infrastructure.NewRepository(pool)
	newCase := func(key string) application.Command {
		d, err := service.Detect(ctx, application.Command{WorkspaceID: ws, ActorID: actor, Key: key, Reason: "attention fixture"}, sourceVersion)
		if err != nil || d.CaseID == nil {
			t.Fatalf("attention case: %v", err)
		}
		return application.Command{WorkspaceID: ws, ActorID: actor, CaseID: *d.CaseID, Key: key, Reason: "attention fixture"}
	}
	wanted := map[uuid.UUID]bool{}
	for i := 0; i < 101; i++ {
		c := newCase(fmt.Sprintf("attention-page-%d", i))
		wanted[c.CaseID] = true
	}
	pending := newCase("attention-pending")
	pending.Key = "assign-first"
	if _, err := service.Assign(ctx, pending); err != nil {
		t.Fatal(err)
	}
	pending.Key = "assign-second"
	if _, err := service.Assign(ctx, pending); err != nil {
		t.Fatal(err)
	}
	execute := command
	execute.Command = pending
	execute.Key = "attention-execute"
	e, err := service.Execute(ctx, execute)
	if err != nil {
		t.Fatal(err)
	}
	if err := run(e.ID); err != nil {
		t.Fatal(err)
	}
	var link, outputVersion, outputDataset uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT l.id,e.output_dataset_version_id,e.output_dataset_id FROM quality_remediation_execution l JOIN execution e ON e.id=l.execution_id WHERE e.id=$1`, e.ID).Scan(&link, &outputVersion, &outputDataset); err != nil {
		t.Fatal(err)
	}
	originalQuality := service.Quality
	lost := &lostQualityResponse{inner: originalQuality}
	lost.drop.Store(true)
	service.Quality = lost
	pending.Key = "older-pending-recheck"
	_, err = service.Recheck(ctx, pending, link)
	service.Quality = originalQuality
	if err == nil {
		t.Fatal("expected response loss before association")
	}
	pending.Key = "newer-completed-recheck"
	latestRecheck, err := service.Recheck(ctx, pending, link)
	if err != nil {
		t.Fatal(err)
	}
	var expectedAssessment uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT assessment_id FROM quality_remediation_recheck_outcome WHERE id=$1`, latestRecheck).Scan(&expectedAssessment); err != nil {
		t.Fatal(err)
	}
	pending.Key = "close-with-pending"
	if _, err := service.Close(ctx, pending); err != nil {
		t.Fatal(err)
	}

	active := newCase("attention-active")
	active.Key = "assign"
	if _, err := service.Assign(ctx, active); err != nil {
		t.Fatal(err)
	}
	execute.Command = active
	execute.Key = "queued-execution"
	if _, err := service.Execute(ctx, execute); err != nil {
		t.Fatal(err)
	}
	active.Key = "close-with-active-execution"
	if _, err := service.Close(ctx, active); err != nil {
		t.Fatal(err)
	}

	fingerprint := func() map[string]string {
		result := map[string]string{}
		for _, table := range []string{"quality_remediation_case", "quality_remediation_detection", "quality_result", "execution", "cost_event", "evidence", "audit_event"} {
			var hash string
			if err := pool.QueryRow(ctx, `SELECT md5(COALESCE(jsonb_agg(to_jsonb(t) ORDER BY id)::text,'[]')) FROM `+table+` t WHERE workspace_id=$1`, ws).Scan(&hash); err != nil {
				t.Fatal(err)
			}
			result[table] = hash
		}
		for _, table := range []string{"quality_remediation_assignment", "quality_remediation_execution", "quality_remediation_recheck", "quality_remediation_close"} {
			var hash string
			if err := pool.QueryRow(ctx, `SELECT md5(COALESCE(jsonb_agg(to_jsonb(t) ORDER BY t.id)::text,'[]')) FROM `+table+` t JOIN quality_remediation_case c ON c.id=t.case_id WHERE c.workspace_id=$1`, ws).Scan(&hash); err != nil {
				t.Fatal(err)
			}
			result[table] = hash
		}
		var outcomeHash string
		if err := pool.QueryRow(ctx, `SELECT md5(COALESCE(jsonb_agg(to_jsonb(o) ORDER BY o.id)::text,'[]')) FROM quality_remediation_recheck_outcome o JOIN quality_remediation_recheck r ON r.id=o.id JOIN quality_remediation_case c ON c.id=r.case_id WHERE c.workspace_id=$1`, ws).Scan(&outcomeHash); err != nil {
			t.Fatal(err)
		}
		result["quality_remediation_recheck_outcome"] = outcomeHash
		// Scope to this fixture even when other packages share PostgreSQL in CI.
		var outboxHash string
		if err := pool.QueryRow(ctx, `SELECT md5(COALESCE(jsonb_agg(to_jsonb(t) ORDER BY id)::text,'[]')) FROM outbox_event t WHERE aggregate_id IN (SELECT id FROM quality_remediation_case WHERE workspace_id=$1 UNION SELECT id FROM execution WHERE workspace_id=$1 UNION SELECT id FROM quality_result WHERE workspace_id=$1 UNION SELECT v.id FROM dataset_version v JOIN dataset d ON d.id=v.dataset_id WHERE d.workspace_id=$1)`, ws).Scan(&outboxHash); err != nil {
			t.Fatal(err)
		}
		result["outbox_event"] = outboxHash
		return result
	}
	before := fingerprint()
	seen := map[uuid.UUID]bool{}
	var total, closedCount int
	for offset := 0; ; offset += 25 {
		page, err := repo.Attention(ctx, ws, "ALL", 25, offset)
		if err != nil {
			t.Fatal(err)
		}
		if offset == 0 {
			total, closedCount = page.Page.Total, page.Summary.ClosedCount
		}
		if total < 103 || page.Page.Total != total || page.Summary.ClosedCount != closedCount {
			t.Fatal("truncated/inconsistent summary", page.Page)
		}
		for _, item := range page.Items {
			if seen[item.ID] || item.WorkspaceID != ws {
				t.Fatal("duplicate or foreign case", item.ID)
			}
			seen[item.ID] = true
			delete(wanted, item.ID)
			if item.ID == pending.CaseID && (!item.ManuallyClosed || !item.PendingRecheck || !item.NeedsFollowUp || item.RecheckOutcome == nil || *item.RecheckOutcome != "ASSESSED" || item.RecheckAssessmentID == nil || item.RecheckVersionID == nil) {
				t.Fatal("older pending work hidden by closure/latest recheck", item)
			}
			if item.ID == pending.CaseID && (*item.RecheckVersionID != outputVersion || item.RecheckDatasetID == nil || *item.RecheckDatasetID != outputDataset || *item.RecheckAssessmentID != expectedAssessment || item.ActorID == nil || *item.ActorID != actor) {
				t.Fatal("report or assignee not bound to actual immutable facts", item)
			}
			if item.ID == active.CaseID && (!item.ManuallyClosed || !item.ActiveExecution || !item.NeedsFollowUp) {
				t.Fatal("accepted execution hidden by closure", item)
			}
		}
		if offset+len(page.Items) >= total {
			break
		}
	}
	if len(wanted) != 0 || len(seen) != total {
		t.Fatal("pagination omitted cases", len(wanted), len(seen), total)
	}
	for _, view := range []string{"CLOSED", "FOLLOW_UP"} {
		page, err := repo.Attention(ctx, ws, view, 100, 0)
		if err != nil {
			t.Fatal(err)
		}
		if view == "CLOSED" && page.Page.Total != closedCount {
			t.Fatal("closure count differs")
		}
		if view == "FOLLOW_UP" && page.Page.Total != page.Summary.FollowUpCount {
			t.Fatal("follow-up count differs")
		}
		for _, item := range page.Items {
			if view == "CLOSED" && !item.ManuallyClosed || view == "FOLLOW_UP" && !item.NeedsFollowUp {
				t.Fatal("wrong filtered membership")
			}
		}
	}
	empty, err := repo.Attention(ctx, uuid.New(), "ALL", 25, 0)
	if err != nil || empty.Page.Total != 0 || len(empty.Items) != 0 {
		t.Fatal("workspace isolation", err)
	}
	end, err := repo.Attention(ctx, ws, "ALL", 25, total+25)
	if err != nil || end.Page.Total != total || len(end.Items) != 0 {
		t.Fatal("empty page lost count", err)
	}
	resolver, err := principal.NewStaticResolver(true, "attention-integration-secret", "attention-integration", actor.String(), []string{ws.String()}, []string{principal.CapabilityHumanDecision})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	remediationhttp.NewHandler(service, repo, resolver).Register(mux)
	r := httptest.NewRequest("GET", "/api/v1/workspaces/"+ws.String()+"/quality-remediations/attention?view=ALL&offset=100&limit=25", nil)
	r.Header.Set("Authorization", "Bearer attention-integration-secret")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	var page infrastructure.AttentionPage
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || w.Code != 200 || page.Page.Total != total || page.Page.Offset != 100 {
		t.Fatal("real HTTP page", w.Code, w.Body.String(), err)
	}
	if !reflect.DeepEqual(before, fingerprint()) {
		t.Fatal("read projection changed business facts")
	}
	t.Logf("quality attention: %d real cases, complete pagination, closed pending/active visible; authenticated HTTP; business fingerprints unchanged", total)
}
