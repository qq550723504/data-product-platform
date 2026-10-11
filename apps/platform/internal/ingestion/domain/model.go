// Package domain owns ingestion identities and batch acceptance semantics.
// Connectors, SQL, engine SDKs and credentials belong to adapters.
package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	ErrManifest = errors.New("invalid ingestion manifest")
	ErrConflict = errors.New("ingestion idempotency conflict")
	ErrDenied   = errors.New("ingestion authority denied")
	ErrUnknown  = errors.New("ingestion outcome unknown; reconciliation required")
	ErrState    = errors.New("ingestion command not allowed in current state")
)

const Contract = "text-csv-batch-v1"
const MaxBytes int64 = 128 << 20

var token = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)
var digest = regexp.MustCompile(`^[0-9a-f]{64}$`)

type Slot struct {
	Code       string    `json:"code"`
	DatasetID  uuid.UUID `json:"datasetId"`
	ResourceID uuid.UUID `json:"resourceId"`
	Columns    []string  `json:"columns"`
	Rows       int64     `json:"rows"`
}
type Request struct {
	WorkspaceID   uuid.UUID `json:"workspaceId"`
	ActorID       uuid.UUID `json:"actorId"`
	PrincipalRef  string    `json:"principalRef"`
	RequestKey    string    `json:"requestKey"`
	BatchKey      string    `json:"batchKey"`
	ConnectionRef string    `json:"connectionRef"`
	InstanceRef   string    `json:"instanceRef"`
	ConfigHash    string    `json:"configHash"`
	TemplateHash  string    `json:"templateHash"`
	SchemaHash    string    `json:"schemaHash"`
	Slots         []Slot    `json:"slots"`
}
type Operation struct {
	ID        uuid.UUID `json:"id"`
	Request   Request   `json:"request"`
	State     string    `json:"state"`
	AttemptID uuid.UUID `json:"attemptId"`
	RemoteID  string    `json:"remoteId"`
	Manifest  *Manifest `json:"manifest,omitempty"`
}
type Attempt struct {
	ID          uuid.UUID `json:"id"`
	OperationID uuid.UUID `json:"operationId"`
	StartedAt   time.Time `json:"startedAt"`
}
type Output struct {
	Code      string    `json:"code"`
	DatasetID uuid.UUID `json:"datasetId"`
	URI       string    `json:"uri"`
	Bytes     int64     `json:"bytes"`
	Rows      int64     `json:"rows"`
	SHA256    string    `json:"sha256"`
	Columns   []string  `json:"columns"`
}
type Manifest struct {
	Contract      string    `json:"contract"`
	OperationID   uuid.UUID `json:"operationId"`
	AttemptID     uuid.UUID `json:"attemptId"`
	WorkspaceID   uuid.UUID `json:"workspaceId"`
	RequestKey    string    `json:"requestKey"`
	BatchKey      string    `json:"batchKey"`
	ConnectionRef string    `json:"connectionRef"`
	InstanceRef   string    `json:"instanceRef"`
	ConfigHash    string    `json:"configHash"`
	TemplateHash  string    `json:"templateHash"`
	SchemaHash    string    `json:"schemaHash"`
	EngineVersion string    `json:"engineVersion"`
	RemoteID      string    `json:"remoteId"`
	ObservedAt    time.Time `json:"observedAt"`
	Outputs       []Output  `json:"outputs"`
}
type Binding struct {
	Code      string    `json:"code"`
	DatasetID uuid.UUID `json:"datasetId"`
	VersionID uuid.UUID `json:"versionId"`
	URI       string    `json:"uri"`
	SHA256    string    `json:"sha256"`
}
type Receipt struct {
	ID           uuid.UUID `json:"id"`
	OperationID  uuid.UUID `json:"operationId"`
	AttemptID    uuid.UUID `json:"attemptId"`
	ManifestHash string    `json:"manifestHash"`
	Bindings     []Binding `json:"bindings"`
}

