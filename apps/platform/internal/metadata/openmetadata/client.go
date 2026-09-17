package openmetadata

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	metadataengine "github.com/qq550723504/data-product-platform/apps/platform/internal/engine/metadata"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewClient(baseURL, token string, httpClient *http.Client) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("OpenMetadata base URL is required")
	}
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("parse OpenMetadata base URL: %w", err)
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{baseURL: baseURL, token: strings.TrimSpace(token), httpClient: httpClient}, nil
}

func (c *Client) GetAsset(ctx context.Context, entityType, fullyQualifiedName string) (metadataengine.Asset, error) {
	entityType = strings.ToUpper(strings.TrimSpace(entityType))
	fullyQualifiedName = strings.TrimSpace(fullyQualifiedName)
	if fullyQualifiedName == "" {
		return metadataengine.Asset{}, fmt.Errorf("asset fully qualified name is required")
	}
	var endpoint string
	switch entityType {
	case "TABLE":
		endpoint = "/v1/tables/name/" + url.PathEscape(fullyQualifiedName)
	default:
		return metadataengine.Asset{}, fmt.Errorf("unsupported OpenMetadata asset type %q", entityType)
	}

	var response struct {
		ID                 string `json:"id"`
		Name               string `json:"name"`
		FullyQualifiedName string `json:"fullyQualifiedName"`
		DisplayName        string `json:"displayName"`
		Description        string `json:"description"`
		ServiceType        string `json:"serviceType"`
		Version            any    `json:"version"`
	}
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
		return metadataengine.Asset{}, err
	}
	return metadataengine.Asset{
		ID:                 response.ID,
		EntityType:         entityType,
		Name:               response.Name,
		FullyQualifiedName: response.FullyQualifiedName,
		DisplayName:        response.DisplayName,
		Description:        response.Description,
		Metadata: map[string]any{
			"serviceType": response.ServiceType,
			"version":     response.Version,
		},
	}, nil
}

func (c *Client) UpsertDataProduct(ctx context.Context, product metadataengine.GovernanceProduct) (metadataengine.ExternalEntity, error) {
	product.Name = strings.TrimSpace(product.Name)
	product.Domain = strings.TrimSpace(product.Domain)
	if product.Name == "" || product.Domain == "" {
		return metadataengine.ExternalEntity{}, fmt.Errorf("OpenMetadata data product name and domain are required")
	}
	request := map[string]any{
		"name":        product.Name,
		"displayName": strings.TrimSpace(product.DisplayName),
		"description": strings.TrimSpace(product.Description),
		"domain":      product.Domain,
	}
	var response struct {
		ID                 string `json:"id"`
		Name               string `json:"name"`
		FullyQualifiedName string `json:"fullyQualifiedName"`
		Version            any    `json:"version"`
	}
	if err := c.doJSON(ctx, http.MethodPut, "/v1/dataProducts", request, &response); err != nil {
		return metadataengine.ExternalEntity{}, err
	}
	return metadataengine.ExternalEntity{
		ID:                 response.ID,
		EntityType:         "DATA_PRODUCT",
		FullyQualifiedName: response.FullyQualifiedName,
		Metadata: map[string]any{
			"name":    response.Name,
			"version": response.Version,
		},
	}, nil
}

func (c *Client) doJSON(ctx context.Context, method, endpoint string, requestBody any, responseBody any) error {
	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("encode OpenMetadata request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, body)
	if err != nil {
		return fmt.Errorf("create OpenMetadata request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("OpenMetadata request %s %s: %w", method, endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		limited, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return fmt.Errorf("OpenMetadata request %s %s returned %d: %s", method, endpoint, resp.StatusCode, strings.TrimSpace(string(limited)))
	}
	if responseBody == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(responseBody); err != nil {
		return fmt.Errorf("decode OpenMetadata response: %w", err)
	}
	return nil
}

var _ metadataengine.Engine = (*Client)(nil)
