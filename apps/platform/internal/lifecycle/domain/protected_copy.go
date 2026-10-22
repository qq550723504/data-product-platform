package domain

import (
	"encoding/hex"
	"path"
	"strings"

	"github.com/google/uuid"
)

const ProtectedCreateIdentity = "PROTECTED_CREATE"

// ProtectedCopy is a different identity from Copy's exact provider version.
// It cannot be passed to the existing versioned delete/archive/restore ports.
// VerificationID references immutable conditional-create + readback facts.
type ProtectedCopy struct {
	ID, WorkspaceID, DatasetVersionID, ProviderInstanceID, IncarnationID, VerificationID uuid.UUID
	Namespace, Key, ContentSHA256                                                        string
	ByteSize                                                                             int64
}

func (c ProtectedCopy) Validate() error {
	h, err := hex.DecodeString(c.ContentSHA256)
	if c.ID == uuid.Nil || c.WorkspaceID == uuid.Nil || c.DatasetVersionID == uuid.Nil || c.ProviderInstanceID == uuid.Nil || c.IncarnationID == uuid.Nil || c.VerificationID == uuid.Nil ||
		!boundedText(c.Namespace, 255) || strings.ContainsAny(c.Namespace, "/\\") || !boundedText(c.Key, 1024) ||
		!strings.HasPrefix(c.Key, "core/"+c.WorkspaceID.String()+"/") || path.Clean(c.Key) != c.Key || strings.ContainsAny(c.Key, "%\\\x00\r\n") ||
		err != nil || len(h) != 32 || strings.ToLower(c.ContentSHA256) != c.ContentSHA256 || c.ByteSize <= 0 || c.ByteSize > MaxFixtureBytes {
		return ErrInvalidDispositionFact
	}
	return nil
}
