package contracthttp

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/contract/domain"
	q "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/readmodel"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type sourceReader struct {
	calls int
	err   error
	query q.Query
}

func (f *sourceReader) ReadAcceptanceSources(_ context.Context, x q.Query) (q.Snapshot, error) {
	f.calls++
	f.query = x
	if f.err != nil {
		return q.Snapshot{}, f.err
	}
	now := time.Now().UTC()
	r := q.Snapshot{Total: 1, Versions: []q.StoredVersion{{WorkspaceID: x.WorkspaceID, Name: "SOURCE_TEST", Version: domain.ContractVersion{ID: uuid.MustParse("44444444-4444-4444-8444-444444444444"), ContractID: uuid.New(), Status: domain.VersionPublished, SourceRef: "private-source-location", SourceSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CreatedAt: now, PublishedAt: &now, Document: map[string]any{"secret": "private-document-sentinel"}}}}}
	r.EvaluatedAt = now
	if x.DeliveryID != nil {
		r.Delivery = &q.Delivery{ID: *x.DeliveryID, DatasetVersionID: *x.DatasetVersionID, Status: "BLOCKED"}
	}
	return r, nil
}
func TestAcceptanceReadonlyAuthorityBeforeRepository(t *testing.T) {
	ws := "11111111-1111-4111-8111-111111111111"
	id := "44444444-4444-4444-8444-444444444444"
	root := "/api/v1/workspaces/" + ws
	resolver, _ := principal.NewStaticResolver(true, "test-token", "operator", "22222222-2222-4222-8222-222222222222", []string{ws}, []string{principal.CapabilityHumanDecision})
	reader := &sourceReader{}
	h := ReadOnlyHandler{Reader: reader, Principal: resolver}
	mux := http.NewServeMux()
	h.Register(mux)
	for _, tc := range []struct {
		method, path, token string
		status              int
	}{
		{"GET", root + "/contract-versions", "", 401}, {"GET", "/api/v1/workspaces/33333333-3333-4333-8333-333333333333/contract-versions", "test-token", 403},
		{"GET", "/api/v1/contract-versions/" + id, "test-token", 400}, {"GET", "/api/v1/contract-versions/" + id + "?workspaceId=" + ws, "", 401},
		{"GET", root + "/contract-versions?offset=-1", "test-token", 400}, {"GET", root + "/contract-versions?limit=25&limit=25", "test-token", 400}, {"GET", root + "/contract-versions?actorId=" + id, "test-token", 400},
		{"GET", root + "/acceptance-context?deliveryOperationId=" + id, "test-token", 400}, {"GET", root + "/acceptance-context?deliveryOperationId=" + id + "&datasetVersionId=" + id + "&contractVersionId=bad", "test-token", 400},
		{"POST", root + "/acceptance-context", "test-token", 405},
		{"GET", root + "/acceptance-context?deliveryOperationId=" + id + "&datasetVersionId=" + id + "&bindingOffset=-1", "test-token", 400},
		{"GET", root + "/acceptance-context?deliveryOperationId=" + id + "&datasetVersionId=" + id + "&bindingId=bad", "test-token", 400},
		{"GET", root + "/acceptance-context?deliveryOperationId=" + id + "&datasetVersionId=" + id + "&bindingId=" + id + "&bindingId=" + id, "test-token", 400},
	} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		r.Header.Set("X-Actor-ID", id)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
	}
	if reader.calls != 0 {
		t.Fatal("denied request reached repository")
	}
	for _, path := range []string{root + "/contract-versions", "/api/v1/contract-versions/" + id + "?workspaceId=" + ws, root + "/acceptance-context?deliveryOperationId=" + id + "&datasetVersionId=" + id} {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != 200 || w.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		var value map[string]any
		if json.Unmarshal(w.Body.Bytes(), &value) != nil {
			t.Fatal("invalid response")
		}
		if _, ok := value["document"]; ok {
			t.Fatal("raw document leaked")
		}
		if _, ok := value["sourceRef"]; ok {
			t.Fatal("source location leaked")
		}
		if path == root+"/contract-versions" && reader.query.WorkspaceID.String() != ws {
			t.Fatal("scope missing")
		}
		if reader.query.ActorID.String() != "22222222-2222-4222-8222-222222222222" || reader.query.Subject != "operator" {
			t.Fatal("trusted caller omitted or replaced by header")
		}
	}
	reader.err = q.ErrNotFound
	r := httptest.NewRequest("GET", root+"/contract-versions", nil)
	r.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatal("missing source fell back")
	}
}
