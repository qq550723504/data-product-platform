package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrReleaseNotReady      = errors.New("product release is not ready")
	ErrIdempotencyKeyNeeded = errors.New("idempotency key is required")
	ErrIdempotencyConflict  = errors.New("idempotency key is already bound to another object")
)

func (r *ProductRelease) Publish(evidenceSnapshotID uuid.UUID, actorID *uuid.UUID) error {
	if r.Status != ReleaseReady || evidenceSnapshotID == uuid.Nil {
		return ErrReleaseNotReady
	}
	now := time.Now().UTC()
	r.Status = ReleasePublished
	r.EvidenceSnapshotID = &evidenceSnapshotID
	r.ReleasedAt = &now
	r.ReleasedBy = actorID
	return nil
}

func NormalizeIdempotencyKey(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ErrIdempotencyKeyNeeded
	}
	if len(value) > 255 {
		return "", ErrIdempotencyKeyNeeded
	}
	return value, nil
}
