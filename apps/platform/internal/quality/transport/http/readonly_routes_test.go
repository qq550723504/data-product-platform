package qualityhttp

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadOnlyRegistrationDoesNotExposeEvaluationCommands(t *testing.T) {
	mux := http.NewServeMux()
	NewHandler(nil, nil).RegisterReadOnly(mux)
	for _, path := range []string{"/api/v1/dataset-versions/4c208c9b-d4d7-42ad-a119-101637d99e77/quality-checks", "/api/v1/dataset-versions/4c208c9b-d4d7-42ad-a119-101637d99e77/gold-quality-checks"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("POST", path, nil))
		if w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("write route exposed: %s -> %d", path, w.Code)
		}
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/dataset-versions/4c208c9b-d4d7-42ad-a119-101637d99e77/quality-assessments?workspaceId=11111111-1111-4111-8111-111111111111", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("read route must authenticate before repository: %d", w.Code)
	}
}
