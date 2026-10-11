package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/google/uuid"
	"regexp"
	"strings"
	"time"
)

const UsagePageLimit = 50

var streamPattern = regexp.MustCompile(`^urn:[A-Za-z0-9][A-Za-z0-9._:-]{0,247}$`)
var cursorPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{0,512}$`)

func ValidStream(s string) bool { return streamPattern.MatchString(s) }
func ValidCursor(s string) bool { return cursorPattern.MatchString(s) }

type UsageSourceSpec struct {
	BindingID uuid.UUID `json:"bindingId"`
	Stream    string    `json:"stream"`
}
type UsageSource struct {
	ID                uuid.UUID `json:"id"`
	WorkspaceID       uuid.UUID `json:"workspaceId"`
	BindingID         uuid.UUID `json:"bindingId"`
	Provider          string    `json:"provider"`
	Instance          string    `json:"instance"`
	AdapterRevision   string    `json:"adapterRevision"`
	Stream            string    `json:"stream"`
	ExternalSubjectID string    `json:"externalSubjectId"`
	ActorID           uuid.UUID `json:"actorId"`
	PrincipalRef      string    `json:"principalRef"`
	EvidenceID        uuid.UUID `json:"evidenceId"`
	Revision          int64     `json:"revision"`
	Cursor            string    `json:"cursor"`
}
type UsageReadRequest struct {
	Source   UsageSource `json:"source"`
	Revision int64       `json:"revision"`
	Cursor   string      `json:"cursor"`
	Limit    int         `json:"limit"`
}

func (q UsageReadRequest) Hash() string { h, _ := Fingerprint(q); return h }

type UsageOccurrence struct {
	ID                 string    `json:"id"`
	Source             string    `json:"source"`
	Category           string    `json:"category"`
	OccurredAt         time.Time `json:"occurredAt"`
	ExternalSubjectID  string    `json:"externalSubjectId"`
	ExternalIdentifier string    `json:"externalIdentifier"`
	RemoteID           string    `json:"remoteId"`
}

func (v UsageOccurrence) Normalize() UsageOccurrence {
	v.OccurredAt = v.OccurredAt.UTC().Truncate(time.Microsecond)
	return v
}
func (v UsageOccurrence) Hash() string {
	v = v.Normalize()
	// Every string is bounded and excludes control characters; newlines are unambiguous.
	h := sha256.Sum256([]byte(strings.Join([]string{v.ID, v.Source, v.Category, v.OccurredAt.Format("2006-01-02T15:04:05.000000Z"), v.ExternalSubjectID, v.ExternalIdentifier, v.RemoteID}, "\n")))
	return hex.EncodeToString(h[:])
}
func (v UsageOccurrence) Valid(stream string) bool {
	if !ValidText(v.ID, 255) || v.Source != stream || !ValidStream(v.Source) || !ValidText(v.Category, 64) || v.OccurredAt.IsZero() || v.OccurredAt.Year() < 1 || v.OccurredAt.Year() > 9999 {
		return false
	}
	for _, x := range []string{v.ExternalSubjectID, v.ExternalIdentifier, v.RemoteID} {
		if x != "" && !ValidText(x, 255) {
			return false
		}
	}
	return true
}
func (v UsageOccurrence) KnownCategory() bool {
	switch v.Category {
	case "USAGE", "ACCESS", "DELIVERY", "CONNECTOR_LOG":
		return true
	}
	return false
}

type UsagePage struct {
	Instance    string            `json:"instance"`
	RequestHash string            `json:"requestHash"`
	FromCursor  string            `json:"fromCursor"`
	NextCursor  string            `json:"nextCursor"`
	Events      []UsageOccurrence `json:"events"`
}

func (p UsagePage) Normalize(q UsageReadRequest) (UsagePage, error) {
	if p.Instance != q.Source.Instance || p.RequestHash != q.Hash() || p.FromCursor != q.Cursor || !ValidCursor(p.NextCursor) || len(p.Events) > UsagePageLimit || p.Events == nil || (len(p.Events) > 0 && p.NextCursor == q.Cursor) {
		return UsagePage{}, ErrInvalid
	}
	p.Events = append([]UsageOccurrence{}, p.Events...)
	for n, v := range p.Events {
		if !v.Valid(q.Source.Stream) {
			return UsagePage{}, ErrInvalid
		}
		p.Events[n] = v.Normalize()
	}
	return p, nil
}

type UsagePull struct {
	ID          uuid.UUID `json:"id"`
	SourceID    uuid.UUID `json:"sourceId"`
	WorkspaceID uuid.UUID `json:"workspaceId"`
	Revision    int64     `json:"revision"`
	Cursor      string    `json:"cursor"`
	Outcome     string    `json:"outcome"`
	EvidenceID  uuid.UUID `json:"evidenceId"`
}
type UsageCall struct {
	ID         uuid.UUID  `json:"id"`
	PullID     uuid.UUID  `json:"pullId"`
	Dispatched bool       `json:"dispatched"`
	State      string     `json:"state"`
	Page       *UsagePage `json:"-"`
	EvidenceID uuid.UUID  `json:"evidenceId"`
}
type UsageEvent struct {
	ID              uuid.UUID `json:"id"`
	WorkspaceID     uuid.UUID `json:"workspaceId"`
	SourceID        uuid.UUID `json:"sourceId"`
	Provider        string    `json:"provider"`
	Instance        string    `json:"instance"`
	UsageOccurrence `json:"occurrence"`
	PayloadHash     string    `json:"payloadHash"`
	EvidenceID      uuid.UUID `json:"evidenceId"`
	Status          string    `json:"status"`
	Reason          string    `json:"reason"`
}
type UsageDecision struct {
	ID            uuid.UUID  `json:"id"`
	EventID       uuid.UUID  `json:"eventId"`
	Status        string     `json:"status"`
	Reason        string     `json:"reason"`
	BindingID     *uuid.UUID `json:"bindingId,omitempty"`
	IdentifierID  *uuid.UUID `json:"identifierId,omitempty"`
	PublicationID *uuid.UUID `json:"publicationId,omitempty"`
	ReleaseID     *uuid.UUID `json:"releaseId,omitempty"`
	ObservationID *uuid.UUID `json:"observationId,omitempty"`
	EvidenceID    uuid.UUID  `json:"evidenceId"`
}
