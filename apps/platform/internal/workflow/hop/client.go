package hop

import (
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
)

const hopDateLayout = "2006-01-02T15:04:05.000-0700"

type Client struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
}

func NewClient(baseURL, username, password string, httpClient *http.Client) (*Client, error) {
	baseURL, err := normalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(username) == "" || password == "" {
		return nil, fmt.Errorf("Hop Server username and password are required")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		baseURL:    baseURL,
		username:   username,
		password:   password,
		httpClient: httpClient,
	}, nil
}

func normalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("Hop Server base URL is required")
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid Hop Server base URL %q", raw)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("Hop Server base URL must not contain query or fragment")
	}
	path := strings.TrimRight(parsed.Path, "/")
	if path == "/hop" || strings.HasSuffix(path, "/hop") {
		path = strings.TrimSuffix(path, "/hop")
	}
	parsed.Path = path
	parsed.RawPath = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

type webResult struct {
	XMLName xml.Name `xml:"webresult"`
	Result  string   `xml:"result"`
	Message string   `xml:"message"`
	ID      string   `xml:"id"`
}

type pipelineStatus struct {
	ID                  string           `json:"id"`
	PipelineName        string           `json:"pipelineName"`
	StatusDescription   string           `json:"statusDescription"`
	ErrorDescription    string           `json:"errorDescription"`
	LoggingString       string           `json:"loggingString"`
	FirstLoggingLineNr  int              `json:"firstLoggingLineNr"`
	LastLoggingLineNr   int              `json:"lastLoggingLineNr"`
	TransformStatusList []map[string]any `json:"transformStatusList"`
	Result              map[string]any   `json:"result"`
	Paused              bool             `json:"paused"`
	ExecutionStartDate  *string          `json:"executionStartDate"`
	ExecutionEndDate    *string          `json:"executionEndDate"`
}

func (c *Client) Submit(ctx context.Context, request workflowapp.ManagedSubmitRequest) (workflowapp.EngineRun, error) {
	name := strings.TrimSpace(request.Name)
	if name == "" || len(request.Definition) == 0 {
		return workflowapp.EngineRun{}, workflowapp.NewManagedEngineError(
			workflowapp.ManagedEngineInvalidRequest, "submit", false, 0,
			fmt.Errorf("pipeline name and pipeline configuration are required"),
		)
	}
	for key := range request.Parameters {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "name", "id", "xml":
			return workflowapp.EngineRun{}, workflowapp.NewManagedEngineError(
				workflowapp.ManagedEngineInvalidRequest, "submit", false, 0,
				fmt.Errorf("parameter %q is reserved for remote run identity", key),
			)
		}
	}

	registerQuery := url.Values{"xml": []string{"Y"}}
	registered, err := c.webResultRequest(ctx, http.MethodPost, "/hop/registerPipeline", registerQuery, request.Definition, request.ContentType)
	if err != nil {
		return workflowapp.EngineRun{}, fmt.Errorf("register Hop pipeline %q: %w", name, err)
	}
	if strings.TrimSpace(registered.ID) == "" {
		return workflowapp.EngineRun{}, workflowapp.NewManagedEngineError(
			workflowapp.ManagedEngineInvalidResponse, "register pipeline", false, 0,
			fmt.Errorf("remote registration returned no execution id"),
		)
	}

	startQuery := url.Values{
		"name": []string{name},
		"id":   []string{registered.ID},
		"xml":  []string{"Y"},
	}
	for key, value := range request.Parameters {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		startQuery.Set(key, value)
	}
	if _, err := c.webResultRequest(ctx, http.MethodGet, "/hop/startPipeline", startQuery, nil, ""); err != nil {
		return workflowapp.EngineRun{}, fmt.Errorf("start Hop pipeline %q (%s): %w", name, registered.ID, err)
	}

	run, err := c.Status(ctx, name, registered.ID)
	if err != nil {
		// Registration/start already succeeded. Preserve the external id even if the
		// immediate status probe races Hop Server startup; the reconciler can recover.
		return workflowapp.EngineRun{
			ID:      registered.ID,
			Name:    name,
			State:   workflowapp.EngineRunQueued,
			Metrics: map[string]any{"definitionRef": request.DefinitionRef},
		}, nil
	}
	if run.Metrics == nil {
		run.Metrics = map[string]any{}
	}
	run.Metrics["definitionRef"] = request.DefinitionRef
	return run, nil
}

func (c *Client) Status(ctx context.Context, name, runID string) (workflowapp.EngineRun, error) {
	status, err := c.getStatus(ctx, name, runID, 0)
	if err != nil {
		return workflowapp.EngineRun{}, err
	}
	return mapStatus(status), nil
}

