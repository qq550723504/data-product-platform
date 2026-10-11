package http

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	app "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/application"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"io"
	"net/http"
	"strconv"
	"time"
)

type Handler struct {
	Service  *app.Service
	Resolver principal.Resolver
}

func New(service *app.Service, resolver principal.Resolver) *Handler {
	return &Handler{Service: service, Resolver: resolver}
}

const base = "/api/v1/workspaces/{workspaceId}/external-integration"

type endpoint func(http.ResponseWriter, *http.Request, uuid.UUID, app.Caller)

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST "+base+"/identity-bindings", h.wrap(h.bind))
	mux.HandleFunc("GET "+base+"/identity-bindings", h.wrap(h.bindings))
	mux.HandleFunc("POST "+base+"/identity-bindings/{id}/invalidate", h.wrap(h.invalidateIdentity))
	mux.HandleFunc("POST "+base+"/identifiers", h.wrap(h.record))
	mux.HandleFunc("GET "+base+"/identifiers", h.wrap(h.identifiers))
	mux.HandleFunc("POST "+base+"/identifiers/{id}/invalidate", h.wrap(h.invalidateIdentifier))
	mux.HandleFunc("POST "+base+"/publications", h.wrap(h.create))
	mux.HandleFunc("GET "+base+"/publications", h.wrap(h.publications))
	mux.HandleFunc("GET "+base+"/publications/{id}", h.wrap(h.publication))
	mux.HandleFunc("POST "+base+"/publications/{id}/request", h.wrap(h.request))
}
func (h *Handler) wrap(fn endpoint) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		ws, err := uuid.Parse(r.PathValue("workspaceId"))
		if err != nil || ws == uuid.Nil {
			fail(w, r, d.ErrInvalid)
			return
		}
		caller := func(ctx context.Context) (principal.RequestPrincipal, error) {
			if h.Resolver == nil {
				return principal.RequestPrincipal{}, principal.ErrNotConfigured
			}
			return h.Resolver.Resolve(r.WithContext(ctx), ws, d.Capability)
		}
		if _, err := app.Authorize(ctx, ws, caller); err != nil {
			fail(w, r, err)
			return
		}
		fn(w, r, ws, caller)
	}
}
func decode(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		fail(w, r, d.ErrInvalid)
		return false
	}
	var tail any
	if err := dec.Decode(&tail); err != io.EOF {
		fail(w, r, d.ErrInvalid)
		return false
	}
	return true
}
func reply(w http.ResponseWriter, r *http.Request, v any, err error, status int) {
	if err != nil {
		fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, r *http.Request, err error) {
	status, code, msg := http.StatusServiceUnavailable, "EXTERNAL_INTEGRATION_UNAVAILABLE", "external integration unavailable"
	switch {
	case errors.Is(err, principal.ErrUnauthenticated):
		status, code, msg = 401, "AUTHENTICATION_REQUIRED", "authenticated principal required"
	case errors.Is(err, principal.ErrWorkspaceDenied), errors.Is(err, principal.ErrCapabilityDenied), errors.Is(err, d.ErrDenied):
		status, code, msg = 403, "EXTERNAL_INTEGRATION_DENIED", "external integration access denied"
	case errors.Is(err, d.ErrNotFound):
		status, code, msg = 404, "EXTERNAL_OBJECT_NOT_FOUND", "external integration object not found"
	case errors.Is(err, d.ErrConflict), errors.Is(err, d.ErrState):
		status, code, msg = 409, "EXTERNAL_INTEGRATION_CONFLICT", "external integration command conflicts with current facts"
	case errors.Is(err, d.ErrInvalid):
		status, code, msg = 400, "INVALID_EXTERNAL_REQUEST", "invalid external integration request"
	}
	httpserver.WriteError(w, r, status, code, msg, nil)
}
func key(r *http.Request) string { return r.Header.Get("Idempotency-Key") }
func pathID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil || id == uuid.Nil {
		fail(w, r, d.ErrInvalid)
		return uuid.Nil, false
	}
	return id, true
}
func (h *Handler) bind(w http.ResponseWriter, r *http.Request, ws uuid.UUID, c app.Caller) {
	var s d.BindingSpec
	if !decode(w, r, &s) {
		return
	}
	rec, err := h.Service.BindIdentity(r.Context(), ws, c, key(r), s)
	reply(w, r, rec, err, 201)
}
func (h *Handler) record(w http.ResponseWriter, r *http.Request, ws uuid.UUID, c app.Caller) {
	var s d.IdentifierSpec
	if !decode(w, r, &s) {
		return
	}
	rec, err := h.Service.RecordIdentifier(r.Context(), ws, c, key(r), s)
	reply(w, r, rec, err, 201)
}
func (h *Handler) create(w http.ResponseWriter, r *http.Request, ws uuid.UUID, c app.Caller) {
	var s d.PublicationSpec
	if !decode(w, r, &s) {
		return
	}
	rec, err := h.Service.CreatePublication(r.Context(), ws, c, key(r), s)
	reply(w, r, rec, err, 201)
}
func (h *Handler) invalidateIdentity(w http.ResponseWriter, r *http.Request, ws uuid.UUID, c app.Caller) {
	h.invalidate(w, r, ws, c, true)
}
func (h *Handler) invalidateIdentifier(w http.ResponseWriter, r *http.Request, ws uuid.UUID, c app.Caller) {
	h.invalidate(w, r, ws, c, false)
}
func (h *Handler) invalidate(w http.ResponseWriter, r *http.Request, ws uuid.UUID, c app.Caller, identity bool) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if !decode(w, r, &body) {
		return
	}
	spec := app.InvalidateSpec{ID: id, Reason: body.Reason}
	var rec d.Receipt
	var err error
	if identity {
		rec, err = h.Service.InvalidateIdentity(r.Context(), ws, c, key(r), spec)
	} else {
		rec, err = h.Service.InvalidateIdentifier(r.Context(), ws, c, key(r), spec)
	}
	reply(w, r, rec, err, 200)
}
func (h *Handler) request(w http.ResponseWriter, r *http.Request, ws uuid.UUID, c app.Caller) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct{}
	if !decode(w, r, &body) {
		return
	}
	rec, err := h.Service.RequestPublication(r.Context(), ws, c, key(r), id)
	reply(w, r, rec, err, 200)
}
func pagination(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	limit, offset := 50, 0
	var err error
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil {
			fail(w, r, d.ErrInvalid)
			return 0, 0, false
		}
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		offset, err = strconv.Atoi(raw)
	}
	if err != nil || limit < 1 || limit > 100 || offset < 0 || offset > 10000 {
		fail(w, r, d.ErrInvalid)
		return 0, 0, false
	}
	return limit, offset, true
}
func (h *Handler) bindings(w http.ResponseWriter, r *http.Request, ws uuid.UUID, _ app.Caller) {
	l, o, ok := pagination(w, r)
	if !ok {
		return
	}
	v, err := h.Service.Repo.Bindings(r.Context(), ws, l, o)
	reply(w, r, v, err, 200)
}
func (h *Handler) identifiers(w http.ResponseWriter, r *http.Request, ws uuid.UUID, _ app.Caller) {
	l, o, ok := pagination(w, r)
	if !ok {
		return
	}
	v, err := h.Service.Repo.Identifiers(r.Context(), ws, l, o)
	reply(w, r, v, err, 200)
}
func (h *Handler) publications(w http.ResponseWriter, r *http.Request, ws uuid.UUID, _ app.Caller) {
	l, o, ok := pagination(w, r)
	if !ok {
		return
	}
	v, err := h.Service.Repo.Publications(r.Context(), ws, l, o)
	reply(w, r, v, err, 200)
}
func (h *Handler) publication(w http.ResponseWriter, r *http.Request, ws uuid.UUID, _ app.Caller) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	v, err := h.Service.Repo.PublicationHistory(r.Context(), ws, id)
	reply(w, r, v, err, 200)
}
