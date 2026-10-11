package workflowhttp

import (
	"errors"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestQuarantineRejectsUntrustedWorkspaceBeforeReadingPayload(t *testing.T) {
	for _, configured := range []bool{false, true} {
		h := NewHandler(nil, nil, nil)
		if configured {
			h.Authorize = func(*http.Request, uuid.UUID) (*uuid.UUID, error) { return nil, errors.New("foreign workspace") }
		}
		r := httptest.NewRequest("GET", "/quarantine", nil)
		r.SetPathValue("workspaceId", uuid.NewString())
		r.SetPathValue("executionId", uuid.NewString())
		r.Header.Set("X-Actor-ID", uuid.NewString())
		w := httptest.NewRecorder()
		h.quarantine(w, r)
		if w.Code != 401 && w.Code != 403 {
			t.Fatalf("untrusted payload read status %d", w.Code)
		}
	}
}
