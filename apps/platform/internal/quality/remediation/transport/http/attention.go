package remediationhttp

import (
	"net/http"
	"strconv"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/remediation/infrastructure"
)

func (h *Handler) attention(w http.ResponseWriter, r *http.Request) {
	c, ok := h.authority(w, r)
	if !ok {
		return
	}
	view := r.URL.Query().Get("view")
	if view == "" {
		view = "FOLLOW_UP"
	}
	if !infrastructure.ValidAttentionView(view) {
		failure(w, r, 400, "INVALID_ATTENTION_VIEW")
		return
	}
	limit, offset := 25, 0
	for _, param := range []struct {
		name  string
		value *int
	}{{"limit", &limit}, {"offset", &offset}} {
		if raw := r.URL.Query().Get(param.name); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 0 || (param.name == "limit" && (n < 1 || n > 100)) || n > 2147483647 {
				failure(w, r, 400, "INVALID_ATTENTION_PAGINATION")
				return
			}
			*param.value = n
		}
	}
	if h.Repo == nil || h.Repo.Pool == nil {
		failure(w, r, 503, "QUALITY_ATTENTION_UNAVAILABLE")
		return
	}
	page, err := h.Repo.Attention(r.Context(), c.WorkspaceID, view, limit, offset)
	if err != nil {
		failure(w, r, 503, "QUALITY_ATTENTION_UNAVAILABLE")
		return
	}
	respond(w, page)
}
