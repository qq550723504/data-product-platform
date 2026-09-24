package labelstudio

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
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
	if strings.Count(token, ".") == 2 {
		return nil, fmt.Errorf("Label Studio JWT refresh tokens are not supported by the reference adapter; configure a direct API token")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{baseURL: baseURL, token: token, instanceRef: instanceRef, httpClient: httpClient}, nil
}

func (c *Client) Provider() string    { return Provider }
func (c *Client) InstanceRef() string { return c.instanceRef }

func (c *Client) EnsureCampaignBinding(ctx context.Context, req annotationapp.EngineCampaignRequest) (annotationapp.EngineCampaignBinding, error) {
	if req.WorkspaceID == uuid.Nil || req.CampaignID == uuid.Nil ||
		strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.RequestFingerprint) == "" ||
		strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.SchemaContent) == "" ||
		strings.TrimSpace(req.SchemaSHA256) == "" {
		return annotationapp.EngineCampaignBinding{}, annotationapp.NewAnnotationEngineError(
			annotationapp.ErrAnnotationEngineInvalidRequest, "ensure campaign binding", false, 0, nil,
		)
	}

	labelConfig, configSHA256, err := labelConfigFromSchema(req.SchemaContent)
	if err != nil {
		return annotationapp.EngineCampaignBinding{}, annotationapp.NewAnnotationEngineError(
			annotationapp.ErrAnnotationEngineInvalidRequest, "build label config", false, 0, err,
		)
	}
	body, err := json.Marshal(map[string]any{
		"title":        req.Title,
		"label_config": labelConfig,
		"description":  correlationDescription(req.CampaignID, req.RequestID, req.RequestFingerprint, req.SchemaSHA256),
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
		strings.TrimSpace(project.LabelConfig) != strings.TrimSpace(labelConfig) {
		return annotationapp.EngineCampaignBinding{}, annotationapp.NewAnnotationEngineError(
			annotationapp.ErrAnnotationEngineInvalidResponse, "verify project config", false, 0, nil,
		)
	}
	lookup, err := c.LookupCampaignBinding(ctx, req)
	if err != nil {
		return annotationapp.EngineCampaignBinding{}, err
	}
	if lookup.State != annotationapp.EngineLookupMatched || lookup.Binding == nil {
		return annotationapp.EngineCampaignBinding{}, annotationapp.NewAnnotationEngineError(
			annotationapp.ErrAnnotationEngineInvalidResponse,
			"verify created project",
			false,
			0,
			nil,
		)
	}
	if lookup.Binding.ExternalProjectID != project.ID.String() ||
		lookup.Binding.ConfigSHA256 != configSHA256 {
		return annotationapp.EngineCampaignBinding{}, annotationapp.NewAnnotationEngineError(
			annotationapp.ErrAnnotationEngineInvalidResponse,
			"verify created project identity",
			false,
			0,
			nil,
		)
	}
	return *lookup.Binding, nil
}

func (c *Client) LookupCampaignBinding(
	ctx context.Context,
	req annotationapp.EngineCampaignRequest,
) (annotationapp.EngineCampaignLookup, error) {
	if req.WorkspaceID == uuid.Nil || req.CampaignID == uuid.Nil ||
		strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.RequestFingerprint) == "" ||
		strings.TrimSpace(req.SchemaContent) == "" || strings.TrimSpace(req.SchemaSHA256) == "" {
		return annotationapp.EngineCampaignLookup{}, annotationapp.NewAnnotationEngineError(
			annotationapp.ErrAnnotationEngineInvalidRequest, "lookup campaign binding", false, 0, nil,
		)
	}

	labelConfig, configSHA256, err := labelConfigFromSchema(req.SchemaContent)
	if err != nil {
		return annotationapp.EngineCampaignLookup{}, annotationapp.NewAnnotationEngineError(
			annotationapp.ErrAnnotationEngineInvalidRequest, "build label config", false, 0, err,
		)
	}
	expectedDescription := correlationDescription(
		req.CampaignID,
		req.RequestID,
		req.RequestFingerprint,
		req.SchemaSHA256,
	)
	pageNumber := 1
	var matched []annotationapp.EngineCampaignBinding
	for {
		query := url.Values{
			"search":    []string{req.RequestID},
			"page":      []string{strconv.Itoa(pageNumber)},
			"page_size": []string{"100"},
		}
		var page struct {
			Count   int `json:"count"`
			Results []struct {
				ID          json.Number `json:"id"`
				Description string      `json:"description"`
				LabelConfig string      `json:"label_config"`
			} `json:"results"`
			Next any `json:"next"`
		}
		if err := c.requestJSON(ctx, http.MethodGet, "/api/projects/", query, nil, &page); err != nil {
			return annotationapp.EngineCampaignLookup{}, err
		}
		for _, project := range page.Results {
			if strings.TrimSpace(project.Description) != expectedDescription {
				continue
			}
			if strings.TrimSpace(project.LabelConfig) != strings.TrimSpace(labelConfig) {
				return annotationapp.EngineCampaignLookup{
					State:         annotationapp.EngineLookupConflict,
					DiagnosticRef: "correlated project config mismatch",
				}, nil
			}
			binding := annotationapp.EngineCampaignBinding{
				Provider:          Provider,
				ProviderInstance:  c.instanceRef,
				ExternalProjectID: project.ID.String(),
				RequestID:         req.RequestID,
				ConfigSHA256:      configSHA256,
			}
			matched = append(matched, binding)
		}
		if page.Count > 0 {
			if pageNumber*100 >= page.Count {
				break
			}
		} else if page.Next == nil || strings.TrimSpace(fmt.Sprint(page.Next)) == "" {
			break
		}
		pageNumber++
		if pageNumber > 1000 {
			return annotationapp.EngineCampaignLookup{}, annotationapp.NewAnnotationEngineError(
				annotationapp.ErrAnnotationEngineInvalidResponse,
				"lookup campaign binding pagination",
				false,
				0,
				nil,
			)
		}
	}
	switch len(matched) {
	case 0:
		return annotationapp.EngineCampaignLookup{
			State:         annotationapp.EngineLookupUnknown,
			DiagnosticRef: "project absence not proven",
		}, nil
	case 1:
		return annotationapp.EngineCampaignLookup{
			State:   annotationapp.EngineLookupMatched,
			Binding: &matched[0],
		}, nil
	default:
		return annotationapp.EngineCampaignLookup{
			State:         annotationapp.EngineLookupConflict,
			DiagnosticRef: "duplicate correlated projects",
		}, nil
	}
}

