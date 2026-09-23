package labelstudio

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	annotationapp "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/application"
)

const Provider = "LABEL_STUDIO"

type Client struct {
	baseURL     string
	token       string
	instanceRef string
	httpClient  *http.Client
}

func NewClient(baseURL, token, instanceRef string, httpClient *http.Client) (*Client, error) {
	baseURL, err := normalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	token = strings.TrimSpace(token)
	instanceRef = strings.TrimSpace(instanceRef)
	if token == "" || instanceRef == "" {
		return nil, fmt.Errorf("Label Studio token and provider instance are required")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{baseURL: baseURL, token: token, instanceRef: instanceRef, httpClient: httpClient}, nil
}

func (c *Client) Provider() string { return Provider }

func (c *Client) EnsureCampaignBinding(ctx context.Context, req annotationapp.EngineCampaignRequest) (annotationapp.EngineCampaignBinding, error) {
	if req.WorkspaceID == uuid.Nil || req.CampaignID == uuid.Nil ||
		strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.RequestFingerprint) == "" ||
		strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.LabelConfig) == "" ||
		strings.TrimSpace(req.ConfigSHA256) == "" {
		return annotationapp.EngineCampaignBinding{}, annotationapp.NewAnnotationEngineError(
			annotationapp.ErrAnnotationEngineInvalidRequest, "ensure campaign binding", false, 0, nil,
		)
	}

	body, err := json.Marshal(map[string]any{
		"title":        req.Title,
		"label_config": req.LabelConfig,
		"description":  correlationDescription(req.CampaignID, req.RequestID, req.RequestFingerprint, req.ConfigSHA256),
	})
	if err != nil {
		return annotationapp.EngineCampaignBinding{}, annotationapp.NewAnnotationEngineError(
			annotationapp.ErrAnnotationEngineInvalidRequest, "encode project request", false, 0, err,
		)
	}

	var project struct {
		ID          json.Number `json:"id"`
		LabelConfig string      `json:"label_config"`
	}
	if err := c.requestJSON(ctx, http.MethodPost, "/api/projects", nil, body, &project); err != nil {
		return annotationapp.EngineCampaignBinding{}, err
	}
	if strings.TrimSpace(project.ID.String()) == "" {
		return annotationapp.EngineCampaignBinding{}, annotationapp.NewAnnotationEngineError(
			annotationapp.ErrAnnotationEngineInvalidResponse, "create project", false, 0, nil,
		)
	}
	if strings.TrimSpace(project.LabelConfig) != "" &&
		strings.TrimSpace(project.LabelConfig) != strings.TrimSpace(req.LabelConfig) {
		return annotationapp.EngineCampaignBinding{}, annotationapp.NewAnnotationEngineError(
			annotationapp.ErrAnnotationEngineInvalidResponse, "verify project config", false, 0, nil,
		)
	}
	return annotationapp.EngineCampaignBinding{
		Provider:          Provider,
		ProviderInstance:  c.instanceRef,
		ExternalProjectID: project.ID.String(),
		RequestID:         req.RequestID,
		ConfigSHA256:      req.ConfigSHA256,
	}, nil
}

