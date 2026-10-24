package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

// AcceptanceCriteria is a versioned, explicit source format. Generic contract
// prose is never interpreted as a complete set of required acceptance rules.
type AcceptanceCriterion struct {
	ID               string `json:"id"`
	Clause           string `json:"clause"`
	Text             string `json:"text"`
	Required         *bool  `json:"required"`
	Method           string `json:"method"`
	EvidenceCategory string `json:"evidenceCategory"`
}
type AcceptanceSource struct {
	Status        string                `json:"status"`
	SchemaVersion string                `json:"schemaVersion"`
	SetHash       string                `json:"setHash"`
	Criteria      []AcceptanceCriterion `json:"criteria"`
}

var acceptanceCode = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

func ReadAcceptanceSource(document map[string]any) AcceptanceSource {
	result := AcceptanceSource{Status: "UNSUPPORTED_SOURCE", Criteria: []AcceptanceCriterion{}}
	spec, ok := document["spec"].(map[string]any)
	if !ok {
		return result
	}
	raw, ok := spec["acceptanceCriteria"]
	if !ok {
		return result
	}
	result.Status = "INVALID_SOURCE"
	encoded, err := json.Marshal(raw)
	if err != nil {
		return result
	}
	var source struct {
		Version  string                `json:"version"`
		Criteria []AcceptanceCriterion `json:"criteria"`
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&source) != nil || source.Version != "1" || len(source.Criteria) == 0 || len(source.Criteria) > 100 {
		return result
	}
	seen := map[string]bool{}
	required := false
	for _, c := range source.Criteria {
		if !acceptanceCode.MatchString(c.ID) || seen[c.ID] || c.Required == nil || strings.TrimSpace(c.Clause) == "" || len(c.Clause) > 256 || strings.TrimSpace(c.Text) == "" || len(c.Text) > 2000 || !acceptanceCode.MatchString(c.EvidenceCategory) {
			return result
		}
		seen[c.ID] = true
		required = required || *c.Required
		if c.Method != "MANUAL_CHECK" && c.Method != "EVIDENCE_ASSERTION" {
			result.Status = "UNSUPPORTED_CRITERION"
			return result
		}
	}
	if !required {
		return result
	}
	sort.Slice(source.Criteria, func(i, j int) bool { return source.Criteria[i].ID < source.Criteria[j].ID })
	encoded, _ = json.Marshal(source)
	hash := sha256.Sum256(encoded)
	return AcceptanceSource{Status: "SUPPORTED", SchemaVersion: "1", SetHash: hex.EncodeToString(hash[:]), Criteria: source.Criteria}
}
