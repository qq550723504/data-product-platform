package domain

import (
	"errors"
	"github.com/google/uuid"
	"strings"
)

var (
	ErrScope    = errors.New("remediation scope does not match")
	ErrConflict = errors.New("remediation idempotency conflict")
	ErrClosed   = errors.New("remediation case is manually closed")
	ErrAssignee = errors.New("remediation requires the assigned operator")
	ErrAttempt  = errors.New("remediation requires an explicit retry of the latest failed execution")
	ErrEvidence = errors.New("remediation requires exact failed finding or successful recheck evidence")
)

type Case struct {
	ID               uuid.UUID `json:"id"`
	WorkspaceID      uuid.UUID `json:"workspaceId"`
	VersionID        uuid.UUID `json:"versionId"`
	AssessmentID     uuid.UUID `json:"assessmentId"`
	FindingID        uuid.UUID `json:"findingId"`
	PlanRef          string    `json:"planRef"`
	PlanVersion      string    `json:"planVersion"`
	PlanHash         string    `json:"planHash"`
	DetectionRuleID  string    `json:"detectionRuleId"`
	WorkflowHash     string    `json:"workflowHash"`
	InputName        string    `json:"inputName"`
	QuarantineReason string    `json:"quarantineReason"`
	RecheckRuleRef   string    `json:"recheckRuleRef"`
	RecheckRuleHash  string    `json:"recheckRuleHash"`
	CreatedBy        uuid.UUID `json:"createdBy"`
}

func ValidateCommand(workspace, actor uuid.UUID, key string) error {
	if workspace == uuid.Nil || actor == uuid.Nil {
		return ErrScope
	}
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 128 {
		return ErrConflict
	}
	return nil
}

func ValidateReason(reason string) error {
	n := len([]rune(strings.TrimSpace(reason)))
	if n < 1 || n > 1000 {
		return errors.New("remediation reason must contain 1..1000 characters")
	}
	return nil
}

// Case closure is only a business handling fact. It never changes a quality or
// certification decision, and never creates delivery authority.
func CanHandle(closed bool, assignee, actor uuid.UUID) error {
	if closed {
		return ErrClosed
	}
	if actor == uuid.Nil || assignee != actor {
		return ErrAssignee
	}
	return nil
}
