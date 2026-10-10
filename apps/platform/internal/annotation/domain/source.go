package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"strings"
	"time"
)

const OfficialCEProtocol = "official-ce-reference-v1"
const ControlledSubmissionProtocol = "controlled-fork-submission-v1"
const ImmutableSubmissionSource = "IMMUTABLE_SUBMISSION"

var ErrSourceIntegrity = errors.New("annotation immutable source integrity failure")
var ErrSourceConflict = errors.New("annotation immutable source identity conflict")

// SourceFingerprint contains business content only. Mutable provider status,
// timestamps, observed-current write tokens and physical attempts are receipts.
type SourceFingerprint struct {
	WorkspaceID            uuid.UUID `json:"workspaceID"`
	ConnectionID           uuid.UUID `json:"connectionID"`
	CampaignID             uuid.UUID `json:"campaignID"`
	TaskID                 uuid.UUID `json:"taskID"`
	InputVersionID         uuid.UUID `json:"inputVersionID"`
	CampaignBindingID      uuid.UUID `json:"campaignBindingID"`
	TaskBindingID          uuid.UUID `json:"taskBindingID"`
	ActorBindingID         uuid.UUID `json:"actorBindingID"`
	ProviderInstance       string    `json:"providerInstance"`
	ProviderIncarnation    string    `json:"providerIncarnation"`
	SourceKind             string    `json:"sourceKind"`
	ExternalID             string    `json:"externalID"`
	AssignmentID           string    `json:"assignmentID"`
	ExternalProjectID      string    `json:"externalProjectID"`
	ExternalTaskID         string    `json:"externalTaskID"`
	ExternalAnnotationID   string    `json:"externalAnnotationID"`
	ExternalAuthorRef      string    `json:"externalAuthorRef"`
	CoreAuthorRef          string    `json:"coreAuthorRef"`
	SourceItemRef          string    `json:"sourceItemRef"`
	InputSHA256            string    `json:"inputSHA256"`
	SourceSHA256           string    `json:"sourceSHA256"`
	TaskTextSHA256         string    `json:"taskTextSHA256"`
	ConfigSHA256           string    `json:"configSHA256"`
	SchemaSHA256           string    `json:"schemaSHA256"`
	TaxonomySHA256         string    `json:"taxonomySHA256"`
	RubricSHA256           string    `json:"rubricSHA256"`
	RendererSHA256         string    `json:"rendererSHA256"`
	ReviewPolicySHA256     string    `json:"reviewPolicySHA256"`
	MappingSHA256          string    `json:"mappingSHA256"`
	NormalizerVersion      string    `json:"normalizerVersion"`
	SnapshotSHA256         string    `json:"snapshotSHA256"`
	CanonicalPayloadSHA256 string    `json:"canonicalPayloadSHA256"`
	SubmissionRevision     int64     `json:"submissionRevision"`
}

// AssignmentObservation is the write token read now, never the submit-time token.
// It is appended to receipts and excluded from immutable identity/fingerprints.
type AssignmentObservation struct {
	Version    int64
	ObservedAt time.Time
}
type SourceObservation struct {
	AssignmentObservation AssignmentObservation
	ID                    uuid.UUID
	SourceFingerprint
	Snapshot         []byte
	CanonicalPayload []byte
}

