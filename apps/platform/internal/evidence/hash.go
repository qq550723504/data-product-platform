package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
			Metadata:     nonNilMetadata(record.Metadata),
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
