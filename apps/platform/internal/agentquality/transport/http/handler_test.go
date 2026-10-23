package agentqualityhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/agentquality"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
)

type fixtureReader struct {
	dataset agentquality.Dataset
	version agentquality.Version
	calls   int
	err     error
}

func (f *fixtureReader) Datasets(_ context.Context, ws uuid.UUID, limit, offset int) (agentquality.Page[agentquality.Dataset], error) {
	f.calls++
	items := []agentquality.Dataset{}
	if ws == f.dataset.WorkspaceID && offset == 0 {
		items = append(items, f.dataset)
	}
	return agentquality.Page[agentquality.Dataset]{Items: items, Page: agentquality.PageInfo{Limit: limit, Offset: offset, Total: 1}}, f.err
}
func (f *fixtureReader) Dataset(_ context.Context, id uuid.UUID) (agentquality.Dataset, error) {
	f.calls++
	if f.err != nil {
		return agentquality.Dataset{}, f.err
	}
	if id != f.dataset.ID {
		return agentquality.Dataset{}, agentquality.ErrNotFound
	}
	return f.dataset, nil
}
func (f *fixtureReader) Versions(_ context.Context, id uuid.UUID, limit, offset int) (agentquality.Page[agentquality.Version], error) {
	f.calls++
	return agentquality.Page[agentquality.Version]{Items: []agentquality.Version{f.version}, Page: agentquality.PageInfo{Limit: limit, Offset: offset, Total: 1}}, f.err
}
func (f *fixtureReader) Version(_ context.Context, id uuid.UUID) (agentquality.Version, error) {
	f.calls++
	if f.err != nil {
		return agentquality.Version{}, f.err
	}
	if id != f.version.ID {
		return agentquality.Version{}, agentquality.ErrNotFound
	}
	return f.version, nil
}
func resolver(t *testing.T, ws uuid.UUID, caps ...string) principal.Resolver {
	t.Helper()
	r, err := principal.NewStaticResolver(true, "test-only-secret", "service", uuid.NewString(), []string{ws.String()}, caps)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func call(h *Handler, method, path, token string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	h.Register(mux)
	r := httptest.NewRequest(method, path, nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	r.Header.Set("X-Actor-ID", uuid.NewString())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}
func TestAuthorityPrecedesAllReads(t *testing.T) {
	ws := uuid.New()
	path := "/api/v1/workspaces/" + ws.String() + "/agent-quality-context"
	for _, tc := range []struct {
		name   string
		r      principal.Resolver
		token  string
		status int
	}{
		{"unconfigured", nil, "test-only-secret", 401},
		{"missing", resolver(t, ws, principal.CapabilityHumanDecision), "", 401},
		{"wrong", resolver(t, ws, principal.CapabilityHumanDecision), "wrong", 401},
		{"workspace", resolver(t, uuid.New(), principal.CapabilityHumanDecision), "test-only-secret", 403},
		{"capability", resolver(t, ws, "OTHER"), "test-only-secret", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := call(&Handler{Principal: tc.r}, "GET", path, tc.token)
			if w.Code != tc.status {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
			if strings.Contains(w.Body.String(), "secret") {
				t.Fatal("secret exposed")
			}
		})
	}
}
func TestSelectionIsolationAndQueryContract(t *testing.T) {
	ws, ds, v := uuid.New(), uuid.New(), uuid.New()
	f := &fixtureReader{dataset: agentquality.Dataset{ID: ds, WorkspaceID: ws, Name: "source", Code: "SOURCE", DatasetType: "RAW"}, version: agentquality.Version{ID: v, DatasetID: ds, VersionNo: 3, Status: "READY", CreatedAt: time.Now()}}
	h := &Handler{Principal: resolver(t, ws, principal.CapabilityHumanDecision), Service: &agentquality.Service{Reader: f}}
	path := "/api/v1/workspaces/" + ws.String() + "/agent-quality-context"
	for _, query := range []string{"?limit=0", "?offset=-1", "?limit=101", "?versionId=bad", "?versionId=" + uuid.Nil.String(), "?datasetId=" + ds.String() + "&datasetId=" + ds.String(), "?workspaceId=" + ws.String()} {
		f.calls = 0
		w := call(h, "GET", path+query, "test-only-secret")
		if w.Code != 400 || f.calls != 0 {
			t.Fatalf("query=%s status=%d calls=%d", query, w.Code, f.calls)
		}
	}
	for _, query := range []string{"", "?datasetId=" + ds.String(), "?versionId=" + v.String(), "?datasetId=" + ds.String() + "&versionId=" + v.String()} {
		w := call(h, "GET", path+query, "test-only-secret")
		if w.Code != 200 {
			t.Fatalf("query=%s %d %s", query, w.Code, w.Body)
		}
		var c agentquality.Context
		if err := json.Unmarshal(w.Body.Bytes(), &c); err != nil {
			t.Fatal(err)
		}
		if c.ExecutionReadiness != "NOT_EVALUATED" || c.Mode != "READ_ONLY" || c.Template.Configured || len(c.Unverified) != 4 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("false readiness: %+v", c)
		}
	}
	for _, query := range []string{"?datasetId=" + uuid.NewString(), "?versionId=" + uuid.NewString(), "?datasetId=" + uuid.NewString() + "&versionId=" + v.String()} {
		if w := call(h, "GET", path+query, "test-only-secret"); w.Code != 404 {
			t.Fatalf("mismatch %d %s", w.Code, w.Body)
		}
	}
	f.dataset.WorkspaceID = uuid.New()
	if w := call(h, "GET", path+"?versionId="+v.String(), "test-only-secret"); w.Code != 404 || strings.Contains(w.Body.String(), `"name"`) {
		t.Fatalf("foreign source exposed: %d %s", w.Code, w.Body)
	}
	f.err = errors.New("postgres://private-secret/storage")
	if w := call(h, "GET", path, "test-only-secret"); w.Code != 503 || strings.Contains(w.Body.String(), "private-secret") {
		t.Fatalf("unsafe error: %d %s", w.Code, w.Body)
	}
	f.calls = 0
	if w := call(h, "POST", path, "test-only-secret"); w.Code != 405 || f.calls != 0 {
		t.Fatalf("write accepted: %d", w.Code)
	}
}
