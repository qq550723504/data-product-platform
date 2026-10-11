package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	app "github.com/qq550723504/data-product-platform/apps/platform/internal/ingestion/application"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/ingestion/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
)

type Handler struct {
	Service *app.Service
	// Resolver is fetched for every check so revocation/config reload is fresh.
	Resolver func() (principal.Resolver, error)
	// Bind constructs the frozen source/template contract from server config.
	Bind func(context.Context, uuid.UUID, string, string, []Target) (d.Request, error)
}
type Target struct {
	Code       string    `json:"code"`
	DatasetID  uuid.UUID `json:"datasetId"`
	ResourceID uuid.UUID `json:"resourceId"`
}
type prepareBody struct {
	WorkspaceID uuid.UUID `json:"workspaceId"`
	RequestKey  string    `json:"requestKey"`
	BatchKey    string    `json:"batchKey"`
	Targets     []Target  `json:"targets"`
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/ingestions", h.prepare)
	mux.HandleFunc("POST /api/v1/ingestions/{id}/{command}", h.command)
	mux.HandleFunc("GET /api/v1/ingestions/{id}", h.get)
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return d.ErrManifest
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return d.ErrManifest
	}
	return nil
}
func reply(w http.ResponseWriter, v any, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	status := http.StatusOK
	code := ""
	if err != nil {
		switch {
		case errors.Is(err, d.ErrManifest):
			status = 400
			code = "INVALID_MANIFEST"
		case errors.Is(err, d.ErrConflict), errors.Is(err, d.ErrState):
			status = 409
			code = "CONFLICT"
		case errors.Is(err, d.ErrDenied):
			status = 403
			code = "AUTHORITY_DENIED"
		case errors.Is(err, d.ErrUnknown):
			status = 503
			code = "OUTCOME_UNKNOWN"
		case errors.Is(err, pgx.ErrNoRows):
			status = 404
			code = "NOT_FOUND"
		default:
			status = 503
			code = "RETRY_REQUIRED"
		}
		v = map[string]string{"code": code}
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func (h *Handler) caller(r *http.Request) app.Caller {
	return func(ctx context.Context, ws uuid.UUID) (principal.RequestPrincipal, error) {
		resolver, err := h.Resolver()
		if err != nil || resolver == nil {
			return principal.RequestPrincipal{}, d.ErrDenied
		}
		p, err := resolver.Resolve(r.WithContext(ctx), ws, "INGEST_RAW")
		if err != nil {
			return p, d.ErrDenied
		}
		return p, nil
	}
}
func (h *Handler) prepare(w http.ResponseWriter, r *http.Request) {
	var body prepareBody
	if err := decode(w, r, &body); err != nil {
		reply(w, nil, err)
		return
	}
	p, err := h.caller(r)(r.Context(), body.WorkspaceID)
	if err != nil {
		reply(w, nil, err)
		return
	}
	req, err := h.Bind(r.Context(), body.WorkspaceID, body.RequestKey, body.BatchKey, body.Targets)
	if err != nil {
		reply(w, nil, d.ErrDenied)
		return
	}
	req.ActorID = p.ActorID
	req.PrincipalRef = p.Subject
	op, err := h.Service.Prepare(r.Context(), req, h.caller(r))
	reply(w, op, err)
}
func (h *Handler) operation(r *http.Request) (d.Operation, error) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		return d.Operation{}, d.ErrManifest
	}
	op, err := h.Service.Repo.Load(r.Context(), h.Service.Repo.Pool, id, false)
	if err != nil {
		return op, err
	}
	p, err := h.caller(r)(r.Context(), op.Request.WorkspaceID)
	if err != nil || p.ActorID != op.Request.ActorID || p.Subject != op.Request.PrincipalRef {
		return d.Operation{}, d.ErrDenied
	}
	return op, nil
}
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	op, err := h.operation(r)
	reply(w, op, err)
}
func (h *Handler) command(w http.ResponseWriter, r *http.Request) {
	op, err := h.operation(r)
	if err != nil {
		reply(w, nil, err)
		return
	}
	var result any
	switch r.PathValue("command") {
	case "admit":
		var m d.Manifest
		err = decode(w, r, &m)
		if err == nil {
			result, err = h.Service.Admit(r.Context(), op.ID, m, h.caller(r))
		}
	case "dispatch", "recover", "retry-failed":
		// Commands carry no secret/config/engine parameters or generic status PATCH.
		r.Body = http.MaxBytesReader(w, r.Body, 1)
		b, e := io.ReadAll(r.Body)
		if e != nil || len(b) != 0 {
			reply(w, nil, d.ErrManifest)
			return
		}
		switch r.PathValue("command") {
		case "dispatch":
			result, err = h.Service.Dispatch(r.Context(), op.ID, h.caller(r))
		case "recover":
			result, err = h.Service.Recover(r.Context(), op.ID, h.caller(r))
		case "retry-failed":
			result, err = h.Service.RetryFailed(r.Context(), op.ID, h.caller(r))
		}
	default:
		err = pgx.ErrNoRows
	}
	reply(w, result, err)
}
