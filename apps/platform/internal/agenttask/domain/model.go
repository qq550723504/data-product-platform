// Package domain owns the platform's finite Agent planning and approval facts.
// It does not own Quality, Execution, engine or model state.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cyberphone/json-canonicalization/go/src/webpki.org/jsoncanonicalizer"
	"github.com/google/uuid"
)

var (
	ErrInvalid     = errors.New("INVALID_AGENT_COMMAND")
	ErrNotFound    = errors.New("AGENT_OBJECT_NOT_FOUND")
	ErrDenied      = errors.New("AGENT_OWNER_DENIED")
	ErrConflict    = errors.New("IDEMPOTENCY_CONFLICT")
	ErrStale       = errors.New("STALE_TASK_REVISION")
	ErrPlanChanged = errors.New("PLAN_BINDING_CHANGED")
	ErrUnresolved  = errors.New("ACTION_UNRESOLVED")
	ErrStopped     = errors.New("TASK_STOPPED")
	ErrPaused      = errors.New("TASK_PAUSED")
	ErrApproval    = errors.New("PLAN_APPROVAL_REQUIRED")
	ErrSlotUsed    = errors.New("ACTION_SLOT_CONSUMED")
	ErrOrder       = errors.New("ACTION_ORDER_BLOCKED")
	ErrBinding     = errors.New("INPUT_BINDING_INCOMPLETE")
)

const Diagnosis = "DIAGNOSIS"
const Repair = "REPAIR"

