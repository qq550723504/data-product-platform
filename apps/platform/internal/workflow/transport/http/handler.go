package workflowhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
)

type Handler struct {
	versions   *workflowapp.WorkflowVersionService
	executions *workflowapp.ExecutionService
	repo       *workflowinfra.PostgresRepository
}

func NewHandler(versions *workflowapp.WorkflowVersionService, executions *workflowapp.ExecutionService, repo *workflowinfra.PostgresRepository) *Handler {
	return &Handler{versions: versions, executions: executions, repo: repo}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/workflow-versions", h.createWorkflowVersion)
	mux.HandleFunc("POST /api/v1/executions", h.createExecution)
	mux.HandleFunc("GET /api/v1/executions/{executionId}", h.getExecution)
	mux.HandleFunc("POST /api/v1/executions/{executionId}/retry", h.retryExecution)
}

type createWorkflowVersionRequest struct {
	WorkspaceID    string `json:"workspaceId"`
	Code           string `json:"code"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	Version        string `json:"version"`
	DefinitionRef  string `json:"definitionRef"`
	DefinitionYAML string `json:"definitionYaml"`
}

func (h *Handler) createWorkflowVersion(w http.ResponseWriter, r *http.Request) {
	var req createWorkflowVersionRequest
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
	version, err := h.versions.Create(r.Context(), workflowapp.CreateWorkflowVersionCommand{
		WorkspaceID:    workspaceID,
		Code:           req.Code,
		Name:           req.Name,
		Description:    req.Description,
		Version:        req.Version,
		DefinitionRef:  req.DefinitionRef,
		DefinitionYAML: []byte(req.DefinitionYAML),
		ActorID:        actorID,
		TraceID:        httpserver.RequestID(r.Context()),
	})
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "WORKFLOW_VERSION_CREATE_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":               version.ID,
		"workflowId":       version.WorkflowID,
		"version":          version.Version,
		"definitionRef":    version.DefinitionRef,
		"definitionSha256": version.DefinitionSHA256,
	})
}

type executionInputRequest struct {
	Name             string `json:"name"`
	DatasetVersionID string `json:"datasetVersionId"`
}

type createExecutionRequest struct {
	WorkspaceID       string                  `json:"workspaceId"`
	WorkflowVersionID string                  `json:"workflowVersionId"`
	OutputDatasetID   string                  `json:"outputDatasetId"`
	TargetPeriod      string                  `json:"targetPeriod"`
	Inputs            []executionInputRequest `json:"inputs"`
}

func (h *Handler) createExecution(w http.ResponseWriter, r *http.Request) {
	var req createExecutionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_JSON", "invalid JSON request", nil)
		return
	}
	workspaceID, err := uuid.Parse(req.WorkspaceID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_WORKSPACE_ID", "workspaceId must be a UUID", nil)
		return
	}
	workflowVersionID, err := uuid.Parse(req.WorkflowVersionID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_WORKFLOW_VERSION_ID", "workflowVersionId must be a UUID", nil)
		return
	}
	outputDatasetID, err := uuid.Parse(req.OutputDatasetID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_OUTPUT_DATASET_ID", "outputDatasetId must be a UUID", nil)
		return
	}
	inputs := make([]domain.InputBinding, 0, len(req.Inputs))
	for _, input := range req.Inputs {
		versionID, err := uuid.Parse(input.DatasetVersionID)
		if err != nil {
			httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_INPUT_DATASET_VERSION_ID", "every input datasetVersionId must be a UUID", map[string]any{"inputName": input.Name})
			return
		}
		inputs = append(inputs, domain.InputBinding{Name: strings.TrimSpace(input.Name), DatasetVersionID: versionID})
	}
	actorID, err := parseActorID(r)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ACTOR_ID", "X-Actor-ID must be a UUID", nil)
		return
	}
	execution, err := h.executions.Create(r.Context(), workflowapp.CreateExecutionCommand{
		WorkspaceID:       workspaceID,
		WorkflowVersionID: workflowVersionID,
		OutputDatasetID:   outputDatasetID,
		TargetPeriod:      req.TargetPeriod,
		Inputs:            inputs,
		ActorID:           actorID,
		TraceID:           httpserver.RequestID(r.Context()),
	})
	if err != nil {
		if errors.Is(err, domain.ErrWorkspaceMismatch) {
			httpserver.WriteError(w, r, http.StatusBadRequest, "EXECUTION_WORKSPACE_MISMATCH", "workflow, inputs, output and execution must belong to one workspace", nil)
			return
		}
		if errors.Is(err, domain.ErrExecutionReferenceUnusable) {
			httpserver.WriteError(w, r, http.StatusBadRequest, "EXECUTION_REFERENCE_UNUSABLE", "an execution input must be READY or SUPERSEDED", nil)
			return
		}
		status := http.StatusBadRequest
		if errors.Is(err, workflowinfra.ErrNotFound) {
			status = http.StatusNotFound
		}
		httpserver.WriteError(w, r, status, "EXECUTION_CREATE_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusAccepted, executionResponse(execution))
}

func (h *Handler) getExecution(w http.ResponseWriter, r *http.Request) {
	executionID, err := uuid.Parse(r.PathValue("executionId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_EXECUTION_ID", "executionId must be a UUID", nil)
		return
	}
	execution, err := h.repo.GetExecution(r.Context(), executionID)
	if err != nil {
		if errors.Is(err, workflowinfra.ErrNotFound) {
			httpserver.WriteError(w, r, http.StatusNotFound, "EXECUTION_NOT_FOUND", "execution not found", nil)
			return
		}
		httpserver.WriteError(w, r, http.StatusInternalServerError, "EXECUTION_READ_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, executionResponse(execution))
}

func (h *Handler) retryExecution(w http.ResponseWriter, r *http.Request) {
	executionID, err := uuid.Parse(r.PathValue("executionId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_EXECUTION_ID", "executionId must be a UUID", nil)
		return
	}
	actorID, err := parseActorID(r)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ACTOR_ID", "X-Actor-ID must be a UUID", nil)
		return
	}
	execution, err := h.executions.Retry(r.Context(), executionID, actorID, httpserver.RequestID(r.Context()))
	if err != nil {
		if errors.Is(err, domain.ErrWorkspaceMismatch) {
			httpserver.WriteError(w, r, http.StatusBadRequest, "EXECUTION_WORKSPACE_MISMATCH", "workflow, inputs, output and execution must belong to one workspace", nil)
			return
		}
		if errors.Is(err, domain.ErrExecutionReferenceUnusable) {
			httpserver.WriteError(w, r, http.StatusBadRequest, "EXECUTION_REFERENCE_UNUSABLE", "an execution input must be READY or SUPERSEDED", nil)
			return
		}
		status := http.StatusConflict
		if errors.Is(err, workflowinfra.ErrNotFound) {
			status = http.StatusNotFound
		}
		httpserver.WriteError(w, r, status, "EXECUTION_RETRY_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusAccepted, executionResponse(execution))
}

func executionResponse(execution domain.Execution) map[string]any {
	inputs := make([]map[string]any, 0, len(execution.Inputs))
	for _, input := range execution.Inputs {
		inputs = append(inputs, map[string]any{
			"name":             input.Name,
			"datasetVersionId": input.DatasetVersionID,
		})
	}
	return map[string]any{
		"id":                     execution.ID,
		"workspaceId":            execution.WorkspaceID,
		"workflowVersionId":      execution.WorkflowVersionID,
		"outputDatasetId":        execution.OutputDatasetID,
		"outputDatasetVersionId": execution.OutputDatasetVersionID,
		"targetPeriod":           execution.TargetPeriod,
		"status":                 execution.Status,
		"attempt":                execution.Attempt,
		"retryOfExecutionId":     execution.RetryOfExecutionID,
		"engineType":             execution.EngineType,
		"engineExecutionId":      execution.EngineExecutionID,
		"errorCode":              execution.ErrorCode,
		"errorMessage":           execution.ErrorMessage,
		"metrics":                execution.Metrics,
		"inputs":                 inputs,
		"createdAt":              execution.CreatedAt,
		"startedAt":              execution.StartedAt,
		"finishedAt":             execution.FinishedAt,
	}
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

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
