package application

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	goldinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/gold/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/tabular"
)

func TestBuildGoldCSVPreservesSourceOrderAndSkipsRejectedTask(t *testing.T) {
	resultID := uuid.New()
	decisionID := uuid.New()
	table := tabular.Table{
		Headers: []string{"company_id", "activity_level"},
		Rows: []map[string]string{
			{"company_id": "c1", "activity_level": "HIGH"},
			{"company_id": "c2", "activity_level": "LOW"},
		},
	}
	payload := []byte(`{"label":"SUFFICIENT_INPUT"}`)
	frozen := []goldinfra.FrozenMember{
		{
			TaskID: uuid.New(), SourceItemRef: "row:2", SourceContentSHA256: mustGoldSourceHash(t, table.Rows[1]),
			DecisionID: uuid.New(), Outcome: "REJECT",
		},
		{
			TaskID: uuid.New(), SourceItemRef: "row:1", SourceContentSHA256: mustGoldSourceHash(t, table.Rows[0]),
			DecisionID: decisionID, Outcome: "ACCEPT", SelectedResultID: &resultID,
			SelectedResultSHA256: hashBytes(payload), SelectedPayload: payload,
		},
	}

	content, members, outputRows, err := buildGoldCSV(table, frozen)
	if err != nil {
		t.Fatalf("buildGoldCSV: %v", err)
	}
	if outputRows != 1 {
		t.Fatalf("outputRows=%d want=1", outputRows)
	}
	got := string(content)
	want := "company_id,activity_level,gold_label,annotation_result_id,review_decision_id\n" +
		"c1,HIGH,SUFFICIENT_INPUT," + resultID.String() + "," + decisionID.String() + "\n"
	if got != want {
		t.Fatalf("content:\n%s\nwant:\n%s", got, want)
	}
	if len(members) != 2 {
		t.Fatalf("production members=%d want=2", len(members))
	}
	if members[0].SourceItemRef != "row:1" || members[0].OutputRowIndex == nil || *members[0].OutputRowIndex != 0 {
		t.Fatalf("accepted member=%+v", members[0])
	}
	if members[1].SourceItemRef != "row:2" || members[1].OutputRowIndex != nil || members[1].SelectedResultID != nil {
		t.Fatalf("rejected member=%+v", members[1])
	}
}

func TestBuildGoldCSVRejectsMissingFrozenTask(t *testing.T) {
	table := tabular.Table{
		Headers: []string{"id"},
		Rows: []map[string]string{{"id": "1"}, {"id": "2"}},
	}
	_, _, _, err := buildGoldCSV(table, []goldinfra.FrozenMember{{
		TaskID: uuid.New(), SourceItemRef: "row:1", SourceContentSHA256: mustGoldSourceHash(t, table.Rows[0]),
		DecisionID: uuid.New(), Outcome: "REJECT",
	}})
	if err == nil || !strings.Contains(err.Error(), "row:2") {
		t.Fatalf("error=%v want missing row:2", err)
	}
}


func mustGoldSourceHash(t *testing.T, row map[string]string) string {
	t.Helper()
	value, err := goldSourceRowHash(row)
	if err != nil {
		t.Fatalf("source hash: %v", err)
	}
	return value
}
