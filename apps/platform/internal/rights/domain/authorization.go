package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type AuthorizationStatus string

const (
	StatusDraft     AuthorizationStatus = "DRAFT"
	StatusReviewing AuthorizationStatus = "REVIEWING"
	StatusApproved  AuthorizationStatus = "APPROVED"
	StatusActive    AuthorizationStatus = "ACTIVE"
	StatusSuspended AuthorizationStatus = "SUSPENDED"
	StatusRevoked   AuthorizationStatus = "REVOKED"
	StatusExpired   AuthorizationStatus = "EXPIRED"
	StatusRejected  AuthorizationStatus = "REJECTED"
)

var (
	ErrInvalidAuthorization = errors.New("authorization is invalid")
	ErrInvalidTransition    = errors.New("authorization state transition is invalid")
	ErrAuthorizationInvalid = errors.New("authorization is not valid for the requested snapshot")
	// ErrResourceWorkspace rejects a grant whose DataResource belongs to another
	// workspace. A resource foreign key proves the row exists, not who owns it.
	ErrResourceWorkspace = errors.New("authorization resource belongs to a different workspace")
)

type ResourceGrant struct {
	ID               uuid.UUID
	AuthorizationID  uuid.UUID
	DataResourceID   uuid.UUID
	Actions          []string
	Scope            map[string]any
	RawExportAllowed bool
	CreatedAt        time.Time
}

type Authorization struct {
	ID          uuid.UUID
	WorkspaceID uuid.UUID
	Code        string
	GrantorRef  string
	GranteeRef  string
	Purpose     string
	Status      AuthorizationStatus
	ValidFrom   *time.Time
	ValidTo     *time.Time
	Metadata    map[string]any
	Resources   []ResourceGrant
	CreatedAt   time.Time
	CreatedBy   *uuid.UUID
	UpdatedAt   time.Time
	UpdatedBy   *uuid.UUID
}

type ResourceGrantSpec struct {
	DataResourceID   uuid.UUID
	Actions          []string
	Scope            map[string]any
	RawExportAllowed bool
}

func NewAuthorization(workspaceID uuid.UUID, code, grantorRef, granteeRef, purpose string, validFrom, validTo *time.Time, metadata map[string]any, resources []ResourceGrantSpec, actorID *uuid.UUID) (Authorization, error) {
	code = strings.TrimSpace(code)
	grantorRef = strings.TrimSpace(grantorRef)
	granteeRef = strings.TrimSpace(granteeRef)
	purpose = strings.TrimSpace(purpose)
	if workspaceID == uuid.Nil || code == "" || grantorRef == "" || granteeRef == "" || purpose == "" || len(resources) == 0 {
		return Authorization{}, ErrInvalidAuthorization
	}
	if validFrom != nil && validTo != nil && !validTo.After(*validFrom) {
		return Authorization{}, ErrInvalidAuthorization
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	now := time.Now().UTC()
	authorization := Authorization{
		ID:          uuid.New(),
		WorkspaceID: workspaceID,
		Code:        code,
		GrantorRef:  grantorRef,
		GranteeRef:  granteeRef,
		Purpose:     purpose,
		Status:      StatusDraft,
		ValidFrom:   validFrom,
		ValidTo:     validTo,
		Metadata:    metadata,
		CreatedAt:   now,
		CreatedBy:   actorID,
		UpdatedAt:   now,
		UpdatedBy:   actorID,
	}
	for _, spec := range resources {
		if spec.DataResourceID == uuid.Nil || len(spec.Actions) == 0 {
			return Authorization{}, ErrInvalidAuthorization
		}
		actions := make([]string, 0, len(spec.Actions))
		for _, action := range spec.Actions {
			action = strings.ToUpper(strings.TrimSpace(action))
			if action == "" {
				return Authorization{}, ErrInvalidAuthorization
			}
			actions = append(actions, action)
		}
		if spec.Scope == nil {
			spec.Scope = map[string]any{}
		}
		authorization.Resources = append(authorization.Resources, ResourceGrant{
			ID:               uuid.New(),
			AuthorizationID:  authorization.ID,
			DataResourceID:   spec.DataResourceID,
			Actions:          actions,
			Scope:            spec.Scope,
			RawExportAllowed: spec.RawExportAllowed,
			CreatedAt:        now,
		})
	}
	return authorization, nil
}

func (a *Authorization) Submit(actorID *uuid.UUID) error {
	return a.transition(StatusDraft, StatusReviewing, actorID)
}

func (a *Authorization) Approve(actorID *uuid.UUID) error {
	return a.transition(StatusReviewing, StatusApproved, actorID)
}

func (a *Authorization) Activate(at time.Time, actorID *uuid.UUID) error {
	if a.Status != StatusApproved {
		return ErrInvalidTransition
	}
	if a.ValidFrom != nil && at.Before(*a.ValidFrom) {
		return ErrAuthorizationInvalid
	}
	if a.ValidTo != nil && !at.Before(*a.ValidTo) {
		return ErrAuthorizationInvalid
	}
	a.Status = StatusActive
	a.UpdatedAt = time.Now().UTC()
	a.UpdatedBy = actorID
	return nil
}

func (a *Authorization) Suspend(actorID *uuid.UUID) error {
	return a.transition(StatusActive, StatusSuspended, actorID)
}

func (a *Authorization) Revoke(actorID *uuid.UUID) error {
	if a.Status != StatusActive && a.Status != StatusSuspended && a.Status != StatusApproved {
		return ErrInvalidTransition
	}
	a.Status = StatusRevoked
	a.UpdatedAt = time.Now().UTC()
	a.UpdatedBy = actorID
	return nil
}

func (a *Authorization) Expire(at time.Time) error {
	if a.ValidTo == nil || at.Before(*a.ValidTo) {
		return ErrInvalidTransition
	}
	if a.Status != StatusActive && a.Status != StatusSuspended {
		return ErrInvalidTransition
	}
	a.Status = StatusExpired
	a.UpdatedAt = time.Now().UTC()
	return nil
}

func (a Authorization) ValidFor(asOf time.Time, purpose string) bool {
	if a.Status != StatusActive || a.Purpose != strings.TrimSpace(purpose) {
		return false
	}
	if a.ValidFrom != nil && asOf.Before(*a.ValidFrom) {
		return false
	}
	if a.ValidTo != nil && !asOf.Before(*a.ValidTo) {
		return false
	}
	return true
}

func (a *Authorization) transition(from, to AuthorizationStatus, actorID *uuid.UUID) error {
	if a.Status != from {
		return ErrInvalidTransition
	}
	a.Status = to
	a.UpdatedAt = time.Now().UTC()
	a.UpdatedBy = actorID
	return nil
}