type Task struct {
	ID                    uuid.UUID  `json:"id"`
	WorkspaceID           uuid.UUID  `json:"workspaceId"`
	Kind                  string     `json:"kind"`
	CreatedBy             uuid.UUID  `json:"createdBy"`
	Subject               string     `json:"subject"`
	ParentTaskID          *uuid.UUID `json:"parentTaskId"`
	CurrentPlanRevisionID uuid.UUID  `json:"currentPlanRevisionId"`
	Revision              int64      `json:"revision"`
	ControlState          string     `json:"controlState"`
	CreatedAt             time.Time  `json:"createdAt"`
}
type Input struct {
	Name            string     `json:"inputName"`
	VersionID       uuid.UUID  `json:"datasetVersionId"`
	Required        bool       `json:"required"`
	ReceiptID       *uuid.UUID `json:"receiptId"`
	ResolutionJobID *uuid.UUID `json:"resolutionJobId"`
}
type Action struct {
	Ordinal int    `json:"ordinal"`
	Command string `json:"commandKind"`
	Phase   string `json:"approvalPhase"`
}
type Plan struct {
	ID                uuid.UUID  `json:"id"`
	TaskID            uuid.UUID  `json:"taskId"`
	WorkspaceID       uuid.UUID  `json:"workspaceId"`
	Ordinal           int        `json:"ordinal"`
	Phase             string     `json:"phase"`
	SourceVersionID   uuid.UUID  `json:"sourceVersionId"`
	AssessmentID      *uuid.UUID `json:"sourceAssessmentId"`
	FindingID         *uuid.UUID `json:"findingId"`
	CaseID            *uuid.UUID `json:"caseId"`
	TemplateRef       string     `json:"templateRef"`
	TemplateVersion   string     `json:"templateVersion"`
	TemplateHash      string     `json:"templateHash"`
	DetectionRuleRef  string     `json:"detectionRuleRef"`
	DetectionRuleHash string     `json:"detectionRuleHash"`
	WorkflowVersionID *uuid.UUID `json:"workflowVersionId"`
	WorkflowHash      string     `json:"workflowHash"`
	OutputDatasetID   *uuid.UUID `json:"outputDatasetId"`
	TargetPeriod      string     `json:"targetPeriod"`
	RecheckRuleRef    string     `json:"recheckRuleRef"`
	RecheckRuleHash   string     `json:"recheckRuleHash"`
	Goal              string     `json:"goal"`
	PurposeNote       string     `json:"purposeNote"`
	AcceptanceText    string     `json:"acceptanceText"`
	Inputs            []Input    `json:"inputs"`
	Actions           []Action   `json:"actions"`
	ContentHash       string     `json:"contentHash"`
	Canonical         string     `json:"-"`
	CreatedAt         time.Time  `json:"createdAt"`
}
type Approval struct {
	ID          uuid.UUID `json:"id"`
	TaskID      uuid.UUID `json:"taskId"`
	PlanID      uuid.UUID `json:"planRevisionId"`
	ContentHash string    `json:"contentHash"`
	Phase       string    `json:"phase"`
	ActorID     uuid.UUID `json:"actorId"`
	Subject     string    `json:"subject"`
	ScopeHash   string    `json:"scopeHash"`
	Reason      string    `json:"reason"`
	CreatedAt   time.Time `json:"createdAt"`
}
type Intent struct {
	ID          uuid.UUID `json:"id"`
	TaskID      uuid.UUID `json:"taskId"`
	PlanID      uuid.UUID `json:"planRevisionId"`
	ApprovalID  uuid.UUID `json:"approvalId"`
	Ordinal     int       `json:"ordinal"`
	Command     string    `json:"commandKind"`
	BusinessKey string    `json:"businessRequestKey"`
	State       string    `json:"dispatchState"`
	Reason      string    `json:"reason"`
	CreatedAt   time.Time `json:"createdAt"`
}
type Receipt struct {
	ID           uuid.UUID  `json:"id"`
	TaskID       uuid.UUID  `json:"taskId"`
	PlanID       uuid.UUID  `json:"planRevisionId"`
	ApprovalID   *uuid.UUID `json:"approvalId,omitempty"`
	ActionID     *uuid.UUID `json:"actionId,omitempty"`
	TaskRevision int64      `json:"taskRevision"`
}
type Page struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}
type PlanSummary struct {
	ID      uuid.UUID `json:"id"`
	Ordinal int       `json:"ordinal"`
	Phase   string    `json:"phase"`
	Hash    string    `json:"contentHash"`
}
type Detail struct {
	Task            Task          `json:"task"`
	Plan            Plan          `json:"plan"`
	Plans           []PlanSummary `json:"plans"`
	PlanPage        Page          `json:"planPage"`
	Approvals       []Approval    `json:"approvals"`
	Actions         []Intent      `json:"actions"`
	DispatchEnabled bool          `json:"dispatchEnabled"`
}

func Text(value string, min, max int) bool {
	return utf8.ValidString(value) && value == strings.TrimSpace(value) && utf8.RuneCountInString(value) >= min && utf8.RuneCountInString(value) <= max && !strings.ContainsRune(value, 0)
}
func Hash(value any) (string, string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", "", err
	}
	canonical, err := jsoncanonicalizer.Transform(raw)
	if err != nil {
		return "", "", err
	}
	hash := sha256.Sum256(canonical)
	return hex.EncodeToString(hash[:]), string(canonical), nil
}

