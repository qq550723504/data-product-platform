package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"strings"
	"time"
)

const ArchiveCopy = "ARCHIVE"
const RestoreCopy = "RESTORE"

var ErrRestoreLedger = errors.New("current independent restore ledger is missing, stale or incomplete")

// A target intent exists before a provider can create bytes. TargetCopyID is
// reserved; it becomes a registered exact version only after native observation.
type CopyOperation struct {
	ID, WorkspaceID, TargetCopyID, TargetNamespaceID, PolicyID uuid.UUID
	Source                                                     Copy
	TargetNamespace, TargetKey, Action, BasisRef               string
	ActorID                                                    uuid.UUID
	PrincipalRef                                               string
	DependencyRevision                                         int64
	CreatedAt, ExpiresAt                                       time.Time
}

func (o CopyOperation) Validate() error {
	if o.ID == uuid.Nil || o.Source.Validate() != nil || o.WorkspaceID != o.Source.WorkspaceID || o.TargetCopyID == uuid.Nil || o.TargetNamespaceID == uuid.Nil || o.PolicyID == uuid.Nil || o.ActorID == uuid.Nil || !boundedText(o.PrincipalRef, 255) || !boundedText(o.BasisRef, 255) || !boundedText(o.TargetNamespace, 255) || !boundedText(o.TargetKey, 1024) || (o.Action != ArchiveCopy && o.Action != RestoreCopy) || !o.ExpiresAt.After(o.CreatedAt) || o.DependencyRevision <= 0 {
		return ErrInvalidDispositionFact
	}
	return nil
}
func (o CopyOperation) Materialized(version string) Copy {
	return Copy{ID: o.TargetCopyID, WorkspaceID: o.WorkspaceID, DatasetVersionID: o.Source.DatasetVersionID, ProviderInstanceID: o.Source.ProviderInstanceID, IncarnationID: o.Source.IncarnationID, Namespace: o.TargetNamespace, Key: o.TargetKey, ProviderVersion: version, ContentSHA256: o.Source.ContentSHA256, ByteSize: o.Source.ByteSize}
}

// This single-subject custody witness is separate from restored Core/provider
// backup contents. Sequence and hash chain establish complete exported history;
// current Core state/revision is independently rechecked under its shared fence.
type RestoreCheckpoint struct {
	ID, WorkspaceID, DatasetVersionID, Epoch   uuid.UUID
	Sequence, DependencyRevision               int64
	StateSHA256, PreviousSHA256, ContentSHA256 string
	CreatedAt, ExpiresAt                       time.Time
}

func (c RestoreCheckpoint) Hash() string {
	c.ContentSHA256 = ""
	b, _ := json.Marshal(c)
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
func (c RestoreCheckpoint) Validate(at time.Time) error {
	if c.ID == uuid.Nil || c.WorkspaceID == uuid.Nil || c.DatasetVersionID == uuid.Nil || c.Epoch == uuid.Nil || c.Sequence < 1 || c.DependencyRevision < 0 || !c.ExpiresAt.After(c.CreatedAt) || at.Before(c.CreatedAt) || !at.Before(c.ExpiresAt) || c.ContentSHA256 != c.Hash() {
		return ErrRestoreLedger
	}
	for _, h := range []string{c.StateSHA256, c.PreviousSHA256} {
		b, e := hex.DecodeString(h)
		if e != nil || len(b) != 32 || strings.ToLower(h) != h {
			return ErrRestoreLedger
		}
	}
	return nil
}
func ValidateCheckpointChain(cuts []RestoreCheckpoint, w, v, epoch uuid.UUID) error {
	if len(cuts) == 0 || len(cuts) > 256 {
		return ErrRestoreLedger
	}
	previous := strings.Repeat("0", 64)
	var revision int64 = -1
	var created time.Time
	seen := map[uuid.UUID]bool{}
	for i, c := range cuts {
		if c.Validate(c.CreatedAt) != nil || seen[c.ID] || c.CreatedAt.Before(created) || c.WorkspaceID != w || c.DatasetVersionID != v || c.Epoch != epoch || c.Sequence != int64(i+1) || c.DependencyRevision < revision || c.PreviousSHA256 != previous || c.ContentSHA256 != c.Hash() {
			return ErrRestoreLedger
		}
		previous = c.ContentSHA256
		revision = c.DependencyRevision
		created = c.CreatedAt
		seen[c.ID] = true
	}
	return nil
}
