package pilot

import (
	_ "embed"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
)

// This is a read-only checkpoint of the current append-only policy successors,
// captured after v22. The expired policies grant no new use or restore rights.
//
//go:embed remaining_policy_heads.json
var remainingPolicyCheckpoint []byte

const RemainingPolicyCheckpointSHA256 = "aab0e61530bee63429a15ce635880a6d1db286cb744767f7d9fdb50082313df8"

func RemainingPolicyHeads(group string) map[string]uuid.UUID {
	result := map[string]uuid.UUID{}
	if group != "business-races" {
		return result
	}
	var checkpoint struct {
		Heads map[string]uuid.UUID `json:"heads"`
	}
	if json.Unmarshal(remainingPolicyCheckpoint, &checkpoint) != nil {
		return result
	}
	for version, head := range checkpoint.Heads {
		result[version] = head
	}
	return result
}

// Exact identities are shared by generation, offline validation and the actual
// Core query. Matching cardinality alone cannot accept an old predecessor.
func ValidateRemainingPolicyHeads(group string, actual map[string]uuid.UUID) error {
	expected := RemainingPolicyHeads(group)
	if len(expected) != len(actual) || (group == "business-races" && len(expected) != 5) {
		return errors.New("current policy checkpoint membership differs")
	}
	for version, head := range expected {
		if head == uuid.Nil || actual[version] != head {
			return errors.New("current policy checkpoint identity differs")
		}
	}
	return nil
}