func (c *Client) SubmitTasks(ctx context.Context, req annotationapp.EngineSubmitRequest) (annotationapp.EngineSubmission, error) {
	if err := validateBinding(c.instanceRef, req.Binding); err != nil {
		return annotationapp.EngineSubmission{}, err
	}
	if req.WorkspaceID == uuid.Nil || req.CampaignID == uuid.Nil ||
		strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.RequestFingerprint) == "" ||
		len(req.Tasks) == 0 {
		return annotationapp.EngineSubmission{}, annotationapp.NewAnnotationEngineError(
			annotationapp.ErrAnnotationEngineInvalidRequest, "submit tasks", false, 0, nil,
		)
	}

	payload := make([]map[string]any, 0, len(req.Tasks))
	for _, task := range req.Tasks {
		if task.TaskID == uuid.Nil || strings.TrimSpace(task.CorrelationKey) == "" ||
			strings.TrimSpace(task.SourceSHA256) == "" || strings.TrimSpace(task.TaskTextSHA256) == "" {
			return annotationapp.EngineSubmission{}, annotationapp.NewAnnotationEngineError(
				annotationapp.ErrAnnotationEngineInvalidRequest, "submit tasks", false, 0, nil,
			)
		}
		payload = append(payload, map[string]any{
			"data": map[string]any{"text": task.TaskText},
			"meta": map[string]any{
				"core_task_id":            task.TaskID.String(),
				"core_source_item_ref":    task.SourceItemRef,
				"core_source_sha256":      task.SourceSHA256,
				"core_task_text_sha256":   task.TaskTextSHA256,
				"core_correlation_key":    task.CorrelationKey,
				"core_request_id":         req.RequestID,
				"core_request_fingerprint": req.RequestFingerprint,
			},
		})
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return annotationapp.EngineSubmission{}, annotationapp.NewAnnotationEngineError(
			annotationapp.ErrAnnotationEngineInvalidRequest, "encode tasks", false, 0, err,
		)
	}
	query := url.Values{"return_task_ids": []string{"true"}}
	path := "/api/projects/" + url.PathEscape(req.Binding.ExternalProjectID) + "/import"

	var response struct {
		TaskCount int           `json:"task_count"`
		TaskIDs   []json.Number `json:"task_ids"`
	}
	if err := c.requestJSON(ctx, http.MethodPost, path, query, body, &response); err != nil {
		return annotationapp.EngineSubmission{}, err
	}
	if len(response.TaskIDs) != len(req.Tasks) || (response.TaskCount != 0 && response.TaskCount != len(req.Tasks)) {
		return annotationapp.EngineSubmission{}, annotationapp.NewAnnotationEngineError(
			annotationapp.ErrAnnotationEngineInvalidResponse, "verify task import", false, 0, nil,
		)
	}
	external := make(map[uuid.UUID]string, len(req.Tasks))
	for i, externalID := range response.TaskIDs {
		if strings.TrimSpace(externalID.String()) == "" {
			return annotationapp.EngineSubmission{}, annotationapp.NewAnnotationEngineError(
				annotationapp.ErrAnnotationEngineInvalidResponse, "verify task ids", false, 0, nil,
			)
		}
		external[req.Tasks[i].TaskID] = externalID.String()
	}
	return annotationapp.EngineSubmission{
		State:           annotationapp.EngineLookupMatched,
		RequestID:       req.RequestID,
		ExternalTaskIDs: external,
	}, nil
}