func (c *Client) Cancel(ctx context.Context, name, runID string) error {
	query, err := runQuery(name, runID)
	if err != nil {
		return err
	}
	query.Set("xml", "Y")
	_, err = c.webResultRequest(ctx, http.MethodGet, "/hop/stopPipeline", query, nil, "")
	if err != nil {
		return fmt.Errorf("stop Hop pipeline %q (%s): %w", name, runID, err)
	}
	return nil
}

func (c *Client) Logs(ctx context.Context, name, runID string, from int) (workflowapp.EngineLogPage, error) {
	if from < 0 {
		return workflowapp.EngineLogPage{}, workflowapp.NewManagedEngineError(
			workflowapp.ManagedEngineInvalidRequest, "read logs", false, 0,
			fmt.Errorf("log offset must be >= 0"),
		)
	}
	status, err := c.getStatus(ctx, name, runID, from)
	if err != nil {
		return workflowapp.EngineLogPage{}, err
	}
	text, err := decodeLoggingString(status.LoggingString)
	if err != nil {
		return workflowapp.EngineLogPage{}, fmt.Errorf("decode Hop pipeline logs: %w", err)
	}
	next := status.LastLoggingLineNr + 1
	if next < from {
		next = from
	}
	return workflowapp.EngineLogPage{
		Text:       text,
		From:       status.FirstLoggingLineNr,
		NextOffset: next,
	}, nil
}

func (c *Client) Metrics(ctx context.Context, name, runID string) (map[string]any, error) {
	status, err := c.getStatus(ctx, name, runID, 0)
	if err != nil {
		return nil, err
	}
	return statusMetrics(status), nil
}

func (c *Client) getStatus(ctx context.Context, name, runID string, from int) (pipelineStatus, error) {
	query, err := runQuery(name, runID)
	if err != nil {
		return pipelineStatus{}, err
	}
	query.Set("json", "Y")
	query.Set("from", strconv.Itoa(from))

	var status pipelineStatus
	if err := c.jsonRequest(ctx, http.MethodGet, "/hop/pipelineStatus", query, nil, "", &status); err != nil {
		return pipelineStatus{}, fmt.Errorf("get Hop pipeline status %q (%s): %w", name, runID, err)
	}
	if strings.TrimSpace(status.ID) == "" {
		status.ID = runID
	}
	if strings.TrimSpace(status.PipelineName) == "" {
		status.PipelineName = name
	}
	return status, nil
}

func mapStatus(status pipelineStatus) workflowapp.EngineRun {
	nrErrors := resultErrors(status.Result)
	description := strings.ToLower(strings.TrimSpace(status.StatusDescription))
	state := workflowapp.EngineRunUnknown
	switch {
	case strings.Contains(description, "finished with errors"),
		strings.Contains(description, "stopped with errors"),
		strings.Contains(description, "failed"),
		strings.Contains(description, "error"):
		state = workflowapp.EngineRunFailed
	case strings.Contains(description, "finished"):
		if nrErrors > 0 {
			state = workflowapp.EngineRunFailed
		} else {
			state = workflowapp.EngineRunSucceeded
		}
	case strings.Contains(description, "stopped"), strings.Contains(description, "cancel"):
		if nrErrors > 0 {
			state = workflowapp.EngineRunFailed
		} else {
			state = workflowapp.EngineRunCancelled
		}
	case strings.Contains(description, "running"),
		strings.Contains(description, "initializing"),
		strings.Contains(description, "paused"),
		strings.Contains(description, "waiting"):
		state = workflowapp.EngineRunRunning
	case description == "":
		state = workflowapp.EngineRunUnknown
	}

	return workflowapp.EngineRun{
		ID:           status.ID,
		Name:         status.PipelineName,
		State:        state,
		StartedAt:    parseHopTime(status.ExecutionStartDate),
		FinishedAt:   parseHopTime(status.ExecutionEndDate),
		ErrorMessage: managedRunErrorMessage(state),
		Metrics:      statusMetrics(status),
	}
}

func managedRunErrorMessage(state workflowapp.EngineRunState) string {
	switch state {
	case workflowapp.EngineRunFailed:
		return "remote processing engine reported failure"
	case workflowapp.EngineRunCancelled:
		return "remote processing engine cancelled execution"
	default:
		return ""
	}
}

func statusMetrics(status pipelineStatus) map[string]any {
	return map[string]any{
		"statusDescription":  status.StatusDescription,
		"paused":             status.Paused,
		"firstLoggingLineNr": status.FirstLoggingLineNr,
		"lastLoggingLineNr":  status.LastLoggingLineNr,
		"transformCount":     len(status.TransformStatusList),
		"nrErrors":           resultErrors(status.Result),
		"result":             status.Result,
		"transformStatus":    status.TransformStatusList,
	}
}

func resultErrors(result map[string]any) int64 {
	if result == nil {
		return 0
	}
	for _, key := range []string{"nrErrors", "errors"} {
		value, ok := result[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return int64(typed)
		case json.Number:
			parsed, _ := typed.Int64()
			return parsed
		case string:
			parsed, _ := strconv.ParseInt(typed, 10, 64)
			return parsed
		}
	}
	return 0
}

