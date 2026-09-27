package gx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/tabular"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/domain"
	qualityengine "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/engine"
)

const EngineName = "gx-core"

type Config struct {
	BaseURL               string
	Token                 string
	ExpectedEngineVersion string
	Timeout               time.Duration
}

type Client struct {
	baseURL    string
	token      string
	descriptor qualityengine.Descriptor
	httpClient *http.Client
}

type healthResponse struct {
	Status        string   `json:"status"`
	EngineName    string   `json:"engineName"`
	EngineVersion string   `json:"engineVersion"`
	Capabilities  []string `json:"capabilities"`
}

type evaluateRequest struct {
	AttemptID        string              `json:"attemptId"`
	DatasetVersionID string              `json:"datasetVersionId"`
	RuleSetRef       string              `json:"ruleSetRef"`
	RuleSetContent   string              `json:"ruleSetContent"`
	Headers          []string            `json:"headers"`
	Rows             []map[string]string `json:"rows"`
}

type evaluateResponse struct {
	Engine struct {
		Name         string   `json:"name"`
		Version      string   `json:"version"`
		Capabilities []string `json:"capabilities"`
	} `json:"engine"`
	Findings []struct {
		RuleID   string         `json:"ruleId"`
		Status   string         `json:"status"`
		Observed map[string]any `json:"observed"`
	} `json:"findings"`
	DiagnosticsRef string `json:"diagnosticsRef"`
	Execution      struct {
		Ref            string `json:"ref"`
		DurationMillis int64  `json:"durationMillis"`
	} `json:"execution"`
}

func NewClient(cfg Config, httpClient *http.Client) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	parsed, err := url.ParseRequestURI(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid GX service URL")
	}
	version := strings.TrimSpace(cfg.ExpectedEngineVersion)
	if version == "" {
		return nil, fmt.Errorf("GX expected engine version is required")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	return &Client{
		baseURL: baseURL,
		token:   strings.TrimSpace(cfg.Token),
		descriptor: qualityengine.Descriptor{
			Name:    EngineName,
			Version: version,
			Capabilities: []string{
				"not_null",
				"completeness_ratio",
				"unique",
				"duplicate_ratio",
				"range",
				"enum",
			},
		},
		httpClient: httpClient,
	}, nil
}

func (c *Client) Descriptor() qualityengine.Descriptor {
	descriptor := c.descriptor
	descriptor.Capabilities = append([]string(nil), descriptor.Capabilities...)
	return descriptor
}

func (c *Client) Probe(ctx context.Context) error {
	var health healthResponse
	if err := c.request(ctx, http.MethodGet, "/health", nil, &health); err != nil {
		return err
	}
	if strings.TrimSpace(health.Status) != "ok" ||
		strings.ToLower(strings.TrimSpace(health.EngineName)) != EngineName ||
		strings.TrimSpace(health.EngineVersion) != c.descriptor.Version {
		return qualityengine.NewExecutionError(qualityengine.ErrorProviderInvalidResponse, false)
	}
	reported := make(map[string]struct{}, len(health.Capabilities))
	for _, capability := range health.Capabilities {
		reported[strings.ToLower(strings.TrimSpace(capability))] = struct{}{}
	}
	for _, capability := range c.descriptor.Capabilities {
		if _, ok := reported[capability]; !ok {
			return qualityengine.NewExecutionError(qualityengine.ErrorProviderInvalidResponse, false)
		}
	}
	return nil
}

func (c *Client) Evaluate(ctx context.Context, request qualityengine.Request) (qualityengine.Result, error) {
	payload := evaluateRequest{
		AttemptID:        request.AttemptID.String(),
		DatasetVersionID: request.DatasetVersionID.String(),
		RuleSetRef:       request.RuleSet.Ref,
		RuleSetContent:   string(request.RuleSet.Content),
		Headers:          append([]string(nil), request.Dataset.Table.Headers...),
		Rows:             providerRows(request.Dataset.Table),
	}
	var response evaluateResponse
	if err := c.request(ctx, http.MethodPost, "/v1/evaluate", payload, &response); err != nil {
		return qualityengine.Result{}, err
	}
	if strings.ToLower(strings.TrimSpace(response.Engine.Name)) != EngineName ||
		strings.TrimSpace(response.Engine.Version) != c.descriptor.Version {
		return qualityengine.Result{}, qualityengine.NewExecutionError(qualityengine.ErrorProviderInvalidResponse, false)
	}
	if response.Execution.Ref != request.AttemptID.String() || response.Execution.DurationMillis < 0 {
		return qualityengine.Result{}, qualityengine.NewExecutionError(qualityengine.ErrorProviderInvalidResponse, false)
	}

	findings := make([]domain.Finding, 0, len(response.Findings))
	for _, item := range response.Findings {
		status := domain.FindingStatus(strings.ToUpper(strings.TrimSpace(item.Status)))
		switch status {
		case domain.FindingPass, domain.FindingFail, domain.FindingSkipped:
		default:
			return qualityengine.Result{}, qualityengine.NewExecutionError(qualityengine.ErrorProviderInvalidResponse, false)
		}
		ruleID := strings.TrimSpace(item.RuleID)
		if ruleID == "" {
			return qualityengine.Result{}, qualityengine.NewExecutionError(qualityengine.ErrorProviderInvalidResponse, false)
		}
		findings = append(findings, domain.Finding{
			RuleID:   ruleID,
			Status:   status,
			Observed: item.Observed,
		})
	}
	return qualityengine.Result{
		Findings:       findings,
		DiagnosticsRef: response.DiagnosticsRef,
		Execution: qualityengine.ExecutionMetadata{
			ExecutionRef:   response.Execution.Ref,
			DurationMillis: response.Execution.DurationMillis,
		},
	}, nil
}

func (c *Client) request(ctx context.Context, method, path string, payload any, target any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return qualityengine.NewExecutionError(qualityengine.ErrorProviderExecutionFailed, false)
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return qualityengine.NewExecutionError(qualityengine.ErrorProviderExecutionFailed, false)
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) {
			return qualityengine.NewExecutionError(qualityengine.ErrorProviderTimeout, true)
		}
		if timeoutErr, ok := err.(interface{ Timeout() bool }); ok && timeoutErr.Timeout() {
			return qualityengine.NewExecutionError(qualityengine.ErrorProviderTimeout, true)
		}
		return qualityengine.NewExecutionError(qualityengine.ErrorProviderUnavailable, true)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		if resp.StatusCode >= 500 {
			return qualityengine.NewExecutionError(qualityengine.ErrorProviderUnavailable, true)
		}
		return qualityengine.NewExecutionError(qualityengine.ErrorProviderInvalidResponse, false)
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 8<<20))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return qualityengine.NewExecutionError(qualityengine.ErrorProviderInvalidResponse, false)
	}
	return nil
}

func providerRows(table tabular.Table) []map[string]string {
	result := make([]map[string]string, 0, len(table.Rows))
	for rowIndex := range table.Rows {
		row := make(map[string]string, len(table.Headers))
		for _, header := range table.Headers {
			row[header] = table.RawValue(rowIndex, header)
		}
		result = append(result, row)
	}
	return result
}

var _ qualityengine.Engine = (*Client)(nil)
