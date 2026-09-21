package openmetadata

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
		return metadataengine.Asset{}, metadataengine.NewExternalError(metadataengine.ErrorInvalidRequest, "get asset", 0, errors.New("asset fully qualified name is required"))
	}
	var endpoint string
	switch entityType {
	case "TABLE":
		endpoint = "/v1/tables/name/" + url.PathEscape(fullyQualifiedName)
	default:
		return metadataengine.Asset{}, metadataengine.NewExternalError(metadataengine.ErrorInvalidRequest, "get asset", 0, fmt.Errorf("unsupported metadata asset type %q", entityType))
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
		return metadataengine.ExternalEntity{}, metadataengine.NewExternalError(metadataengine.ErrorInvalidRequest, "upsert data product", 0, errors.New("data product name and domain are required"))
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
			return metadataengine.NewExternalError(metadataengine.ErrorInvalidRequest, "encode request", 0, err)
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, body)
	if err != nil {
		return metadataengine.NewExternalError(metadataengine.ErrorInvalidRequest, "create request", 0, err)
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
		return metadataengine.NewExternalError(metadataengine.ErrorUnavailable, "request", 0, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		limited, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		cause := errors.New(strings.TrimSpace(string(limited)))
		return metadataengine.NewExternalError(classifyStatus(resp.StatusCode), "request", resp.StatusCode, cause)
	}
	if responseBody == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(responseBody); err != nil {
		return metadataengine.NewExternalError(metadataengine.ErrorInvalidResponse, "decode response", resp.StatusCode, err)
	}
	return nil
}

func classifyStatus(status int) metadataengine.ErrorKind {
	switch {
	case status == http.StatusBadRequest || status == http.StatusUnprocessableEntity:
		return metadataengine.ErrorInvalidRequest
	case status == http.StatusNotFound:
		return metadataengine.ErrorNotFound
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return metadataengine.ErrorUnauthorized
	case status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= http.StatusInternalServerError:
		return metadataengine.ErrorUnavailable
	default:
		return metadataengine.ErrorRejected
	}
}

var _ metadataengine.Engine = (*Client)(nil)
