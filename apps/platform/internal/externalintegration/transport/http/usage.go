package http

import (
	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	app "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/application"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/domain"
	"net/http"
)

func (h *Handler) WithUsage(u *app.UsageImporter) *Handler { h.Usage = u; return h }
func (h *Handler) usage(fn endpoint) http.HandlerFunc {
	return h.wrap(func(w http.ResponseWriter, r *http.Request, ws uuid.UUID, c app.Caller) {
		if h.Usage == nil {
			fail(w, r, d.ErrUnsupported)
			return
		}
		if err := h.Usage.AuthorizeQuery(r.Context(), ws, c); err != nil {
			fail(w, r, err)
			return
		}
		fn(w, r, ws, c)
	})
}
func (h *Handler) createUsageSource(w http.ResponseWriter, r *http.Request, ws uuid.UUID, c app.Caller) {
	var spec d.UsageSourceSpec
	if !decode(w, r, &spec) {
		return
	}
	v, err := h.Usage.CreateSource(r.Context(), ws, c, key(r), spec)
	reply(w, r, v, err, 201)
}
func (h *Handler) requestUsagePull(w http.ResponseWriter, r *http.Request, ws uuid.UUID, c app.Caller) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct{}
	if !decode(w, r, &body) {
		return
	}
	v, err := h.Usage.RequestPull(r.Context(), ws, c, key(r), id)
	reply(w, r, v, err, 202)
}
func (h *Handler) usageSources(w http.ResponseWriter, r *http.Request, ws uuid.UUID, _ app.Caller) {
	l, o, ok := pagination(w, r)
	if !ok {
		return
	}
	v, err := h.Service.Repo.UsageSources(r.Context(), ws, l, o)
	reply(w, r, v, err, 200)
}
func (h *Handler) usageSource(w http.ResponseWriter, r *http.Request, ws uuid.UUID, _ app.Caller) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	v, err := h.Service.Repo.UsageSource(r.Context(), h.Service.Repo.Pool, ws, id, false)
	reply(w, r, v, err, 200)
}
func (h *Handler) usagePulls(w http.ResponseWriter, r *http.Request, ws uuid.UUID, _ app.Caller) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	l, o, ok := pagination(w, r)
	if !ok {
		return
	}
	if _, err := h.Service.Repo.UsageSource(r.Context(), h.Service.Repo.Pool, ws, id, false); err != nil {
		fail(w, r, err)
		return
	}
	v, err := h.Service.Repo.UsagePulls(r.Context(), ws, id, l, o)
	reply(w, r, v, err, 200)
}
func (h *Handler) usagePull(w http.ResponseWriter, r *http.Request, ws uuid.UUID, _ app.Caller) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	v, err := h.Service.Repo.UsagePull(r.Context(), h.Service.Repo.Pool, ws, id)
	reply(w, r, v, err, 200)
}
func (h *Handler) usageCalls(w http.ResponseWriter, r *http.Request, ws uuid.UUID, _ app.Caller) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	l, o, ok := pagination(w, r)
	if !ok {
		return
	}
	if _, err := h.Service.Repo.UsagePull(r.Context(), h.Service.Repo.Pool, ws, id); err != nil {
		fail(w, r, err)
		return
	}
	v, err := h.Service.Repo.UsageCalls(r.Context(), h.Service.Repo.Pool, ws, id, l, o)
	reply(w, r, v, err, 200)
}
func (h *Handler) usageEvents(w http.ResponseWriter, r *http.Request, ws uuid.UUID, _ app.Caller) {
	l, o, ok := pagination(w, r)
	if !ok {
		return
	}
	status := r.URL.Query().Get("status")
	if status != "" && status != "ACCEPTED" && status != "QUARANTINED" {
		fail(w, r, d.ErrInvalid)
		return
	}
	v, err := h.Service.Repo.UsageEvents(r.Context(), ws, status, l, o)
	reply(w, r, v, err, 200)
}
func (h *Handler) usageEvent(w http.ResponseWriter, r *http.Request, ws uuid.UUID, _ app.Caller) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	v, err := h.Service.Repo.UsageEvent(r.Context(), h.Service.Repo.Pool, ws, id, false)
	if err != nil {
		fail(w, r, err)
		return
	}
	dec, err := h.Service.Repo.UsageDecisions(r.Context(), h.Service.Repo.Pool, ws, id)
	if err != nil {
		fail(w, r, err)
		return
	}
	ev, err := evidence.NewQueryRepository(h.Service.Repo.Pool).ListReferencesForObject(r.Context(), "ExternalUsageEvent", id, ws, 3)
	reply(w, r, map[string]any{"event": v, "decisions": dec, "evidence": ev}, err, 200)
}
func (h *Handler) usageConflicts(w http.ResponseWriter, r *http.Request, ws uuid.UUID, _ app.Caller) {
	l, o, ok := pagination(w, r)
	if !ok {
		return
	}
	v, err := h.Service.Repo.UsageConflicts(r.Context(), ws, l, o)
	reply(w, r, v, err, 200)
}
func (h *Handler) recheckUsage(w http.ResponseWriter, r *http.Request, ws uuid.UUID, c app.Caller) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct{}
	if !decode(w, r, &body) {
		return
	}
	v, err := h.Usage.Recheck(r.Context(), ws, c, key(r), id)
	reply(w, r, v, err, 200)
}
