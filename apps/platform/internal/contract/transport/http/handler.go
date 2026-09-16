package contracthttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/contract/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/contract/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
)

type Handler struct {
	service *application.Service
	repo    *infrastructure.PostgresRepository
}

func NewHandler(service *application.Service, repo *infrastructure.PostgresRepository) *Handler {
	return &Handler{service: service, repo: repo}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/data-contracts/versions", h.createVersion)
	mux.HandleFunc("GET /api/v1/contract-versions/{versionId}", h.getVersion)
	mux.HandleFunc("POST /api/v1/contract-versions/{versionId}/publish", h.publishVersion)
}

type createVersionRequest struct {
	WorkspaceID  string `json:"workspaceId"`
	SourceRef    string `json:"sourceRef"`
	DocumentYAML string `json:"documentYaml"`
}

func (h *Handler) createVersion(w http.ResponseWriter, r *http.Request) {
	var req createVersionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_JSON", "invalid JSON request", nil)
		return
	}
	workspaceID, err := uuid.Parse(req.WorkspaceID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_WORKSPACE_ID", "workspaceId must be a UUID", nil)
		return
	}
	actorID, err := parseActorID(r)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ACTOR_ID", "X-Actor-ID must be a UUID", nil)
		return
	}
	version, err := h.service.CreateVersionFromYAML(r.Context(), application.CreateVersionFromYAMLCommand{
		WorkspaceID:  workspaceID,
		SourceRef:    req.SourceRef,
		DocumentYAML: []byte(req.DocumentYAML),
		ActorID:      actorID,
		TraceID:      httpserver.RequestID(r.Context()),
	})
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "CONTRACT_VERSION_CREATE_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, versionResponse(version))
}

func (h *Handler) getVersion(w http.ResponseWriter, r *http.Request) {
	versionID, err := uuid.Parse(r.PathValue("versionId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_CONTRACT_VERSION_ID", "versionId must be a UUID", nil)
		return
	}
	version, err := h.repo.GetVersion(r.Context(), versionID)
	if err != nil {
		if errors.Is(err, infrastructure.ErrNotFound) {
			httpserver.WriteError(w, r, http.StatusNotFound, "CONTRACT_VERSION_NOT_FOUND", "contract version not found", nil)
			return
		}
		httpserver.WriteError(w, r, http.StatusInternalServerError, "CONTRACT_VERSION_READ_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, versionResponse(version))
}

func (h *Handler) publishVersion(w http.ResponseWriter, r *http.Request) {
	versionID, err := uuid.Parse(r.PathValue("versionId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_CONTRACT_VERSION_ID", "versionId must be a UUID", nil)
		return
	}
	actorID, err := parseActorID(r)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ACTOR_ID", "X-Actor-ID must be a UUID", nil)
		return
	}
	version, err := h.service.PublishVersion(r.Context(), application.PublishVersionCommand{
		VersionID: versionID,
		ActorID:   actorID,
		TraceID:   httpserver.RequestID(r.Context()),
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, infrastructure.ErrNotFound) {
			status = http.StatusNotFound
		}
		httpserver.WriteError(w, r, status, "CONTRACT_VERSION_PUBLISH_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, versionResponse(version))
}

func versionResponse(version interface {
	Semver() string
}) map[string]any {
	return map[string]any{"version": version.Semver()}
}

func parseActorID(r *http.Request) (*uuid.UUID, error) {
	value := strings.TrimSpace(r.Header.Get("X-Actor-ID"))
	if value == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
