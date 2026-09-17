package splink

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

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/resolution"
)

var (
	ErrUnavailable     = errors.New("entity resolution engine unavailable")
	ErrInvalidResponse = errors.New("entity resolution engine returned an invalid response")
)

type Config struct {
	BaseURL               string
	Token                 string
	ExpectedEngineVersion string
	ModelRef              string
	ModelVersion          string
	Timeout               time.Duration
}

type Client struct {
	baseURL    string
	token      string
	descriptor resolution.EngineDescriptor
	modelRef   string
	httpClient *http.Client
}

type Health struct {
	Status        string `json:"status"`
	EngineName    string `json:"engineName"`
	EngineVersion string `json:"engineVersion"`
	ModelRef      string `json:"modelRef"`
	ModelVersion  string `json:"modelVersion"`
}

func NewClient(cfg Config, httpClient *http.Client) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	parsed, err := url.ParseRequestURI(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid Splink service URL %q", cfg.BaseURL)
	}
	modelRef := strings.TrimSpace(cfg.ModelRef)
	modelVersion := strings.TrimSpace(cfg.ModelVersion)
	if modelRef == "" || modelVersion == "" {
		return nil, fmt.Errorf("Splink model ref and version are required")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	return &Client{
		baseURL: baseURL,
		token:   strings.TrimSpace(cfg.Token),
		descriptor: resolution.EngineDescriptor{
			Name:         "SPLINK",
			Version:      strings.TrimSpace(cfg.ExpectedEngineVersion),
			ModelVersion: modelVersion,
		},
		modelRef:   modelRef,
		httpClient: httpClient,
	}, nil
}

func (c *Client) Descriptor() resolution.EngineDescriptor {
	return c.descriptor
}

func (c *Client) Probe(ctx context.Context) (Health, error) {
	var health Health
	if err := c.request(ctx, http.MethodGet, "/health", nil, &health); err != nil {
		return Health{}, err
	}
	if !strings.EqualFold(strings.TrimSpace(health.Status), "ok") ||
		!strings.EqualFold(strings.TrimSpace(health.EngineName), "SPLINK") ||
		strings.TrimSpace(health.EngineVersion) == "" {
		return Health{}, fmt.Errorf("%w: health contract", ErrInvalidResponse)
	}
	if expected := strings.TrimSpace(c.descriptor.Version); expected != "" && health.EngineVersion != expected {
		return Health{}, fmt.Errorf("%w: expected Splink %s, service reports %s", ErrInvalidResponse, expected, health.EngineVersion)
	}
	if health.ModelRef != "" && health.ModelRef != c.modelRef {
		return Health{}, fmt.Errorf("%w: expected model ref %s, service reports %s", ErrInvalidResponse, c.modelRef, health.ModelRef)
	}
	if health.ModelVersion != "" && health.ModelVersion != c.descriptor.ModelVersion {
		return Health{}, fmt.Errorf("%w: expected model version %s, service reports %s", ErrInvalidResponse, c.descriptor.ModelVersion, health.ModelVersion)
	}
	return health, nil
}

type candidateRequest struct {
	EntityType    string            `json:"entityType"`
	Source        matchRecord       `json:"source"`
	References    []referenceRecord `json:"references"`
	PolicyRef     string            `json:"policyRef"`
	PolicyVersion string            `json:"policyVersion"`
	ModelRef      string            `json:"modelRef"`
	ModelVersion  string            `json:"modelVersion"`
}

type matchRecord struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Fields map[string]string `json:"fields"`
}

type referenceRecord struct {
	EntityID string            `json:"entityId"`
	Name     string            `json:"name"`
	Fields   map[string]string `json:"fields"`
}

type candidateResponse struct {
	Engine struct {
		Name         string `json:"name"`
		Version      string `json:"version"`
		ModelVersion string `json:"modelVersion"`
	} `json:"engine"`
	Candidates []struct {
		EntityID string         `json:"entityId"`
		Score    float64        `json:"score"`
		Method   string         `json:"method"`
		Metadata map[string]any `json:"metadata"`
	} `json:"candidates"`
}

func (c *Client) Generate(ctx context.Context, request resolution.CandidateRequest) ([]resolution.Candidate, error) {
	payload := candidateRequest{
		EntityType:    request.EntityType,
		Source:        matchRecord{ID: request.Source.ID, Name: request.Source.Name, Fields: request.Source.Fields},
		PolicyRef:     request.PolicyRef,
		PolicyVersion: request.PolicyVersion,
		ModelRef:      c.modelRef,
		ModelVersion:  c.descriptor.ModelVersion,
		References:    make([]referenceRecord, 0, len(request.References)),
	}
	for _, reference := range request.References {
		payload.References = append(payload.References, referenceRecord{
			EntityID: reference.EntityID.String(),
			Name:     reference.Name,
			Fields:   reference.Fields,
		})
	}

	var response candidateResponse
	if err := c.request(ctx, http.MethodPost, "/v1/candidates", payload, &response); err != nil {
		return nil, err
	}
	descriptor := resolution.EngineDescriptor{
		Name:         strings.ToUpper(strings.TrimSpace(response.Engine.Name)),
		Version:      strings.TrimSpace(response.Engine.Version),
		ModelVersion: strings.TrimSpace(response.Engine.ModelVersion),
	}
	if descriptor.Name != "SPLINK" || descriptor.Version == "" || descriptor.ModelVersion == "" {
		return nil, fmt.Errorf("%w: missing engine provenance", ErrInvalidResponse)
	}
	if expected := strings.TrimSpace(c.descriptor.Version); expected != "" && descriptor.Version != expected {
		return nil, fmt.Errorf("%w: expected Splink %s, response reports %s", ErrInvalidResponse, expected, descriptor.Version)
	}
	if descriptor.ModelVersion != c.descriptor.ModelVersion {
		return nil, fmt.Errorf("%w: expected model version %s, response reports %s", ErrInvalidResponse, c.descriptor.ModelVersion, descriptor.ModelVersion)
	}

	result := make([]resolution.Candidate, 0, len(response.Candidates))
	for i, candidate := range response.Candidates {
		entityID, err := uuid.Parse(candidate.EntityID)
		if err != nil {
			return nil, fmt.Errorf("%w: candidate %d entityId", ErrInvalidResponse, i)
		}
		converted := resolution.Candidate{
			EntityID: entityID,
			Score:    candidate.Score,
			Method:   strings.TrimSpace(candidate.Method),
			Engine:   descriptor,
			Metadata: candidate.Metadata,
		}
		if err := converted.Validate(); err != nil {
			return nil, fmt.Errorf("%w: candidate %d: %v", ErrInvalidResponse, i, err)
		}
		result = append(result, converted)
	}
	return result, nil
}

func (c *Client) request(ctx context.Context, method, path string, payload any, target any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode entity resolution request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("create entity resolution request: %w", err)
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
		return fmt.Errorf("%w: request failed", ErrUnavailable)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return fmt.Errorf("%w: HTTP %d", ErrUnavailable, resp.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 8<<20))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: decode JSON", ErrInvalidResponse)
	}
	return nil
}

var _ resolution.CandidateGenerator = (*Client)(nil)
