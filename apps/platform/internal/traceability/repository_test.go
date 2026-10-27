package traceability

import (
	"bytes"
	"encoding/json"
	"github.com/google/uuid"
	"testing"
)

func TestMappingMergePreservesEmptyJSONCollectionAndDecisionIdentity(t *testing.T) {
	rows := appendUniqueEntityMappings(nil, nil)
	data, err := json.Marshal(rows)
	if err != nil || !bytes.Equal(data, []byte("[]")) {
		t.Fatalf("empty mapping history must encode as array: %s", data)
	}
	mapping := uuid.New()
	a := EntityMappingTrace{ID: mapping, DecisionID: uuid.New()}
	b := EntityMappingTrace{ID: mapping, DecisionID: uuid.New()}
	rows = appendUniqueEntityMappings([]EntityMappingTrace{a}, []EntityMappingTrace{a, b})
	if len(rows) != 2 || rows[0].DecisionID != a.DecisionID || rows[1].DecisionID != b.DecisionID {
		t.Fatal("immutable decision membership changed")
	}
}
