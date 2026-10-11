package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
)

const (
	CapabilityInspectStorage = "LIFECYCLE_INSPECT_STORAGE"
	StorageCallStartedEvent  = "LifecycleStorageCallStarted"
	StorageCallObservedEvent = "LifecycleStorageCallObserved"
)

// PostgresStorageCallJournal is private server wiring, never an HTTP receipt
// ingestion API. Only the instrumented adapter supplies physical observations.
// Start admission commits before the adapter sends I/O; each observed physical
// invocation commits its own cost independently of inspection success.
type PostgresStorageCallJournal struct{ service *OccupancyService }

var _ StorageCallJournal = (*PostgresStorageCallJournal)(nil)

func NewPostgresStorageCallJournal(s *OccupancyService) *PostgresStorageCallJournal {
	return &PostgresStorageCallJournal{service: s}
}

func (j *PostgresStorageCallJournal) StartStorageCall(ctx context.Context, c StorageCallStart) error {
	if j == nil || j.service == nil || !j.service.configured() || c.ID == uuid.Nil || c.Copy.Validate() != nil || c.StartedAt.IsZero() {
		return ErrStorageScope
	}
	switch c.Operation {
	case StorageStatExact, StorageRetention, StorageLegalHold, StorageListVersions:
	default:
		return ErrStorageUnsupported
	}
	s := j.service
	c.StartedAt = c.StartedAt.UTC().Truncate(time.Microsecond)
	return s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		revision, err := deliveryfence.Lock(ctx, tx, c.Copy.WorkspaceID)
		if err != nil {
			return err
		}
		copies, err := s.repo.LockFixtureCopies(ctx, tx, c.Copy.WorkspaceID, []uuid.UUID{c.Copy.ID})
		if err != nil {
			return err
		}
		if len(copies) != 1 || copies[0] != c.Copy {
			return ErrStorageScope
		}
		caller, at, err := s.authorize(ctx, tx, c.Copy.WorkspaceID, CapabilityInspectStorage, nil)
		if err != nil {
			return err
		}
		if c.StartedAt.After(at) {
			return ErrStorageContract
		}
		tag, err := tx.Exec(ctx, `INSERT INTO lifecycle_storage_call(id,workspace_id,copy_id,operation,started_at,actor_id,principal_ref,dependency_revision)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(id) DO NOTHING`, c.ID, c.Copy.WorkspaceID, c.Copy.ID, c.Operation, c.StartedAt, caller.ActorID, caller.Subject, revision)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			// Reusing a physical identity must never admit a second request. The
			// adapter creates a fresh ID even when re-inspecting the same copy.
			return ErrIdempotencyConflict
		}
		return appendDispositionFacts(ctx, tx, StorageCallStartedEvent, "LIFECYCLE_STORAGE_CALL", c.ID, caller, revision, at,
			map[string]any{"copy_id": c.Copy.ID, "operation": c.Operation})
	})
}

func (j *PostgresStorageCallJournal) ObserveStorageCall(ctx context.Context, o StorageCallObservation) error {
	if j == nil || j.service == nil || !j.service.configured() || !validStorageObservation(o) {
		return ErrStorageContract
	}
	s := j.service
	o.ObservedAt = o.ObservedAt.UTC().Truncate(time.Microsecond)
	if o.InvokedAt != nil {
		at := o.InvokedAt.UTC().Truncate(time.Microsecond)
		o.InvokedAt = &at
	}
	body, err := json.Marshal(o)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(body)
	proof := hex.EncodeToString(hash[:])
	return s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var w, copyID uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT workspace_id,copy_id FROM lifecycle_storage_call WHERE id=$1`, o.CallID).Scan(&w, &copyID); err != nil {
			return err
		}
		revision, err := deliveryfence.Lock(ctx, tx, w)
		if err != nil {
			return err
		}
		if _, err := s.repo.LockFixtureCopies(ctx, tx, w, []uuid.UUID{copyID}); err != nil {
			return err
		}
		var started time.Time
		// Observing incurred work is not a new permission to invoke. Revoking the
		// initiating caller must not make a failed/late request's cost disappear.
		caller := principal.RequestPrincipal{WorkspaceID: w}
		if err := tx.QueryRow(ctx, `SELECT started_at,actor_id,principal_ref FROM lifecycle_storage_call WHERE id=$1 AND workspace_id=$2 FOR UPDATE`, o.CallID, w).
			Scan(&started, &caller.ActorID, &caller.Subject); err != nil {
			return err
		}
		at := s.now().UTC()
		if o.ObservedAt.Before(started) || o.ObservedAt.After(at) || (o.InvokedAt != nil && o.InvokedAt.Before(started)) {
			return ErrStorageContract
		}
		id := uuid.New()
		tag, err := tx.Exec(ctx, `INSERT INTO lifecycle_storage_observation(id,workspace_id,call_id,observed_at,invoked_at,http_status,request_ref,response_sha256,failure,proof_sha256,copy_result_version)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,'')) ON CONFLICT(call_id,proof_sha256) DO NOTHING`, id, w, o.CallID, o.ObservedAt, o.InvokedAt, o.HTTPStatus, o.RequestRef, o.ResponseSHA256, o.Failure, proof, o.CopyResultVersion)
		if err != nil {
			return err
		}
		if o.InvokedAt != nil {
			if err := cost.AppendLifecycleStorageActivity(ctx, tx, w, o.CallID, *o.InvokedAt); err != nil {
				return err
			}
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		return appendDispositionFacts(ctx, tx, StorageCallObservedEvent, "LIFECYCLE_STORAGE_OBSERVATION", id, caller, revision, at,
			map[string]any{"call_id": o.CallID, "http_status": o.HTTPStatus, "invocation_confirmed": o.InvokedAt != nil, "failure": o.Failure})
	})
}

func validStorageObservation(o StorageCallObservation) bool {
	if o.CopyResultVersion != "" && (!validText(o.CopyResultVersion, 1024) || o.CopyResultVersion == "null") {
		return false
	}
	if o.CallID == uuid.Nil || o.ObservedAt.IsZero() || (o.InvokedAt != nil && (o.InvokedAt.IsZero() || o.InvokedAt.After(o.ObservedAt))) ||
		(o.HTTPStatus != 0 && (o.HTTPStatus < 100 || o.HTTPStatus > 599 || o.InvokedAt == nil)) || len(o.RequestRef) > 256 || strings.ContainsAny(o.RequestRef, "\x00\r\n") {
		return false
	}
	if o.ResponseSHA256 != "" {
		digest, err := hex.DecodeString(o.ResponseSHA256)
		if err != nil || len(digest) != 32 || strings.ToLower(o.ResponseSHA256) != o.ResponseSHA256 {
			return false
		}
	}
	switch o.Failure {
	case "", "TRANSPORT", "CONTRACT", "JOURNAL":
		return true
	default:
		return false
	}
}