// Content is the versioned canonical envelope, shared with the database freeze
// check. Time, UI state and credentials are deliberately absent.
func (p Plan) Content() map[string]any {
	inputs := append([]Input{}, p.Inputs...)
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].Name < inputs[j].Name })
	actions := append([]Action{}, p.Actions...)
	sort.Slice(actions, func(i, j int) bool { return actions[i].Ordinal < actions[j].Ordinal })
	return map[string]any{"schemaVersion": 1, "workspaceId": p.WorkspaceID, "taskId": p.TaskID, "ordinal": p.Ordinal, "phase": p.Phase,
		"sourceVersionId": p.SourceVersionID, "sourceAssessmentId": p.AssessmentID, "findingId": p.FindingID, "caseId": p.CaseID,
		"templateRef": p.TemplateRef, "templateVersion": p.TemplateVersion, "templateHash": p.TemplateHash,
		"detectionRuleRef": p.DetectionRuleRef, "detectionRuleHash": p.DetectionRuleHash, "workflowVersionId": p.WorkflowVersionID, "workflowHash": p.WorkflowHash,
		"outputDatasetId": p.OutputDatasetID, "targetPeriod": p.TargetPeriod, "recheckRuleRef": p.RecheckRuleRef, "recheckRuleHash": p.RecheckRuleHash,
		"goal": p.Goal, "purposeNote": p.PurposeNote, "acceptanceText": p.AcceptanceText, "inputs": inputs, "actions": actions}
}
func (p *Plan) Freeze() error {
	if p.TaskID == uuid.Nil || p.WorkspaceID == uuid.Nil || p.SourceVersionID == uuid.Nil || p.Ordinal < 1 || !Text(p.Goal, 1, 300) || !Text(p.PurposeNote, 0, 1000) || !Text(p.AcceptanceText, 1, 2000) {
		return ErrInvalid
	}
	for _, h := range []string{p.TemplateHash, p.DetectionRuleHash, p.WorkflowHash, p.RecheckRuleHash} {
		if len(h) != 64 {
			return ErrBinding
		}
		if _, err := hex.DecodeString(h); err != nil {
			return ErrBinding
		}
		if h != strings.ToLower(h) {
			return ErrBinding
		}
	}
	if p.TemplateRef == "" || p.TemplateVersion == "" || p.DetectionRuleRef == "" || p.RecheckRuleRef == "" {
		return ErrBinding
	}
	names := map[string]bool{}
	for _, i := range p.Inputs {
		if !Text(i.Name, 1, 128) || names[i.Name] || i.VersionID == uuid.Nil || !i.Required {
			return ErrBinding
		}
		names[i.Name] = true
		if (i.ReceiptID != nil && *i.ReceiptID == uuid.Nil) || (i.ResolutionJobID != nil && *i.ResolutionJobID == uuid.Nil) {
			return ErrBinding
		}
	}
	switch p.Phase {
	case Diagnosis:
		if len(p.Inputs) != 1 || p.Inputs[0].VersionID != p.SourceVersionID || p.Inputs[0].ReceiptID != nil || p.Inputs[0].ResolutionJobID != nil || p.CaseID != nil || p.AssessmentID != nil || p.FindingID != nil || p.WorkflowVersionID != nil || p.OutputDatasetID != nil || p.TargetPeriod != "" || len(p.Actions) != 1 || p.Actions[0] != (Action{1, "DETECT", Diagnosis}) {
			return ErrBinding
		}
	case Repair:
		if p.CaseID == nil || p.AssessmentID == nil || p.FindingID == nil || p.WorkflowVersionID == nil || p.OutputDatasetID == nil || !Text(p.TargetPeriod, 1, 64) || len(p.Inputs) == 0 || len(p.Actions) != 3 {
			return ErrBinding
		}
		for _, id := range []*uuid.UUID{p.CaseID, p.AssessmentID, p.FindingID, p.WorkflowVersionID, p.OutputDatasetID} {
			if *id == uuid.Nil {
				return ErrBinding
			}
		}
		for _, i := range p.Inputs {
			if (i.ReceiptID == nil) == (i.ResolutionJobID == nil) {
				return ErrBinding
			}
		}
		for i, c := range []string{"ASSIGN_SELF", "EXECUTE_REPAIR", "RECHECK"} {
			if p.Actions[i] != (Action{i + 1, c, Repair}) {
				return ErrBinding
			}
		}
	default:
		return ErrInvalid
	}
	var err error
	p.ContentHash, p.Canonical, err = Hash(p.Content())
	return err
}

func CanMutate(t Task, expected int64) error {
	if t.Revision != expected {
		return ErrStale
	}
	if t.ControlState == "STOPPED" {
		return ErrStopped
	}
	if t.ControlState == "PAUSED" {
		return ErrPaused
	}
	return nil
}
