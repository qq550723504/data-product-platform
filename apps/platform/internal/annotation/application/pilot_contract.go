package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
)

const (
	pilotSchemaKind   = "single-label-v1"
	pilotRendererKind = "field-list-v1"
)

type pilotSchemaContract struct {
	Kind   string   `json:"kind"`
	Labels []string `json:"labels"`
}

type pilotRendererContract struct {
	Kind   string   `json:"kind"`
	Fields []string `json:"fields"`
}

func validateAnnotationPayload(schema annotationdomain.FrozenSpec, payload []byte) error {
	var contract pilotSchemaContract
	if err := json.Unmarshal([]byte(schema.ContentSnapshot), &contract); err != nil {
		return fmt.Errorf("%w: frozen annotation schema is not valid JSON", annotationdomain.ErrInvalidResult)
	}
	if contract.Kind != pilotSchemaKind || len(contract.Labels) == 0 {
		return fmt.Errorf("%w: unsupported or empty annotation schema", annotationdomain.ErrInvalidResult)
	}
	allowed := make(map[string]struct{}, len(contract.Labels))
	for _, label := range contract.Labels {
		label = strings.TrimSpace(label)
		if label == "" {
			return fmt.Errorf("%w: frozen annotation schema contains empty label", annotationdomain.ErrInvalidResult)
		}
		if _, exists := allowed[label]; exists {
			return fmt.Errorf("%w: frozen annotation schema contains duplicate label", annotationdomain.ErrInvalidResult)
		}
		allowed[label] = struct{}{}
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil {
		return fmt.Errorf("%w: annotation payload is not valid JSON", annotationdomain.ErrInvalidResult)
	}
	if len(raw) != 1 {
		return fmt.Errorf("%w: annotation payload must contain exactly one label field", annotationdomain.ErrInvalidResult)
	}
	labelRaw, ok := raw["label"]
	if !ok {
		return fmt.Errorf("%w: annotation payload is missing label", annotationdomain.ErrInvalidResult)
	}
	var label string
	if err := json.Unmarshal(labelRaw, &label); err != nil || strings.TrimSpace(label) != label || label == "" {
		return fmt.Errorf("%w: annotation label must be a nonempty normalized string", annotationdomain.ErrInvalidResult)
	}
	if _, ok := allowed[label]; !ok {
		return fmt.Errorf("%w: annotation label is not allowed by frozen schema", annotationdomain.ErrInvalidResult)
	}
	return nil
}

func rendererTaskTextSHA256(renderer annotationdomain.FrozenSpec, row map[string]string) (string, error) {
	var contract pilotRendererContract
	if err := json.Unmarshal([]byte(renderer.ContentSnapshot), &contract); err != nil {
		return "", fmt.Errorf("%w: frozen renderer is not valid JSON", annotationdomain.ErrInvalidTask)
	}
	if contract.Kind != pilotRendererKind || len(contract.Fields) == 0 {
		return "", fmt.Errorf("%w: unsupported or empty renderer", annotationdomain.ErrInvalidTask)
	}
	type renderedField struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	type renderedTask struct {
		RendererRef     string          `json:"rendererRef"`
		RendererVersion string          `json:"rendererVersion"`
		Fields          []renderedField `json:"fields"`
	}
	fields := make([]renderedField, 0, len(contract.Fields))
	seen := make(map[string]struct{}, len(contract.Fields))
	for _, name := range contract.Fields {
		name = strings.TrimSpace(name)
		if name == "" {
			return "", fmt.Errorf("%w: renderer contains empty field", annotationdomain.ErrInvalidTask)
		}
		if _, exists := seen[name]; exists {
			return "", fmt.Errorf("%w: renderer contains duplicate field", annotationdomain.ErrInvalidTask)
		}
		seen[name] = struct{}{}
		value, exists := row[name]
		if !exists {
			return "", fmt.Errorf("%w: renderer field %q is missing from input", annotationdomain.ErrInvalidTask, name)
		}
		fields = append(fields, renderedField{Name: name, Value: value})
	}
	encoded, err := json.Marshal(renderedTask{
		RendererRef: strings.TrimSpace(renderer.Ref), RendererVersion: strings.TrimSpace(renderer.Version), Fields: fields,
	})
	if err != nil {
		return "", err
	}
	return hashBytes(encoded), nil
}

func validateFrozenPilotContracts(campaign annotationdomain.Campaign) error {
	if err := validateFrozenSchemaContract(campaign.Schema); err != nil {
		return err
	}
	var renderer pilotRendererContract
	if err := json.Unmarshal([]byte(campaign.Renderer.ContentSnapshot), &renderer); err != nil {
		return fmt.Errorf("%w: frozen renderer is not valid JSON", annotationdomain.ErrInvalidCampaign)
	}
	if renderer.Kind != pilotRendererKind || len(renderer.Fields) == 0 {
		return fmt.Errorf("%w: unsupported or empty frozen renderer", annotationdomain.ErrInvalidCampaign)
	}
	return nil
}

func validateFrozenSchemaContract(schema annotationdomain.FrozenSpec) error {
	var contract pilotSchemaContract
	if err := json.Unmarshal([]byte(schema.ContentSnapshot), &contract); err != nil {
		return fmt.Errorf("%w: frozen schema is not valid JSON", annotationdomain.ErrInvalidCampaign)
	}
	if contract.Kind != pilotSchemaKind || len(contract.Labels) == 0 {
		return fmt.Errorf("%w: unsupported or empty frozen schema", annotationdomain.ErrInvalidCampaign)
	}
	seen := make(map[string]struct{}, len(contract.Labels))
	for _, label := range contract.Labels {
		label = strings.TrimSpace(label)
		if label == "" {
			return fmt.Errorf("%w: frozen schema contains empty label", annotationdomain.ErrInvalidCampaign)
		}
		if _, exists := seen[label]; exists {
			return fmt.Errorf("%w: frozen schema contains duplicate label", annotationdomain.ErrInvalidCampaign)
		}
		seen[label] = struct{}{}
	}
	if len(seen) != len(contract.Labels) {
		return errors.New("unreachable duplicate annotation schema labels")
	}
	return nil
}
