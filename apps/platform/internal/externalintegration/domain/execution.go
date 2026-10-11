package domain

import (
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"time"
)

var ErrUnavailable = errors.New("external provider unavailable")
var ErrUnsupported = errors.New("external capability unsupported")

// Request is metadata only. It cannot contain data, storage addresses or credentials.
type Request struct {
	WorkspaceID        uuid.UUID `json:"workspaceId"`
	PublicationID      uuid.UUID `json:"publicationId"`
	ReleaseID          uuid.UUID `json:"releaseId"`
	BindingID          uuid.UUID `json:"bindingId"`
	IdentifierID       uuid.UUID `json:"identifierId"`
	Provider           string    `json:"provider"`
	Instance           string    `json:"instance"`
	AdapterRevision    string    `json:"adapterRevision"`
	ExternalSubjectID  string    `json:"externalSubjectId"`
	ExternalIdentifier string    `json:"externalIdentifier"`
	RequestKey         string    `json:"requestKey"`
}

func (r Request) Bytes() ([]byte, error) { return json.Marshal(r) }
func (r Request) Hash() string           { h, _ := Fingerprint(r); return h }

type ProviderResult struct {
	Instance    string `json:"instance"`
	RequestKey  string `json:"requestKey"`
	RequestHash string `json:"requestHash"`
	State       string `json:"state"`
	ExternalID  string `json:"externalId"`
	RemoteID    string `json:"remoteId"`
}

func (r ProviderResult) Valid(q Request, kind string) bool {
	if r.Instance != q.Instance || r.RequestKey != q.RequestKey || r.RequestHash != q.Hash() {
		return false
	}
	if r.State == "FAILED" || r.State == "UNKNOWN" || (r.State == "ABSENT" && kind == "POLL") {
		return r.ExternalID == "" && r.RemoteID == ""
	}
	if kind == "IDENTITY" {
		return r.State == "VERIFIED" && r.ExternalID == q.ExternalSubjectID && r.RemoteID == ""
	}
	if kind == "IDENTIFIER" {
		return r.State == "VERIFIED" && r.ExternalID == q.ExternalIdentifier && r.RemoteID == ""
	}
	if kind != "SUBMIT" && kind != "POLL" {
		return false
	}
	if r.State == "REGISTERED" || r.State == "PUBLISHED" {
		return r.ExternalID == q.ExternalIdentifier && ValidText(r.RemoteID, 255)
	}
	return false
}

type Attempt struct {
	ID           uuid.UUID `json:"id"`
	Number       int64     `json:"number"`
	Request      Request   `json:"request"`
	PrincipalRef string    `json:"principalRef"`
	ActorID      uuid.UUID `json:"actorId"`
	EvidenceID   uuid.UUID `json:"evidenceId"`
	CreatedAt    time.Time `json:"createdAt"`
	Resolution   string    `json:"resolution"`
}
type Call struct {
	ID            uuid.UUID      `json:"id"`
	AttemptID     uuid.UUID      `json:"attemptId"`
	Kind          string         `json:"kind"`
	CreatedAt     time.Time      `json:"createdAt"`
	Dispatched    bool           `json:"dispatched"`
	ObservationID *uuid.UUID     `json:"observationId,omitempty"`
	Result        ProviderResult `json:"result"`
	Code          string         `json:"code"`
}

func AdvancePublication(from PublicationStatus, target string) ([]PublicationStatus, error) {
	if target == "REGISTERED" {
		if from == Submitting {
			return []PublicationStatus{Registered}, nil
		}
		if from == Registered || from == Published {
			return nil, nil
		}
	}
	if target == "PUBLISHED" {
		if from == Submitting {
			return []PublicationStatus{Registered, Published}, nil
		}
		if from == Registered {
			return []PublicationStatus{Published}, nil
		}
		if from == Published {
			return nil, nil
		}
	}
	return nil, ErrState
}
