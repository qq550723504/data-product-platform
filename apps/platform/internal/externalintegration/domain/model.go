package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"regexp"
	"strings"
	"time"
	"unicode"
)

var (
	ErrInvalid  = errors.New("invalid external integration request")
	ErrConflict = errors.New("external integration conflict")
	ErrNotFound = errors.New("external integration object not found")
	ErrState    = errors.New("external integration state does not permit command")
	ErrDenied   = errors.New("external integration caller denied")
)

const Capability = "EXTERNAL_INTEGRATION"

type PublicationStatus string

const (
	Draft      PublicationStatus = "DRAFT"
	Submitting PublicationStatus = "SUBMITTING"
	Registered PublicationStatus = "REGISTERED" // Verified #86 provider observation.
	Published  PublicationStatus = "PUBLISHED"
	Suspended  PublicationStatus = "SUSPENDED"
	Withdrawn  PublicationStatus = "WITHDRAWN"
)

func RequestPublication(status PublicationStatus) (PublicationStatus, error) {
	if status != Draft {
		return status, ErrState
	}
	return Submitting, nil
}

var providerPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)

func ValidProvider(s string) bool { return providerPattern.MatchString(s) }
func ValidText(s string, max int) bool {
	if s == "" || s != strings.TrimSpace(s) || len(s) > max {
		return false
	}
	for _, c := range s {
		if unicode.IsControl(c) {
			return false
		}
	}
	return true
}
func Fingerprint(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

type BindingSpec struct {
	Provider          string    `json:"provider"`
	ExternalSubjectID string    `json:"externalSubjectId"`
	BasisEvidenceID   uuid.UUID `json:"basisEvidenceId"`
}

func (s BindingSpec) Validate() error {
	if !ValidProvider(s.Provider) || !ValidText(s.ExternalSubjectID, 255) || s.BasisEvidenceID == uuid.Nil {
		return ErrInvalid
	}
	return nil
}

type IdentifierSpec struct {
	Provider         string     `json:"provider"`
	ObjectType       string     `json:"objectType"`
	ObjectID         uuid.UUID  `json:"objectId"`
	SubjectBindingID *uuid.UUID `json:"subjectBindingId,omitempty"`
	ExternalID       string     `json:"externalId"`
	BasisEvidenceID  uuid.UUID  `json:"basisEvidenceId"`
}

func (s IdentifierSpec) Validate() error {
	if !ValidProvider(s.Provider) || !ValidText(s.ExternalID, 255) || s.ObjectID == uuid.Nil || s.BasisEvidenceID == uuid.Nil {
		return ErrInvalid
	}
	switch s.ObjectType {
	case "SUBJECT":
		if s.SubjectBindingID == nil || *s.SubjectBindingID == uuid.Nil {
			return ErrInvalid
		}
	case "DATA_RESOURCE", "DATA_PRODUCT", "PRODUCT_RELEASE":
		if s.SubjectBindingID != nil {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

type PublicationSpec struct {
	Provider          string    `json:"provider"`
	IdentityBindingID uuid.UUID `json:"identityBindingId"`
	ProductReleaseID  uuid.UUID `json:"productReleaseId"`
}

func (s PublicationSpec) Validate() error {
	if !ValidProvider(s.Provider) || s.IdentityBindingID == uuid.Nil || s.ProductReleaseID == uuid.Nil {
		return ErrInvalid
	}
	return nil
}

type Binding struct {
	ID          uuid.UUID `json:"id"`
	WorkspaceID uuid.UUID `json:"workspaceId"`
	BindingSpec
	SubjectType  string    `json:"subjectType"`
	SubjectID    uuid.UUID `json:"subjectId"`
	PrincipalRef string    `json:"principalRef"`
	EvidenceID   uuid.UUID `json:"evidenceId"`
	Status       string    `json:"status"`
	CreatedBy    uuid.UUID `json:"createdBy"`
	CreatedAt    time.Time `json:"createdAt"`
}
type Identifier struct {
	ID          uuid.UUID `json:"id"`
	WorkspaceID uuid.UUID `json:"workspaceId"`
	IdentifierSpec
	PrincipalRef string    `json:"principalRef"`
	EvidenceID   uuid.UUID `json:"evidenceId"`
	Status       string    `json:"status"`
	CreatedBy    uuid.UUID `json:"createdBy"`
	CreatedAt    time.Time `json:"createdAt"`
}
type Publication struct {
	ID          uuid.UUID `json:"id"`
	WorkspaceID uuid.UUID `json:"workspaceId"`
	PublicationSpec
	Status       PublicationStatus `json:"status"`
	Revision     int64             `json:"revision"`
	PrincipalRef string            `json:"principalRef"`
	CreatedBy    uuid.UUID         `json:"createdBy"`
	CreatedAt    time.Time         `json:"createdAt"`
	Transitions  []Transition      `json:"transitions"`
}
type Transition struct {
	ProviderObservationID *uuid.UUID        `json:"providerObservationId,omitempty"`
	ID                    uuid.UUID         `json:"id"`
	Revision              int64             `json:"revision"`
	FromStatus            PublicationStatus `json:"fromStatus"`
	ToStatus              PublicationStatus `json:"toStatus"`
	EvidenceID            uuid.UUID         `json:"evidenceId"`
	EventID               uuid.UUID         `json:"eventId"`
	PrincipalRef          string            `json:"principalRef"`
	CreatedBy             uuid.UUID         `json:"createdBy"`
	CreatedAt             time.Time         `json:"createdAt"`
}
type Receipt struct {
	ObjectID   uuid.UUID `json:"objectId"`
	FactID     uuid.UUID `json:"factId"`
	EvidenceID uuid.UUID `json:"evidenceId"`
}
