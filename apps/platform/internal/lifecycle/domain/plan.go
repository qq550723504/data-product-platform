// Package domain owns lifecycle decisions and historical disposition scope.
// Storage SDKs and actual destructive dispatch belong outside this package.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	DeleteExact       = "DELETE_EXACT"
	Draft             = "DRAFT"
	Finalized         = "FINALIZED"
	PlanHashAlgorithm = "SHA256-LIFECYCLE-PLAN-V1"
	MaxPlanTargets    = 64
	MaxFixtureBytes   = 128 << 20
)

var ErrInvalidPlan = errors.New("invalid lifecycle plan")

// Copy identifies one registered synthetic physical version, never a bare key
// or an ETag. Registration is a server-owned fixture boundary, not user input.
type Copy struct {
	ID                 uuid.UUID
	WorkspaceID        uuid.UUID
	DatasetVersionID   uuid.UUID
	ProviderInstanceID uuid.UUID
	IncarnationID      uuid.UUID
	Namespace          string
	Key                string
	ProviderVersion    string
	ContentSHA256      string
	ByteSize           int64
}

func (c Copy) Validate() error {
	if c.ID == uuid.Nil || c.WorkspaceID == uuid.Nil || c.DatasetVersionID == uuid.Nil ||
		c.ProviderInstanceID == uuid.Nil || c.IncarnationID == uuid.Nil ||
		!boundedText(c.Namespace, 255) || !boundedText(c.Key, 1024) ||
		!boundedText(c.ProviderVersion, 1024) || c.ProviderVersion == "null" ||
		c.ByteSize <= 0 || c.ByteSize > MaxFixtureBytes {
		return ErrInvalidPlan
	}
	decoded, err := hex.DecodeString(c.ContentSHA256)
	if err != nil || len(decoded) != sha256.Size || strings.ToLower(c.ContentSHA256) != c.ContentSHA256 {
		return ErrInvalidPlan
	}
	return nil
}

func boundedText(s string, maxBytes int) bool {
	return s != "" && len(s) <= maxBytes && utf8.ValidString(s) &&
		strings.TrimSpace(s) == s && !strings.ContainsAny(s, "\x00\r\n")
}

type Plan struct {
	ID                 uuid.UUID
	WorkspaceID        uuid.UUID
	Action             string
	Status             string
	PreparedBy         uuid.UUID
	PrincipalRef       string
	DependencyRevision int64
	CreatedAt          time.Time
	FinalizedAt        *time.Time
	ContentHash        string
	Targets            []Copy
}

// ScopeHash uses UTF-8 byte-length framing and UUID order, also implemented by
// the database finalization guard. Delimiters inside keys cannot alias scope.
// It intentionally excludes the plan ID/time, so identical scope is comparable.
func ScopeHash(workspaceID uuid.UUID, action string, targets []Copy) (string, error) {
	if workspaceID == uuid.Nil || action != DeleteExact || len(targets) == 0 || len(targets) > MaxPlanTargets {
		return "", ErrInvalidPlan
	}
	ordered := append([]Copy(nil), targets...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID.String() < ordered[j].ID.String() })
	var b strings.Builder
	frame := func(s string) { b.WriteString(strconv.Itoa(len(s))); b.WriteByte(':'); b.WriteString(s) }
	frame(PlanHashAlgorithm)
	frame(workspaceID.String())
	frame(action)
	for i, c := range ordered {
		if err := c.Validate(); err != nil || c.WorkspaceID != workspaceID || (i > 0 && ordered[i-1].ID == c.ID) {
			return "", ErrInvalidPlan
		}
		for _, s := range []string{c.ID.String(), c.DatasetVersionID.String(), c.ProviderInstanceID.String(),
			c.IncarnationID.String(), c.Namespace, c.Key, c.ProviderVersion, c.ContentSHA256, strconv.FormatInt(c.ByteSize, 10)} {
			frame(s)
		}
	}
	hash := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(hash[:]), nil
}

func (p Plan) VerifyFrozenScope() error {
	hash, err := ScopeHash(p.WorkspaceID, p.Action, p.Targets)
	if err != nil || p.ID == uuid.Nil || p.PreparedBy == uuid.Nil || !boundedText(p.PrincipalRef, 255) ||
		p.DependencyRevision < 0 || p.CreatedAt.IsZero() || p.Status != Finalized || p.FinalizedAt == nil ||
		p.FinalizedAt.Before(p.CreatedAt) || p.ContentHash != hash {
		return fmt.Errorf("%w: scope is not finalized and exact", ErrInvalidPlan)
	}
	return nil
}
