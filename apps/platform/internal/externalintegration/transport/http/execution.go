package http

import (
	"github.com/google/uuid"
	app "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/application"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/domain"
	"net/http"
)

func (h *Handler) execute(w http.ResponseWriter, r *http.Request, ws uuid.UUID, c app.Caller) {
	h.executionCommand(w, r, ws, c, "EXECUTE")
}
func (h *Handler) retry(w http.ResponseWriter, r *http.Request, ws uuid.UUID, c app.Caller) {
	h.executionCommand(w, r, ws, c, "RETRY")
}
func (h *Handler) reconcile(w http.ResponseWriter, r *http.Request, ws uuid.UUID, c app.Caller) {
	h.executionCommand(w, r, ws, c, "RECONCILE")
}
func (h *Handler) executionCommand(w http.ResponseWriter, r *http.Request, ws uuid.UUID, c app.Caller, command string) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct{}
	if !decode(w, r, &body) {
		return
	}
	if h.Executor == nil {
		fail(w, r, d.ErrUnsupported)
		return
	}
	var rec d.Receipt
	var err error
	if command == "RECONCILE" {
		rec, err = h.Executor.Reconcile(r.Context(), ws, c, key(r), id)
	} else {
		rec, err = h.Executor.Prepare(r.Context(), ws, c, key(r), id, command == "RETRY")
	}
	reply(w, r, rec, err, 202)
}
func (h *Handler) providers(w http.ResponseWriter, r *http.Request, _ uuid.UUID, _ app.Caller) {
	if h.Executor == nil {
		fail(w, r, d.ErrUnsupported)
		return
	}
	reply(w, r, h.Executor.Registry.List(), nil, 200)
}
func (h *Handler) attempts(w http.ResponseWriter, r *http.Request, ws uuid.UUID, _ app.Caller) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	l, o, ok := pagination(w, r)
	if !ok {
		return
	}
	if _, err := h.Service.Repo.Publication(r.Context(), h.Service.Repo.Pool, ws, id, false); err != nil {
		fail(w, r, err)
		return
	}
	v, err := h.Service.Repo.Attempts(r.Context(), ws, id, l, o)
	reply(w, r, v, err, 200)
}
func (h *Handler) calls(w http.ResponseWriter, r *http.Request, ws uuid.UUID, _ app.Caller) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	a, err := uuid.Parse(r.PathValue("attemptId"))
	if err != nil || a == uuid.Nil {
		fail(w, r, d.ErrInvalid)
		return
	}
	l, o, ok := pagination(w, r)
	if !ok {
		return
	}
	var exists bool
	if err = h.Service.Repo.Pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM external_publication_attempt WHERE id=$1 AND workspace_id=$2 AND publication_id=$3)`, a, ws, id).Scan(&exists); err != nil {
		fail(w, r, err)
		return
	}
	if !exists {
		fail(w, r, d.ErrNotFound)
		return
	}
	v, err := h.Service.Repo.Calls(r.Context(), h.Service.Repo.Pool, ws, id, a, l, o)
	reply(w, r, v, err, 200)
}
