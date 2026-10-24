package readmodel

import (
	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"sort"
	"time"
)

type Admission struct {
	ProfileVersion    string           `json:"profileVersion"`
	PrincipalID       uuid.UUID        `json:"principalId"`
	CanonicalSubject  string           `json:"canonicalSubject"`
	WorkspaceID       uuid.UUID        `json:"workspaceId"`
	AuthorityRevision string           `json:"authorityRevision"`
	EvaluatedAt       time.Time        `json:"evaluatedAt"`
	ExpiresAt         time.Time        `json:"expiresAt"`
	Capabilities      []string         `json:"capabilities"`
	Actions           []ActionPrecheck `json:"actions"`
	WriteEnabled      bool             `json:"writeEnabled"`
}
type ActionPrecheck struct {
	Command                string   `json:"command"`
	Role                   string   `json:"role"`
	Capability             string   `json:"capability"`
	ResponsibilityEligible bool     `json:"responsibilityEligible"`
	Blockers               []string `json:"blockers"`
}

func admissionView(a principal.Authority, c *Context) *Admission {
	p := a.Principal
	view := &Admission{ProfileVersion: "business-acceptance.v1", PrincipalID: p.ActorID, CanonicalSubject: p.Subject, WorkspaceID: p.WorkspaceID, AuthorityRevision: p.IdentityRevision, EvaluatedAt: a.EvaluatedAt, ExpiresAt: a.ExpiresAt, Capabilities: []string{}, Actions: []ActionPrecheck{}}
	for _, cap := range []string{principal.CapabilityAcceptanceRead, principal.CapabilityAcceptanceDraft, principal.CapabilityAcceptanceBasis, principal.CapabilityReceiptRecord} {
		if p.HasCapability(cap) {
			view.Capabilities = append(view.Capabilities, cap)
		}
	}
	sort.Strings(view.Capabilities)
	for _, item := range []struct{ command, role, cap string }{{"CreateBusinessAcceptance", "PROVIDER", principal.CapabilityAcceptanceDraft}, {"ReviseAcceptanceDraft", "PROVIDER", principal.CapabilityAcceptanceDraft}, {"ConfirmAcceptanceBasis", "BASIS_CONFIRMER", principal.CapabilityAcceptanceBasis}, {"RecordBusinessReceipt", "RECEIVER", principal.CapabilityReceiptRecord}} {
		action := ActionPrecheck{Command: item.command, Role: item.role, Capability: item.cap, Blockers: []string{"ACCEPTANCE_WRITES_DISABLED"}}
		for _, r := range c.Responsibilities {
			if r.Role == item.role && r.Status == "ASSIGNED" && r.IsCurrentCaller {
				action.ResponsibilityEligible = true
			}
		}
		if !action.ResponsibilityEligible {
			action.Blockers = append(action.Blockers, "CURRENT_TARGET_RESPONSIBILITY_REQUIRED")
		}
		if !p.HasCapability(item.cap) {
			action.Blockers = append(action.Blockers, "ACTION_CAPABILITY_REQUIRED")
		}
		if c.Delivery.Status != "ISSUED" {
			action.Blockers = append(action.Blockers, "DELIVERY_NOT_ISSUED")
		}
		if c.Applicability.Status != "MATCHED" {
			action.Blockers = append(action.Blockers, "STANDARD_APPLICABILITY_REQUIRED")
		}
		for _, r := range c.Responsibilities {
			if r.Representative != nil && r.Representative.Origin == "SYNTHETIC_FIXTURE" {
				action.Blockers = append(action.Blockers, "SYNTHETIC_SOURCE_NOT_BUSINESS_AUTHORITY")
				break
			}
		}
		if item.command != "CreateBusinessAcceptance" {
			action.Blockers = append(action.Blockers, "CASE_COMMAND_NOT_IMPLEMENTED")
		}
		view.Actions = append(view.Actions, action)
	}
	return view
}