func Hash(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }
func (r Request) Validate() error {
	if r.WorkspaceID == uuid.Nil || r.ActorID == uuid.Nil || strings.TrimSpace(r.PrincipalRef) == "" || len(r.PrincipalRef) > 255 || !token.MatchString(r.RequestKey) || !token.MatchString(r.BatchKey) || !token.MatchString(r.ConnectionRef) || !token.MatchString(r.InstanceRef) || !digest.MatchString(r.ConfigHash) || !digest.MatchString(r.TemplateHash) || !digest.MatchString(r.SchemaHash) || len(r.Slots) != 3 {
		return ErrManifest
	}
	codes := map[string]bool{}
	datasets := map[uuid.UUID]bool{}
	resources := map[uuid.UUID]bool{}
	for _, s := range r.Slots {
		if !token.MatchString(s.Code) || codes[s.Code] || datasets[s.DatasetID] || resources[s.ResourceID] || s.DatasetID == uuid.Nil || s.ResourceID == uuid.Nil || s.Rows < 0 || len(s.Columns) == 0 {
			return ErrManifest
		}
		codes[s.Code] = true
		datasets[s.DatasetID] = true
		resources[s.ResourceID] = true
		columns := map[string]bool{}
		for _, c := range s.Columns {
			if !token.MatchString(c) || columns[c] {
				return ErrManifest
			}
			columns[c] = true
		}
	}
	return nil
}
func (r Request) Fingerprint() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	r.Slots = slices.Clone(r.Slots)
	slices.SortFunc(r.Slots, func(a, b Slot) int { return strings.Compare(a.Code, b.Code) })
	data, err := json.Marshal(r)
	return Hash(data), err
}
func (m Manifest) Canonical() ([]byte, string, error) {
	m.Outputs = slices.Clone(m.Outputs)
	slices.SortFunc(m.Outputs, func(a, b Output) int { return strings.Compare(a.Code, b.Code) })
	m.ObservedAt = m.ObservedAt.UTC()
	data, err := json.Marshal(m)
	return data, Hash(data), err
}
func Prefix(bucket string, op Operation) string {
	return fmt.Sprintf("s3://%s/staging/%s/%s/%s/", bucket, op.Request.WorkspaceID, op.ID, op.AttemptID)
}
func CoreKey(op Operation, output Output) string {
	return fmt.Sprintf("core/%s/%s/%s/%s/%s.csv", op.Request.WorkspaceID, op.ID, op.AttemptID, output.SHA256, output.Code)
}
func (m Manifest) Validate(op Operation, bucket, engineVersion string) error {
	r := op.Request
	if m.Contract != Contract || m.OperationID != op.ID || m.AttemptID != op.AttemptID || m.WorkspaceID != r.WorkspaceID || m.RequestKey != r.RequestKey || m.BatchKey != r.BatchKey || m.ConnectionRef != r.ConnectionRef || m.InstanceRef != r.InstanceRef || m.ConfigHash != r.ConfigHash || m.TemplateHash != r.TemplateHash || m.SchemaHash != r.SchemaHash || m.EngineVersion != engineVersion || m.RemoteID == "" || m.RemoteID != op.RemoteID || m.ObservedAt.IsZero() || len(m.Outputs) != 3 {
		return ErrManifest
	}
	seen := map[string]bool{}
	for _, out := range m.Outputs {
		var slot *Slot
		for i := range r.Slots {
			if r.Slots[i].Code == out.Code {
				slot = &r.Slots[i]
				break
			}
		}
		if slot == nil || seen[out.Code] || out.DatasetID != slot.DatasetID || !slices.Equal(out.Columns, slot.Columns) || out.Rows != slot.Rows || out.Bytes <= 0 || out.Bytes > MaxBytes || !digest.MatchString(out.SHA256) || out.URI != Prefix(bucket, op)+out.Code+".csv" {
			return ErrManifest
		}
		seen[out.Code] = true
	}
	return nil
}

// ValidateContent reads original strings. It does not use the processing CSV
// normalizer (which trims values), accept BOM/CRLF, or silently repair widths.
func ValidateContent(out Output, data []byte) error {
	if int64(len(data)) != out.Bytes || Hash(data) != out.SHA256 || !utf8.Valid(data) || bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) || bytes.Contains(data, []byte{'\r'}) || bytes.IndexByte(data, 0) >= 0 || len(data) == 0 || data[len(data)-1] != '\n' {
		return ErrManifest
	}
	r := csv.NewReader(bytes.NewReader(data))
	header, err := r.Read()
	if err != nil || !slices.Equal(header, out.Columns) {
		return ErrManifest
	}
	var count int64
	for {
		_, err = r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ErrManifest
		}
		count++
	}
	if count != out.Rows {
		return ErrManifest
	}
	return nil
}
