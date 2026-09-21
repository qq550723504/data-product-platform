package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrInvalidRightsSnapshot = errors.New("rights snapshot is invalid")

type SnapshotAuthorization struct {
	AuthorizationID uuid.UUID       `json:"authorizationId"`
	Code            string          `json:"code"`
	GrantorRef      string          `json:"grantorRef"`
	GranteeRef      string          `json:"granteeRef"`
	Purpose         string          `json:"purpose"`
	ValidFrom       *time.Time      `json:"validFrom,omitempty"`
	ValidTo         *time.Time      `json:"validTo,omitempty"`
	Resources       []ResourceGrant `json:"resources"`
	DeclarationIDs  []uuid.UUID     `json:"declarationIds,omitempty"`
	BindingIDs      []uuid.UUID     `json:"bindingIds,omitempty"`
}

type RightsManifest struct {
	Purpose        string                  `json:"purpose"`
	ConsumerRef    string                  `json:"consumerRef,omitempty"`
	AsOf           time.Time               `json:"asOf"`
	Authorizations []SnapshotAuthorization `json:"authorizations"`
}

type RightsSnapshot struct {
	ID               uuid.UUID
	WorkspaceID      uuid.UUID
	ProductReleaseID *uuid.UUID
	Purpose          string
	ConsumerRef      string
	AsOf             time.Time
	Manifest         RightsManifest
	RootHash         string
	CreatedAt        time.Time
	CreatedBy        *uuid.UUID
	DeclarationIDs   []uuid.UUID
	BindingIDs       []uuid.UUID
}

func NewRightsSnapshot(workspaceID uuid.UUID, productReleaseID *uuid.UUID, purpose, consumerRef string, asOf time.Time, authorizations []Authorization, actorID *uuid.UUID) (RightsSnapshot, error) {
	purpose = strings.TrimSpace(purpose)
	consumerRef = strings.TrimSpace(consumerRef)
	if workspaceID == uuid.Nil || purpose == "" || asOf.IsZero() || len(authorizations) == 0 {
		return RightsSnapshot{}, ErrInvalidRightsSnapshot
	}

	manifest := RightsManifest{
		Purpose:     purpose,
		ConsumerRef: consumerRef,
		AsOf:        asOf.UTC(),
	}
	for _, authorization := range authorizations {
		if authorization.WorkspaceID != workspaceID || !authorization.ValidFor(asOf, purpose) {
			return RightsSnapshot{}, ErrAuthorizationInvalid
		}
		resources := make([]ResourceGrant, len(authorization.Resources))
		copy(resources, authorization.Resources)
		manifest.Authorizations = append(manifest.Authorizations, SnapshotAuthorization{
			AuthorizationID: authorization.ID,
			Code:            authorization.Code,
			GrantorRef:      authorization.GrantorRef,
			GranteeRef:      authorization.GranteeRef,
			Purpose:         authorization.Purpose,
			ValidFrom:       authorization.ValidFrom,
			ValidTo:         authorization.ValidTo,
			Resources:       resources,
		})
	}

	encoded, err := json.Marshal(manifest)
	if err != nil {
		return RightsSnapshot{}, err
	}
	digest := sha256.Sum256(encoded)
	now := time.Now().UTC()
	return RightsSnapshot{
		ID:               uuid.New(),
		WorkspaceID:      workspaceID,
		ProductReleaseID: productReleaseID,
		Purpose:          purpose,
		ConsumerRef:      consumerRef,
		AsOf:             asOf.UTC(),
		Manifest:         manifest,
		RootHash:         hex.EncodeToString(digest[:]),
		CreatedAt:        now,
		CreatedBy:        actorID,
	}, nil
}
