package evidence

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"
)

const (
	HashAlgorithmLegacy     = "SHA256"
	HashAlgorithmEvidenceV1 = "SHA256-EVIDENCE-V1"
)

type hashEnvelopeV1 struct {
	WorkspaceID  string         `json:"workspaceId"`
	EvidenceType string         `json:"evidenceType"`
	Title        string         `json:"title,omitempty"`
	SourceType   string         `json:"sourceType,omitempty"`
	SourceID     string         `json:"sourceId,omitempty"`
	StorageURI   string         `json:"storageUri,omitempty"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    time.Time      `json:"createdAt"`
	CreatedBy    string         `json:"createdBy,omitempty"`
}

func NormalizeCreatedAt(value time.Time) time.Time {
	return value.UTC().Truncate(time.Microsecond)
}

func ComputeHash(record Record, algorithm string) (string, error) {
	algorithm = strings.ToUpper(strings.TrimSpace(algorithm))
	var payload []byte
	var err error
	switch algorithm {
	case HashAlgorithmLegacy:
		payload, err = json.Marshal(nonNilMetadata(record.Metadata))
	case HashAlgorithmEvidenceV1:
		metadata, normalizeErr := canonicalMetadata(record.Metadata)
		if normalizeErr != nil {
			return "", normalizeErr
		}
		sourceID := ""
		if record.SourceID != nil {
			sourceID = record.SourceID.String()
		}
		createdBy := ""
		if record.CreatedBy != nil {
			createdBy = record.CreatedBy.String()
		}
		payload, err = json.Marshal(hashEnvelopeV1{
			WorkspaceID:  record.WorkspaceID.String(),
			EvidenceType: record.EvidenceType,
			Title:        record.Title,
			SourceType:   record.SourceType,
			SourceID:     sourceID,
			StorageURI:   record.StorageURI,
			Metadata:     metadata,
			CreatedAt:    NormalizeCreatedAt(record.CreatedAt),
			CreatedBy:    createdBy,
		})
	default:
		return "", fmt.Errorf("unsupported evidence hash algorithm %q", algorithm)
	}
	if err != nil {
		return "", fmt.Errorf("marshal evidence hash payload: %w", err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func VerifyHash(record Record, algorithm, expected string) bool {
	actual, err := ComputeHash(record, algorithm)
	if err != nil {
		return false
	}
	return strings.EqualFold(actual, strings.TrimSpace(expected))
}

func nonNilMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return map[string]any{}
	}
	return metadata
}

// canonicalMetadata makes Evidence V1 hashes independent of whether nested
// metadata was held as typed Go structs or decoded from PostgreSQL jsonb maps.
// jsonb canonicalizes object key order, so hashing the original struct-shaped
// value would otherwise produce a different digest after a read round-trip.
func canonicalMetadata(metadata map[string]any) (map[string]any, error) {
	encoded, err := json.Marshal(nonNilMetadata(metadata))
	if err != nil {
		return nil, fmt.Errorf("marshal canonical Evidence metadata: %w", err)
	}
	var canonical map[string]any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&canonical); err != nil {
		return nil, fmt.Errorf("normalize canonical Evidence metadata: %w", err)
	}
	normalized, err := normalizeJSONNumbers(canonical)
	if err != nil {
		return nil, fmt.Errorf("normalize canonical Evidence numbers: %w", err)
	}
	var ok bool
	canonical, ok = normalized.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("canonical Evidence metadata must be an object")
	}
	if canonical == nil {
		canonical = map[string]any{}
	}
	return canonical, nil
}

func decodeMetadata(encoded []byte, metadata *map[string]any) error {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	return decoder.Decode(metadata)
}

func decodeMetadataForHash(encoded []byte, algorithm string, metadata *map[string]any) error {
	if strings.EqualFold(strings.TrimSpace(algorithm), HashAlgorithmLegacy) {
		// Preserve the legacy float64 decode/re-marshal behavior for records
		// written before Evidence V1. V1 uses json.Number plus canonicalization.
		return json.Unmarshal(encoded, metadata)
	}
	return decodeMetadata(encoded, metadata)
}

func normalizeJSONNumbers(value any) (any, error) {
	switch typed := value.(type) {
	case json.Number:
		return canonicalJSONNumber(typed)
	case map[string]any:
		for key, child := range typed {
			normalized, err := normalizeJSONNumbers(child)
			if err != nil {
				return nil, err
			}
			typed[key] = normalized
		}
		return typed, nil
	case []any:
		for index, child := range typed {
			normalized, err := normalizeJSONNumbers(child)
			if err != nil {
				return nil, err
			}
			typed[index] = normalized
		}
		return typed, nil
	default:
		return value, nil
	}
}

func canonicalJSONNumber(value json.Number) (json.Number, error) {
	rat, ok := new(big.Rat).SetString(value.String())
	if !ok {
		return "", fmt.Errorf("invalid JSON number %q", value)
	}
	if rat.Sign() == 0 {
		return "0", nil
	}

	denominator := new(big.Int).Set(rat.Denom())
	twoCount, fiveCount := 0, 0
	two := big.NewInt(2)
	five := big.NewInt(5)
	zero := big.NewInt(0)
	for new(big.Int).Mod(denominator, two).Cmp(zero) == 0 {
		denominator.Div(denominator, two)
		twoCount++
	}
	for new(big.Int).Mod(denominator, five).Cmp(zero) == 0 {
		denominator.Div(denominator, five)
		fiveCount++
	}
	if denominator.Cmp(big.NewInt(1)) != 0 {
		return "", fmt.Errorf("JSON number %q has a non-terminating decimal form", value)
	}

	scale := twoCount
	if fiveCount > scale {
		scale = fiveCount
	}
	scaled := new(big.Int).Set(rat.Num())
	if factor := scale - twoCount; factor > 0 {
		scaled.Mul(scaled, new(big.Int).Exp(five, big.NewInt(int64(factor)), nil))
	}
	if factor := scale - fiveCount; factor > 0 {
		scaled.Mul(scaled, new(big.Int).Exp(two, big.NewInt(int64(factor)), nil))
	}

	negative := scaled.Sign() < 0
	digits := scaled.Abs(scaled).String()
	if scale > 0 {
		if len(digits) <= scale {
			digits = strings.Repeat("0", scale-len(digits)+1) + digits
		}
		position := len(digits) - scale
		digits = digits[:position] + "." + digits[position:]
		digits = strings.TrimRight(strings.TrimRight(digits, "0"), ".")
	}
	if negative {
		digits = "-" + digits
	}
	return json.Number(digits), nil
}
