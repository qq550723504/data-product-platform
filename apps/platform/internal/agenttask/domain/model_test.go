package domain

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func diagnosisPlan() Plan {
	source := uuid.MustParse("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
	return Plan{ID: uuid.New(), TaskID: uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"), WorkspaceID: uuid.MustParse("11111111-1111-4111-8111-111111111111"), Ordinal: 1, Phase: Diagnosis, SourceVersionID: source,
		TemplateRef: "templates/quality.yaml", TemplateVersion: "1", TemplateHash: strings.Repeat("a", 64), DetectionRuleRef: "rules/detect.yaml", DetectionRuleHash: strings.Repeat("b", 64), WorkflowHash: strings.Repeat("c", 64), RecheckRuleRef: "rules/recheck.yaml", RecheckRuleHash: strings.Repeat("d", 64), Goal: "诊断质量问题", PurposeNote: "内部治理", AcceptanceText: "列出发现项与依据", Inputs: []Input{{Name: "source", VersionID: source, Required: true}}, Actions: []Action{{1, "DETECT", Diagnosis}}}
}

func TestFrozenContentIgnoresPresentationIdentityButBindsMeaning(t *testing.T) {
	p := diagnosisPlan()
	if err := p.Freeze(); err != nil {
		t.Fatal(err)
	}
	original := p.ContentHash
	p.ID = uuid.New()
	p.CreatedAt = time.Now()
	if err := p.Freeze(); err != nil {
		t.Fatal(err)
	}
	if p.ContentHash != original {
		t.Fatal("presentation identity changed content")
	}
	changes := map[string]func(*Plan){
		"goal":       func(p *Plan) { p.Goal = "另一项目标" },
		"purpose":    func(p *Plan) { p.PurposeNote = "另一用途" },
		"acceptance": func(p *Plan) { p.AcceptanceText = "另一验收条件" },
		"workspace":  func(p *Plan) { p.WorkspaceID = uuid.New() },
		"task":       func(p *Plan) { p.TaskID = uuid.New() },
		"revision":   func(p *Plan) { p.Ordinal++ },
		"rules":      func(p *Plan) { p.DetectionRuleHash = strings.Repeat("e", 64) },
		"template":   func(p *Plan) { p.TemplateVersion = "2" },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			q := diagnosisPlan()
			change(&q)
			if err := q.Freeze(); err != nil {
				t.Fatal(err)
			}
			if q.ContentHash == original {
				t.Fatal("changed meaning reused hash")
			}
		})
	}
}

func TestCanonicalHashUsesJCS(t *testing.T) {
	hash, canonical, err := Hash(map[string]any{"z": 1, "a": "中文"})
	if err != nil {
		t.Fatal(err)
	}
	if canonical != "{\"a\":\"中文\",\"z\":1}" || len(hash) != 64 {
		t.Fatalf("unexpected canonical: %s %s", canonical, hash)
	}
	other, _, err := Hash(map[string]any{"a": "中文", "z": 1})
	if err != nil || hash != other {
		t.Fatal("map ordering changed hash")
	}
}

func TestDiagnosisCannotSmuggleRepairOrInvalidBindings(t *testing.T) {
	changes := map[string]func(*Plan){
		"repair":         func(p *Plan) { v := uuid.New(); p.CaseID = &v },
		"receipt":        func(p *Plan) { v := uuid.New(); p.Inputs[0].ReceiptID = &v },
		"wrong source":   func(p *Plan) { p.Inputs[0].VersionID = uuid.New() },
		"optional":       func(p *Plan) { p.Inputs[0].Required = false },
		"extra action":   func(p *Plan) { p.Actions = append(p.Actions, Action{2, "RECHECK", Repair}) },
		"ordinal":        func(p *Plan) { p.Actions[0].Ordinal = 2 },
		"uppercase hash": func(p *Plan) { p.TemplateHash = strings.Repeat("A", 64) },
		"blank goal":     func(p *Plan) { p.Goal = " " },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			p := diagnosisPlan()
			change(&p)
			if p.Freeze() == nil {
				t.Fatal("invalid plan accepted")
			}
		})
	}
}

func TestRepairRequiresExactProvenanceAndOrderedActions(t *testing.T) {
	p := diagnosisPlan()
	p.Phase = Repair
	ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	p.CaseID = &ids[0]
	p.AssessmentID = &ids[1]
	p.FindingID = &ids[2]
	p.WorkflowVersionID = &ids[3]
	p.OutputDatasetID = &ids[4]
	p.TargetPeriod = "2026-10"
	p.Inputs[0].ReceiptID = &ids[5]
	p.Inputs = append(p.Inputs, Input{Name: "resolved", VersionID: uuid.New(), Required: true, ResolutionJobID: &ids[6]})
	p.Actions = []Action{{1, "ASSIGN_SELF", Repair}, {2, "EXECUTE_REPAIR", Repair}, {3, "RECHECK", Repair}}
	if err := p.Freeze(); err != nil {
		t.Fatal(err)
	}
	hash := p.ContentHash
	p.Inputs[0], p.Inputs[1] = p.Inputs[1], p.Inputs[0]
	if err := p.Freeze(); err != nil || p.ContentHash != hash {
		t.Fatal("input enumeration changed meaning")
	}
	p.Inputs[0].ReceiptID = &ids[5]
	if !errors.Is(p.Freeze(), ErrBinding) {
		t.Fatal("ambiguous provenance accepted")
	}
	p.Inputs[0].ReceiptID = nil
	nilID := uuid.Nil
	p.WorkflowVersionID = &nilID
	if !errors.Is(p.Freeze(), ErrBinding) {
		t.Fatal("nil reference accepted")
	}
	p.WorkflowVersionID = &ids[3]
	p.Actions[0], p.Actions[1] = p.Actions[1], p.Actions[0]
	if !errors.Is(p.Freeze(), ErrBinding) {
		t.Fatal("out-of-order execution accepted")
	}
}

func TestMutationFence(t *testing.T) {
	for _, tc := range []struct {
		state    string
		expected int64
		want     error
	}{{"ACTIVE", 2, nil}, {"ACTIVE", 1, ErrStale}, {"PAUSED", 2, ErrPaused}, {"STOPPED", 2, ErrStopped}, {"STOPPED", 1, ErrStale}} {
		if got := CanMutate(Task{Revision: 2, ControlState: tc.state}, tc.expected); !errors.Is(got, tc.want) {
			t.Fatalf("%+v got %v", tc, got)
		}
	}
}