func (c *Client) VerifyCampaignBinding(
	ctx context.Context,
	binding annotationapp.EngineCampaignBinding,
) error {
	if err := validateBinding(c.instanceRef, binding); err != nil {
		return err
	}
	var project struct {
		ID          json.Number `json:"id"`
		LabelConfig string      `json:"label_config"`
	}
	path := "/api/projects/" + url.PathEscape(binding.ExternalProjectID)
	if err := c.requestJSON(ctx, http.MethodGet, path, nil, nil, &project); err != nil {
		return err
	}
	if project.ID.String() != binding.ExternalProjectID || strings.TrimSpace(project.LabelConfig) == "" {
		return annotationapp.NewAnnotationEngineError(
			annotationapp.ErrAnnotationEngineInvalidResponse,
			"verify campaign binding",
			false,
			0,
			nil,
		)
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(project.LabelConfig)))
	if hex.EncodeToString(sum[:]) != strings.TrimSpace(binding.ConfigSHA256) {
		return annotationapp.NewAnnotationEngineError(
			annotationapp.ErrAnnotationEngineInvalidResponse,
			"verify campaign binding config",
			false,
			0,
			nil,
		)
	}
	return nil
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
				"core_task_id":             task.TaskID.String(),
				"core_source_item_ref":     task.SourceItemRef,
				"core_source_sha256":       task.SourceSHA256,
				"core_task_text_sha256":    task.TaskTextSHA256,
				"core_correlation_key":     task.CorrelationKey,
				"core_request_id":          req.RequestID,
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
		return annotationapp.EngineSubmission{}, annotationapp.NewAnnotationEngineOutcomeError(
			annotationapp.ErrAnnotationEngineInvalidResponse,
			"verify task import response",
			false,
			true,
			0,
			nil,
		)
	}
	external := make(map[uuid.UUID]string, len(req.Tasks))
	for i, externalID := range response.TaskIDs {
		if strings.TrimSpace(externalID.String()) == "" {
			return annotationapp.EngineSubmission{}, annotationapp.NewAnnotationEngineOutcomeError(
				annotationapp.ErrAnnotationEngineInvalidResponse,
				"verify task import response",
				false,
				true,
				0,
				nil,
			)
		}
		external[req.Tasks[i].TaskID] = externalID.String()
	}
	return annotationapp.EngineSubmission{
		State:           annotationapp.EngineLookupUnknown,
		RequestID:       req.RequestID,
		ExternalTaskIDs: external,
		DiagnosticRef:   "task import accepted; persisted payload verification required",
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
			"fields":    []string{"all"},
		}
		var page struct {
			Total int `json:"total"`
			Tasks []struct {
				ID   json.Number    `json:"id"`
				Data map[string]any `json:"data"`
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
			if !remoteTaskMatches(task, remote.Data, remote.Meta) {
				return annotationapp.EngineSubmission{
					State:         annotationapp.EngineLookupConflict,
					RequestID:     req.RequestID,
					DiagnosticRef: "correlated task payload mismatch",
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

func (c *Client) FetchResults(ctx context.Context, req annotationapp.EngineLookupRequest, cursor annotationapp.EngineResultCursor) (annotationapp.EngineResultPage, error) {
	if err := validateBinding(c.instanceRef, req.Binding); err != nil {
		return annotationapp.EngineResultPage{}, err
	}
	if cursor.Offset < 0 {
		return annotationapp.EngineResultPage{}, annotationapp.NewAnnotationEngineError(
			annotationapp.ErrAnnotationEngineInvalidRequest, "fetch results", false, 0, nil,
		)
	}
	query := url.Values{
		"project":   []string{req.Binding.ExternalProjectID},
		"page":      []string{strconv.Itoa(cursor.Offset/100 + 1)},
		"page_size": []string{"100"},
		"fields":    []string{"all"},
	}
	var response struct {
		Total int `json:"total"`
		Tasks []struct {
			ID          json.Number    `json:"id"`
			Data        map[string]any `json:"data"`
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

	expected := make(map[string]annotationapp.EngineTask, len(req.Tasks))
	for _, task := range req.Tasks {
		expected[task.TaskID.String()] = task
	}
	results := make([]annotationapp.EngineResultObservation, 0)
	for _, task := range response.Tasks {
		requestID, _ := task.Meta["core_request_id"].(string)
		fingerprint, _ := task.Meta["core_request_fingerprint"].(string)
		if requestID != req.RequestID || fingerprint != req.RequestFingerprint {
			continue
		}

		coreTaskText, _ := task.Meta["core_task_id"].(string)
		coreTaskID, err := uuid.Parse(strings.TrimSpace(coreTaskText))
		if err != nil {
			return annotationapp.EngineResultPage{}, annotationapp.NewAnnotationEngineError(
				annotationapp.ErrAnnotationEngineInvalidResponse,
				"verify correlated result task identity",
				false,
				0,
				err,
			)
		}
		expectedTask, tracked := expected[coreTaskID.String()]
		if !tracked {
			return annotationapp.EngineResultPage{}, annotationapp.NewAnnotationEngineError(
				annotationapp.ErrAnnotationEngineInvalidResponse,
				"verify correlated result task identity",
				false,
				0,
				nil,
			)
		}
		if !remoteTaskMatches(expectedTask, task.Data, task.Meta) {
			return annotationapp.EngineResultPage{}, annotationapp.NewAnnotationEngineError(
				annotationapp.ErrAnnotationEngineInvalidResponse,
				"verify result task payload",
				false,
				0,
				nil,
			)
		}
		for _, annotation := range task.Annotations {
			if annotation.WasCancelled {
				continue
			}
			label, ok := singleChoiceLabel(annotation.Result)
			if !ok {
				continue
			}
			externalAuthorRef, ok := labelStudioActorRef(annotation.CompletedBy)
			if !ok {
				return annotationapp.EngineResultPage{}, annotationapp.NewAnnotationEngineError(
					annotationapp.ErrAnnotationEngineInvalidResponse,
					"normalize annotation author",
					false,
					0,
					nil,
				)
			}
			payload, _ := json.Marshal(map[string]string{"label": label})
			sum := sha256.Sum256(payload)
			results = append(results, annotationapp.EngineResultObservation{
				TaskID:                 coreTaskID,
				ExternalTaskID:         task.ID.String(),
				ExternalAnnotationID:   annotation.ID.String(),
				ExternalRevision:       strings.TrimSpace(annotation.UpdatedAt),
				ExternalAuthorRef:      externalAuthorRef,
				CanonicalPayload:       payload,
				CanonicalPayloadSHA256: hex.EncodeToString(sum[:]),
				NormalizerVersion:      "labelstudio-single-label-v1",
				ProviderSubmitted:      true,
			})
		}
	}
	var next *annotationapp.EngineResultCursor
	currentPage := cursor.Offset/100 + 1
	if response.Total > 0 {
		if currentPage*100 < response.Total {
			next = &annotationapp.EngineResultCursor{Offset: cursor.Offset + 100}
		}
	} else if response.Next != nil && strings.TrimSpace(fmt.Sprint(response.Next)) != "" {
		next = &annotationapp.EngineResultCursor{Offset: cursor.Offset + 100}
	}
	return annotationapp.EngineResultPage{Results: results, NextCursor: next}, nil
}

func labelStudioActorRef(value any) (string, bool) {
	switch actor := value.(type) {
	case json.Number:
		ref := strings.TrimSpace(actor.String())
		return ref, ref != ""
	case string:
		ref := strings.TrimSpace(actor)
		return ref, ref != ""
	case float64:
		ref := strconv.FormatInt(int64(actor), 10)
		return ref, actor == float64(int64(actor))
	case map[string]any:
		for _, key := range []string{"id", "pk"} {
			if candidate, ok := actor[key]; ok {
				if ref, ok := labelStudioActorRef(candidate); ok {
					return ref, true
				}
			}
		}
	}
	return "", false
}

func remoteTaskMatches(
	task annotationapp.EngineTask,
	data map[string]any,
	meta map[string]any,
) bool {
	text, _ := data["text"].(string)
	sourceItemRef, _ := meta["core_source_item_ref"].(string)
	sourceSHA256, _ := meta["core_source_sha256"].(string)
	taskTextSHA256, _ := meta["core_task_text_sha256"].(string)
	correlationKey, _ := meta["core_correlation_key"].(string)

	return text == task.TaskText &&
		sourceItemRef == task.SourceItemRef &&
		sourceSHA256 == task.SourceSHA256 &&
		taskTextSHA256 == task.TaskTextSHA256 &&
		correlationKey == task.CorrelationKey
}

func (c *Client) verifyProjectConfig(
	ctx context.Context,
	binding annotationapp.EngineCampaignBinding,
) error {
	var project struct {
		ID          json.Number `json:"id"`
		LabelConfig string      `json:"label_config"`
	}
	path := "/api/projects/" + url.PathEscape(binding.ExternalProjectID)
	if err := c.requestJSON(ctx, http.MethodGet, path, nil, nil, &project); err != nil {
		return err
	}
	if strings.TrimSpace(project.ID.String()) != strings.TrimSpace(binding.ExternalProjectID) {
		return annotationapp.NewAnnotationEngineError(
			annotationapp.ErrAnnotationEngineInvalidResponse,
			"verify project identity",
			false,
			0,
			nil,
		)
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(project.LabelConfig)))
	if hex.EncodeToString(sum[:]) != strings.TrimSpace(binding.ConfigSHA256) {
		return annotationapp.NewAnnotationEngineError(
			annotationapp.ErrAnnotationEngineInvalidResponse,
			"verify project config",
			false,
			0,
			nil,
		)
	}
	return nil
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

type singleLabelSchema struct {
	Kind   string   `json:"kind"`
	Labels []string `json:"labels"`
}

func labelConfigFromSchema(schemaContent string) (string, string, error) {
	var schema singleLabelSchema
	if err := json.Unmarshal([]byte(schemaContent), &schema); err != nil {
		return "", "", err
	}
	if schema.Kind != "single-label-v1" || len(schema.Labels) == 0 {
		return "", "", fmt.Errorf("unsupported annotation schema")
	}
	var b strings.Builder
	b.WriteString(`<View><Text name="text" value="$text"/><Choices name="label" toName="text" choice="single">`)
	seen := make(map[string]struct{}, len(schema.Labels))
	for _, label := range schema.Labels {
		label = strings.TrimSpace(label)
		if label == "" {
			return "", "", fmt.Errorf("annotation schema contains empty label")
		}
		if _, ok := seen[label]; ok {
			return "", "", fmt.Errorf("annotation schema contains duplicate label")
		}
		seen[label] = struct{}{}
		b.WriteString(`<Choice value="`)
		if err := xml.EscapeText(&b, []byte(label)); err != nil {
			return "", "", err
		}
		b.WriteString(`"/>`)
	}
	b.WriteString(`</Choices></View>`)
	config := b.String()
	sum := sha256.Sum256([]byte(config))
	return config, hex.EncodeToString(sum[:]), nil
}

func correlationDescription(campaignID uuid.UUID, requestID, fingerprint, schemaSHA string) string {
	return fmt.Sprintf(
		"core_campaign_id=%s core_request_id=%s core_request_fingerprint=%s core_schema_sha256=%s",
		campaignID, strings.TrimSpace(requestID), strings.TrimSpace(fingerprint), strings.TrimSpace(schemaSHA),
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

func (c *Client) authorizationHeader(_ context.Context) (string, error) {
	return "Token " + c.token, nil
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
	authorization, err := c.authorizationHeader(ctx)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", authorization)
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
