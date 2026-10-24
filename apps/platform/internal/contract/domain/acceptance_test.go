package domain

import (
	"encoding/json"
	"testing"
)

func acceptanceDocument(raw string) map[string]any {
	var d map[string]any
	_ = json.Unmarshal([]byte(raw), &d)
	return d
}
func TestAcceptanceSourceCompleteExplicitRules(t *testing.T) {
	const valid = `{"spec":{"acceptanceCriteria":{"version":"1","criteria":[{"id":"C1","clause":"3.1","text":"确认成果范围","required":true,"method":"MANUAL_CHECK","evidenceCategory":"RECEIPT"}]}}}`
	got := ReadAcceptanceSource(acceptanceDocument(valid))
	if got.Status != "SUPPORTED" || len(got.SetHash) != 64 || len(got.Criteria) != 1 {
		t.Fatalf("%+v", got)
	}
	for _, tc := range []struct{ raw, status string }{
		{`{"spec":{"quality":"complete"}}`, "UNSUPPORTED_SOURCE"},
		{`{"spec":{"acceptanceCriteria":{"version":"2","criteria":[]}}}`, "INVALID_SOURCE"},
		{`{"spec":{"acceptanceCriteria":{"version":"1","criteria":[{"id":"C1","clause":"3.1","text":"x","method":"MANUAL_CHECK","evidenceCategory":"RECEIPT"}]}}}`, "INVALID_SOURCE"},
		{`{"spec":{"acceptanceCriteria":{"version":"1","criteria":[{"id":"C1","clause":"3.1","text":"x","required":false,"method":"MANUAL_CHECK","evidenceCategory":"RECEIPT"}]}}}`, "INVALID_SOURCE"},
		{`{"spec":{"acceptanceCriteria":{"version":"1","criteria":[{"id":"C1","clause":"3.1","text":"x","required":true,"method":"AUTO_RUN","evidenceCategory":"RECEIPT"}]}}}`, "UNSUPPORTED_CRITERION"},
	} {
		r := ReadAcceptanceSource(acceptanceDocument(tc.raw))
		if r.Status != tc.status || len(r.Criteria) != 0 || r.SetHash != "" {
			t.Fatalf("%s: %+v", tc.status, r)
		}
	}
}