func parseHopTime(value *string) *time.Time {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	for _, layout := range []string{hopDateLayout, time.RFC3339Nano, time.RFC3339} {
		parsed, err := time.Parse(layout, strings.TrimSpace(*value))
		if err == nil {
			return &parsed
		}
	}
	return nil
}

func runQuery(name, runID string) (url.Values, error) {
	name = strings.TrimSpace(name)
	runID = strings.TrimSpace(runID)
	if name == "" || runID == "" {
		return nil, workflowapp.NewManagedEngineError(
			workflowapp.ManagedEngineInvalidRequest, "run identity", false, 0,
			fmt.Errorf("pipeline name and external execution id are required"),
		)
	}
	return url.Values{"name": []string{name}, "id": []string{runID}}, nil
}

func (c *Client) webResultRequest(ctx context.Context, method, endpoint string, query url.Values, body []byte, contentType string) (webResult, error) {
	var response webResult
	raw, err := c.do(ctx, method, endpoint, query, body, contentType)
	if err != nil {
		return response, err
	}
	if err := xml.Unmarshal(raw, &response); err != nil {
		return response, workflowapp.NewManagedEngineError(
			workflowapp.ManagedEngineInvalidResponse, "decode response", false, 0, err,
		)
	}
	if !strings.EqualFold(strings.TrimSpace(response.Result), "OK") {
		return response, workflowapp.NewManagedEngineError(
			workflowapp.ManagedEngineRejected, "provider result", false, 0,
			fmt.Errorf("result=%q message=%q", response.Result, strings.TrimSpace(response.Message)),
		)
	}
	return response, nil
}

func (c *Client) jsonRequest(ctx context.Context, method, endpoint string, query url.Values, body []byte, contentType string, target any) error {
	raw, err := c.do(ctx, method, endpoint, query, body, contentType)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return workflowapp.NewManagedEngineError(
			workflowapp.ManagedEngineInvalidResponse, "decode response", false, 0, err,
		)
	}
	return nil
}

func (c *Client) do(ctx context.Context, method, endpoint string, query url.Values, body []byte, contentType string) ([]byte, error) {
	requestURL := c.baseURL + endpoint
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	var reader io.Reader
	if body != nil {
		reader = strings.NewReader(string(body))
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reader)
	if err != nil {
		return nil, workflowapp.NewManagedEngineError(
			workflowapp.ManagedEngineInvalidRequest, "create request", false, 0, err,
		)
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Accept", "application/json, application/xml, text/xml")
	if body != nil {
		if strings.TrimSpace(contentType) == "" {
			contentType = "application/xml"
		}
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, workflowapp.NewManagedEngineError(
			workflowapp.ManagedEngineUnavailable, "request", true, 0, err,
		)
	}
	defer resp.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if readErr != nil {
		return nil, workflowapp.NewManagedEngineError(
			workflowapp.ManagedEngineUnavailable, "read response", true, resp.StatusCode, readErr,
		)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		kind, retryable := classifyHTTPStatus(resp.StatusCode)
		return nil, workflowapp.NewManagedEngineError(
			kind, "request", retryable, resp.StatusCode,
			fmt.Errorf("method=%s endpoint=%s body=%q", method, endpoint, strings.TrimSpace(string(raw))),
		)
	}
	return raw, nil
}

func classifyHTTPStatus(status int) (workflowapp.ManagedEngineErrorKind, bool) {
	switch {
	case status == http.StatusBadRequest || status == http.StatusUnprocessableEntity:
		return workflowapp.ManagedEngineInvalidRequest, false
	case status == http.StatusNotFound:
		return workflowapp.ManagedEngineNotFound, false
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return workflowapp.ManagedEngineUnauthorized, false
	case status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= http.StatusInternalServerError:
		return workflowapp.ManagedEngineUnavailable, true
	default:
		return workflowapp.ManagedEngineRejected, false
	}
}

func decodeLoggingString(encoded string) (string, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return "", nil
	}
	compressed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", workflowapp.NewManagedEngineError(
			workflowapp.ManagedEngineInvalidResponse, "decode logs", false, 0, err,
		)
	}
	reader, err := gzip.NewReader(strings.NewReader(string(compressed)))
	if err != nil {
		return "", workflowapp.NewManagedEngineError(
			workflowapp.ManagedEngineInvalidResponse, "decode logs", false, 0, err,
		)
	}
	defer reader.Close()
	decoded, err := io.ReadAll(reader)
	if err != nil {
		return "", workflowapp.NewManagedEngineError(
			workflowapp.ManagedEngineInvalidResponse, "decode logs", false, 0, err,
		)
	}
	return string(decoded), nil
}

var _ workflowapp.ManagedProcessingEngine = (*Client)(nil)