func (c *Client) LookupSubmission(ctx context.Context, req annotationapp.EngineLookupRequest) (annotationapp.EngineSubmission, error) {
	if err := validateBinding(c.instanceRef, req.Binding); err != nil {
		return annotationapp.EngineSubmission{}, err
	}
	if strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.RequestFingerprint) == "" || len(req.Tasks) == 0 {
		return annotationapp.EngineSubmission{}, annotationapp.NewAnnotationEngineError(
			annotationapp.ErrAnnotationEngineInvalidRequest, "lookup submission", false, 0, nil,
		)
	}

	expected := make(map[string]annotationapp.EngineTask, len(req.Tasks))
	for _, task := range req.Tasks {
		expected[task.TaskID.String()] = task
	}
	matched := make(map[uuid.UUID]string, len(req.Tasks))
	pageNumber := 1
	for {
		query := url.Values{
			"project":   []string{req.Binding.ExternalProjectID},
			"page":      []string{strconv.Itoa(pageNumber)},
			"page_size": []string{"100"},
		}
		var page struct {
			Tasks []struct {
				ID   json.Number    `json:"id"`
				Meta map[string]any `json:"meta"`
			} `json:"tasks"`
			Next any `json:"next"`
		}
		if err := c.requestJSON(ctx, http.MethodGet, "/api/tasks", query, nil, &page); err != nil {
			return annotationapp.EngineSubmission{}, err
		}
		for _, remote := range page.Tasks {
			coreTaskID, _ := remote.Meta["core_task_id"].(string)
			requestID, _ := remote.Meta["core_request_id"].(string)
			fingerprint, _ := remote.Meta["core_request_fingerprint"].(string)
			if requestID != req.RequestID || fingerprint != req.RequestFingerprint {
				continue
			}
			task, ok := expected[coreTaskID]
			if !ok {
				return annotationapp.EngineSubmission{
					State:         annotationapp.EngineLookupConflict,
					RequestID:     req.RequestID,
					DiagnosticRef: "unexpected correlated task",
				}, nil
			}
			if previous, exists := matched[task.TaskID]; exists && previous != remote.ID.String() {
				return annotationapp.EngineSubmission{
					State:         annotationapp.EngineLookupConflict,
					RequestID:     req.RequestID,
					DiagnosticRef: "duplicate correlated task",
				}, nil
			}
			matched[task.TaskID] = remote.ID.String()
		}
		if page.Next == nil || strings.TrimSpace(fmt.Sprint(page.Next)) == "" {
			break
		}
		pageNumber++
		if pageNumber > 1000 {
			return annotationapp.EngineSubmission{}, annotationapp.NewAnnotationEngineError(
				annotationapp.ErrAnnotationEngineInvalidResponse, "lookup submission pagination", false, 0, nil,
			)
		}
	}
	if len(matched) == len(req.Tasks) {
		return annotationapp.EngineSubmission{
			State:           annotationapp.EngineLookupMatched,
			RequestID:       req.RequestID,
			ExternalTaskIDs: matched,
		}, nil
	}
	return annotationapp.EngineSubmission{
		State:           annotationapp.EngineLookupUnknown,
		RequestID:       req.RequestID,
		ExternalTaskIDs: matched,
		DiagnosticRef:   "submission absence not proven",
	}, nil
}

func (c *Client) FetchResults(ctx context.Context, binding annotationapp.EngineCampaignBinding, cursor annotationapp.EngineResultCursor) (annotationapp.EngineResultPage, error) {
	if err := validateBinding(c.instanceRef, binding); err != nil {
		return annotationapp.EngineResultPage{}, err
	}
	if cursor.Offset < 0 {
		return annotationapp.EngineResultPage{}, annotationapp.NewAnnotationEngineError(
			annotationapp.ErrAnnotationEngineInvalidRequest, "fetch results", false, 0, nil,
		)
	}
	query := url.Values{
		"project":   []string{binding.ExternalProjectID},
		"page":      []string{strconv.Itoa(cursor.Offset/100 + 1)},
		"page_size": []string{"100"},
	}
	var response struct {
		Tasks []struct {
			ID          json.Number    `json:"id"`
			Meta        map[string]any `json:"meta"`
			Annotations []struct {
				ID           json.Number `json:"id"`
				CompletedBy  any         `json:"completed_by"`
				WasCancelled bool        `json:"was_cancelled"`
				Result       []any       `json:"result"`
				UpdatedAt    string      `json:"updated_at"`
			} `json:"annotations"`
		} `json:"tasks"`
		Next any `json:"next"`
	}
	if err := c.requestJSON(ctx, http.MethodGet, "/api/tasks", query, nil, &response); err != nil {
		return annotationapp.EngineResultPage{}, err
	}

	results := make([]annotationapp.EngineResultObservation, 0)
	for _, task := range response.Tasks {
		coreTaskText, _ := task.Meta["core_task_id"].(string)
		coreTaskID, err := uuid.Parse(strings.TrimSpace(coreTaskText))
		if err != nil {
			continue
		}
		for _, annotation := range task.Annotations {
			if annotation.WasCancelled {
				continue
			}
			label, ok := singleChoiceLabel(annotation.Result)
			if !ok {
				continue
			}
			payload, _ := json.Marshal(map[string]string{"label": label})
			sum := sha256.Sum256(payload)
			results = append(results, annotationapp.EngineResultObservation{
				TaskID:                coreTaskID,
				ExternalTaskID:        task.ID.String(),
				ExternalAnnotationID:  annotation.ID.String(),
				ExternalRevision:      strings.TrimSpace(annotation.UpdatedAt),
				AuthorRef:             fmt.Sprint(annotation.CompletedBy),
				CanonicalPayload:      payload,
				CanonicalPayloadSHA256: hex.EncodeToString(sum[:]),
				NormalizerVersion:     "labelstudio-single-label-v1",
				ProviderSubmitted:     true,
			})
		}
	}
	var next *annotationapp.EngineResultCursor
	if response.Next != nil && strings.TrimSpace(fmt.Sprint(response.Next)) != "" {
		next = &annotationapp.EngineResultCursor{Offset: cursor.Offset + 100}
	}
	return annotationapp.EngineResultPage{Results: results, NextCursor: next}, nil
}