func (s SourceFingerprint) Payload() []byte {
	b, _ := json.Marshal(s)
	return b
}
func SourceDigest(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (s SourceFingerprint) Hash() string { return SourceDigest(s.Payload()) }
func (s SourceFingerprint) Identity() uuid.UUID {
	// No project, assignment, author, mapping or content enters this key.
	b, _ := json.Marshal([]string{s.WorkspaceID.String(), s.ConnectionID.String(),
		s.ProviderInstance, s.ProviderIncarnation, s.SourceKind, s.ExternalID})
	return uuid.NewSHA1(uuid.NameSpaceURL, b)
}
func (s SourceFingerprint) MappingHash() string {
	b, _ := json.Marshal([]string{s.CampaignBindingID.String(), s.TaskBindingID.String(),
		s.ActorBindingID.String(), s.ConfigSHA256, s.SchemaSHA256, s.TaxonomySHA256,
		s.RubricSHA256, s.RendererSHA256, s.ReviewPolicySHA256})
	return SourceDigest(b)
}
func (s SourceObservation) Validate() error {
	if s.WorkspaceID == uuid.Nil {
		return ErrSourceIntegrity
	}
	if s.ConnectionID == uuid.Nil {
		return ErrSourceIntegrity
	}
	if s.CampaignID == uuid.Nil {
		return ErrSourceIntegrity
	}
	if s.TaskID == uuid.Nil {
		return ErrSourceIntegrity
	}
	if s.InputVersionID == uuid.Nil {
		return ErrSourceIntegrity
	}
	if s.CampaignBindingID == uuid.Nil {
		return ErrSourceIntegrity
	}
	if s.TaskBindingID == uuid.Nil {
		return ErrSourceIntegrity
	}
	if s.ActorBindingID == uuid.Nil {
		return ErrSourceIntegrity
	}
	if strings.TrimSpace(s.ProviderInstance) == "" || s.ProviderInstance != strings.TrimSpace(s.ProviderInstance) {
		return ErrSourceIntegrity
	}
	if strings.TrimSpace(s.ProviderIncarnation) == "" || s.ProviderIncarnation != strings.TrimSpace(s.ProviderIncarnation) {
		return ErrSourceIntegrity
	}
	if strings.TrimSpace(s.SourceKind) == "" || s.SourceKind != strings.TrimSpace(s.SourceKind) {
		return ErrSourceIntegrity
	}
	if strings.TrimSpace(s.ExternalID) == "" || s.ExternalID != strings.TrimSpace(s.ExternalID) {
		return ErrSourceIntegrity
	}
	if strings.TrimSpace(s.AssignmentID) == "" || s.AssignmentID != strings.TrimSpace(s.AssignmentID) {
		return ErrSourceIntegrity
	}
	if strings.TrimSpace(s.ExternalProjectID) == "" || s.ExternalProjectID != strings.TrimSpace(s.ExternalProjectID) {
		return ErrSourceIntegrity
	}
	if strings.TrimSpace(s.ExternalTaskID) == "" || s.ExternalTaskID != strings.TrimSpace(s.ExternalTaskID) {
		return ErrSourceIntegrity
	}
	if strings.TrimSpace(s.ExternalAnnotationID) == "" || s.ExternalAnnotationID != strings.TrimSpace(s.ExternalAnnotationID) {
		return ErrSourceIntegrity
	}
	if strings.TrimSpace(s.ExternalAuthorRef) == "" || s.ExternalAuthorRef != strings.TrimSpace(s.ExternalAuthorRef) {
		return ErrSourceIntegrity
	}
	if strings.TrimSpace(s.CoreAuthorRef) == "" || s.CoreAuthorRef != strings.TrimSpace(s.CoreAuthorRef) {
		return ErrSourceIntegrity
	}
	if strings.TrimSpace(s.SourceItemRef) == "" || s.SourceItemRef != strings.TrimSpace(s.SourceItemRef) {
		return ErrSourceIntegrity
	}
	if strings.TrimSpace(s.NormalizerVersion) == "" || s.NormalizerVersion != strings.TrimSpace(s.NormalizerVersion) {
		return ErrSourceIntegrity
	}
	if !isSHA256(s.InputSHA256) {
		return ErrSourceIntegrity
	}
	if !isSHA256(s.SourceSHA256) {
		return ErrSourceIntegrity
	}
	if !isSHA256(s.TaskTextSHA256) {
		return ErrSourceIntegrity
	}
	if !isSHA256(s.ConfigSHA256) {
		return ErrSourceIntegrity
	}
	if !isSHA256(s.SchemaSHA256) {
		return ErrSourceIntegrity
	}
	if !isSHA256(s.TaxonomySHA256) {
		return ErrSourceIntegrity
	}
	if !isSHA256(s.RubricSHA256) {
		return ErrSourceIntegrity
	}
	if !isSHA256(s.RendererSHA256) {
		return ErrSourceIntegrity
	}
	if !isSHA256(s.ReviewPolicySHA256) {
		return ErrSourceIntegrity
	}
	if !isSHA256(s.MappingSHA256) {
		return ErrSourceIntegrity
	}
	if !isSHA256(s.SnapshotSHA256) {
		return ErrSourceIntegrity
	}
	if !isSHA256(s.CanonicalPayloadSHA256) {
		return ErrSourceIntegrity
	}
	if s.SourceKind != ImmutableSubmissionSource || s.SubmissionRevision < 1 ||
		s.ID != s.Identity() || len(s.Snapshot) == 0 || !json.Valid(s.Snapshot) ||
		len(s.CanonicalPayload) == 0 || !json.Valid(s.CanonicalPayload) ||
		SourceDigest(s.Snapshot) != s.SnapshotSHA256 ||
		SourceDigest(s.CanonicalPayload) != s.CanonicalPayloadSHA256 ||
		s.MappingSHA256 != s.MappingHash() {
		return ErrSourceIntegrity
	}
	return nil
}
func (s SourceFingerprint) ExternalRevision() string {
	return fmt.Sprintf("submission/%s/assignment/%s/revision/%d", s.ExternalID, s.AssignmentID, s.SubmissionRevision)
}

type SubmissionExpectation struct {
	TaskID       uuid.UUID `json:"taskId"`
	AssignmentID string    `json:"assignmentId"`
	Revision     int64     `json:"revision"`
}
type SourceContract struct {
	NormalizerVersion   string
	AdmissionProtocol   string
	ConnectionID        uuid.UUID
	ProviderIncarnation string
	SourceCommit        string
	EngineVersion       string
	ImageDigest         string
}

func (c SourceContract) Protocol() string {
	if c.AdmissionProtocol == "" {
		return OfficialCEProtocol
	}
	return c.AdmissionProtocol
}
func (c SourceContract) Validate() error {
	if c.Protocol() == OfficialCEProtocol {
		return nil
	}
	if c.Protocol() != ControlledSubmissionProtocol || c.ConnectionID == uuid.Nil ||
		strings.TrimSpace(c.ProviderIncarnation) == "" || len(c.SourceCommit) != 40 ||
		strings.TrimSpace(c.NormalizerVersion) == "" || strings.TrimSpace(c.EngineVersion) == "" || !strings.HasPrefix(c.ImageDigest, "sha256:") ||
		!isSHA256(strings.TrimPrefix(c.ImageDigest, "sha256:")) {
		return ErrSourceIntegrity
	}
	for _, r := range c.SourceCommit {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return ErrSourceIntegrity
		}
	}
	return nil
}