func singleChoiceLabel(result []any) (string, bool) {
	for _, raw := range result {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		value, ok := item["value"].(map[string]any)
		if !ok {
			continue
		}
		choices, ok := value["choices"].([]any)
		if !ok || len(choices) != 1 {
			continue
		}
		label, ok := choices[0].(string)
		label = strings.TrimSpace(label)
		if ok && label != "" {
			return label, true
		}
	}
	return "", false
}

func validateBinding(instanceRef string, binding annotationapp.EngineCampaignBinding) error {
	if binding.Provider != Provider ||
		strings.TrimSpace(binding.ProviderInstance) != strings.TrimSpace(instanceRef) ||
		strings.TrimSpace(binding.ExternalProjectID) == "" {
		return annotationapp.NewAnnotationEngineError(
			annotationapp.ErrAnnotationEngineInvalidRequest, "validate provider binding", false, 0, nil,
		)
	}
	return nil
}

func correlationDescription(campaignID uuid.UUID, requestID, fingerprint, configSHA string) string {
	return fmt.Sprintf(
		"core_campaign_id=%s core_request_id=%s core_request_fingerprint=%s core_config_sha256=%s",
		campaignID, strings.TrimSpace(requestID), strings.TrimSpace(fingerprint), strings.TrimSpace(configSHA),
	)
}

func normalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("Label Studio base URL is required")
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid Label Studio base URL %q", raw)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("Label Studio base URL must not contain query or fragment")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func (c *Client) requestJSON(ctx context.Context, method, path string, query url.Values, body []byte, target any) error {
	requestURL := c.baseURL + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reader)
	if err != nil {
		return annotationapp.NewAnnotationEngineError(
			annotationapp.ErrAnnotationEngineInvalidRequest, "create request", false, 0, err,
		)
	}
	req.Header.Set("Authorization", "Token "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return annotationapp.NewAnnotationEngineOutcomeError(
			annotationapp.ErrAnnotationEngineUnavailable,
			"provider request",
			true,
			method != http.MethodGet,
			0,
			err,
		)
	}
	defer resp.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if readErr != nil {
		return annotationapp.NewAnnotationEngineOutcomeError(
			annotationapp.ErrAnnotationEngineUnavailable,
			"read provider response",
			true,
			method != http.MethodGet,
			resp.StatusCode,
			readErr,
		)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		kind, retryable := classifyHTTPStatus(resp.StatusCode)
		return annotationapp.NewAnnotationEngineOutcomeError(
			kind,
			"provider request",
			retryable,
			retryable && method != http.MethodGet,
			resp.StatusCode,
			nil,
		)
	}
	if target == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return annotationapp.NewAnnotationEngineOutcomeError(
			annotationapp.ErrAnnotationEngineInvalidResponse,
			"decode provider response",
			false,
			method != http.MethodGet,
			resp.StatusCode,
			err,
		)
	}
	return nil
}

func classifyHTTPStatus(status int) (error, bool) {
	switch {
	case status == http.StatusBadRequest || status == http.StatusUnprocessableEntity:
		return annotationapp.ErrAnnotationEngineInvalidRequest, false
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return annotationapp.ErrAnnotationEngineUnauthorized, false
	case status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= http.StatusInternalServerError:
		return annotationapp.ErrAnnotationEngineUnavailable, true
	default:
		return annotationapp.ErrAnnotationEngineRejected, false
	}
}

var _ annotationapp.AnnotationEnginePort = (*Client)(nil)
