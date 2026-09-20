package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/csvinput"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

type ObjectStore interface {
	Put(ctx context.Context, objectName string, reader io.Reader, size int64, contentType string) (string, error)
}

type UploadVersionCommand struct {
	DatasetID              uuid.UUID
	Filename               string
	ContentType            string
	Content                []byte
	ActorID                *uuid.UUID
	TraceID                string
	GeneratedByExecutionID *uuid.UUID
	Metadata               map[string]any
}

type UploadVersionService struct {
	tx    *transaction.Manager
	repo  *infrastructure.PostgresRepository
	store ObjectStore
}

func NewUploadVersionService(tx *transaction.Manager, repo *infrastructure.PostgresRepository, store ObjectStore) *UploadVersionService {
	return &UploadVersionService{tx: tx, repo: repo, store: store}
}

// Handle writes a DatasetVersion through the two-phase output path: allocate the
// row, store the object, then publish it READY. The phases are the recovery
// points C2 depends on, so the command carries GeneratedByExecutionID as the
// output idempotency key whenever an Execution is the producer.
//
// With a key present a replayed delivery is not a second output:
//   - an already READY version produced by this Execution is returned untouched;
//   - a half-product of the same Execution (CREATED/PROCESSING/FAILED) reuses the
//     same row, so version_no and object key stay stable, and a FAILED one is
//     republished in place instead of consuming a new version number;
//   - an INVALID/SUPERSEDED row is a hard error rather than a silent overwrite.
//
// Both phases re-read the committed row under a lock: several deliveries of one
// Execution share the row, so nothing may be published on the strength of the
// state this call allocated.
func (s *UploadVersionService) Handle(ctx context.Context, cmd UploadVersionCommand) (domain.DatasetVersion, error) {
	if len(cmd.Content) == 0 {
		return domain.DatasetVersion{}, fmt.Errorf("dataset version content is empty")
	}

	rowCount, err := countRows(cmd.Filename, cmd.ContentType, cmd.Content)
	if err != nil {
		return domain.DatasetVersion{}, err
	}

	var version domain.DatasetVersion
	// alreadyPublished short-circuits a replay of a version this Execution already
	// published: there is nothing left to write, and no new fact to record.
	alreadyPublished := false
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		allocated, reused, err := s.repo.AllocateVersion(ctx, tx, cmd.DatasetID, cmd.ActorID, cmd.GeneratedByExecutionID)
		if err != nil {
			return err
		}
		version = allocated
		if !reused {
			return audit.Append(ctx, tx, audit.Event{
				ActorType:  actorType(cmd.ActorID),
				ActorID:    cmd.ActorID,
				Action:     "DATASET_VERSION_CREATED",
				ObjectType: "DATASET_VERSION",
				ObjectID:   version.ID,
				AfterState: map[string]any{
					"datasetId": version.DatasetID,
					"versionNo": version.VersionNo,
					"status":    version.Status,
				},
				TraceID: cmd.TraceID,
			})
		}

		// A READY version is a published fact: reuse it without rewriting anything.
		if version.Status == domain.VersionReady {
			alreadyPublished = true
			return nil
		}
		if version.Status == domain.VersionInvalid || version.Status == domain.VersionSuperseded {
			return fmt.Errorf("dataset version %s produced by execution %s is %s and cannot be reused as an output", version.ID, cmd.GeneratedByExecutionID, version.Status)
		}
		// CREATED/PROCESSING/FAILED are all reusable half-products. A FAILED row is
		// not repaired here: this attempt has no durable content yet, so its state is
		// only decided when the publish phase runs with the object write behind it.
		return audit.Append(ctx, tx, audit.Event{
			ActorType:  actorType(cmd.ActorID),
			ActorID:    cmd.ActorID,
			Action:     "DATASET_VERSION_REUSED",
			ObjectType: "DATASET_VERSION",
			ObjectID:   version.ID,
			BeforeState: map[string]any{
				"status": allocated.Status,
			},
			AfterState: map[string]any{
				"status":    version.Status,
				"datasetId": version.DatasetID,
				"versionNo": version.VersionNo,
			},
			Reason:  "execution output idempotency key matched an existing half-product",
			TraceID: cmd.TraceID,
		})
	})
	if err != nil {
		return domain.DatasetVersion{}, err
	}
	// Replay of an already published output: no object write, no new facts.
	if alreadyPublished {
		return version, nil
	}

	// Each attempt stages its content under its own object key.
	//
	// The output DatasetVersion row is shared between concurrent deliveries of one
	// Execution, but object storage has no conditional write: if attempts shared a
	// key, a loser could overwrite the bytes the winner already published with a
	// different payload, leaving a READY row whose checksum no longer matches its
	// object. Isolating the staged object means only the winning attempt's own bytes
	// are ever referenced, and a loser merely leaves an unreferenced staged object.
	attemptToken := uuid.New()
	filename := sanitizeFilename(cmd.Filename)
	objectName := fmt.Sprintf("datasets/%s/v%06d/%s/%s", version.DatasetID.String(), version.VersionNo, attemptToken.String(), filename)
	storageURI, err := s.store.Put(ctx, objectName, bytes.NewReader(cmd.Content), int64(len(cmd.Content)), cmd.ContentType)
	if err != nil {
		_ = s.markFailed(context.Background(), version.ID, cmd)
		return domain.DatasetVersion{}, fmt.Errorf("store dataset version: %w", err)
	}

	checksum := fmt.Sprintf("%x", sha256.Sum256(cmd.Content))
	// MarkReady accepts CREATED/PROCESSING/FAILED, so a half-product recovered by
	// this attempt becomes READY with this attempt's staged content.
	if err := version.MarkReady("OBJECT_STORAGE", storageURI, cmd.ContentType, "SHA256", checksum, rowCount, int64(len(cmd.Content))); err != nil {
		return domain.DatasetVersion{}, err
	}
	version.GeneratedByExecutionID = cmd.GeneratedByExecutionID
	if cmd.Metadata != nil {
		version.Metadata = cmd.Metadata
	}

	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		// Lock the row and re-read its committed state. A concurrent delivery of the
		// same Execution may have published this exact version (or failed it) between
		// our allocation and this commit, so the decision to publish is taken on the
		// committed row, never on the state this call allocated.
		current, err := s.repo.LockVersion(ctx, tx, version.ID)
		if err != nil {
			return err
		}
		switch current.Status {
		case domain.VersionReady:
			// Already published by a concurrent delivery: adopt that row, do not emit a
			// second set of facts and do not overwrite its storage reference.
			version = current
			return nil
		case domain.VersionInvalid, domain.VersionSuperseded:
			return fmt.Errorf("dataset version %s is %s and cannot be published", version.ID, current.Status)
		}
		// CREATED/PROCESSING/FAILED: this attempt holds durably stored content, so it
		// is the one that publishes. SetReady performs the transition under the lock
		// and fails loudly if the row is no longer publishable.

		if err := s.repo.SetReady(ctx, tx, version); err != nil {
			return err
		}

		event, err := outbox.NewEvent("DATASET_VERSION", version.ID, "DatasetVersionCreated", map[string]any{
			"datasetVersionId":       version.ID,
			"datasetId":              version.DatasetID,
			"versionNo":              version.VersionNo,
			"status":                 version.Status,
			"previousStatus":         string(current.Status),
			"checksum":               version.ChecksumValue,
			"generatedByExecutionId": version.GeneratedByExecutionID,
		})
		if err != nil {
			return fmt.Errorf("create dataset version event: %w", err)
		}
		if err := outbox.Append(ctx, tx, event); err != nil {
			return err
		}

		return audit.Append(ctx, tx, audit.Event{
			ActorType:  actorType(cmd.ActorID),
			ActorID:    cmd.ActorID,
			Action:     "DATASET_VERSION_READY",
			ObjectType: "DATASET_VERSION",
			ObjectID:   version.ID,
			AfterState: map[string]any{
				"datasetId":              version.DatasetID,
				"versionNo":              version.VersionNo,
				"status":                 version.Status,
				"previousStatus":         string(current.Status),
				"storageUri":             version.StorageURI,
				"checksum":               version.ChecksumValue,
				"rowCount":               rowCount,
				"generatedByExecutionId": version.GeneratedByExecutionID,
			},
			TraceID: cmd.TraceID,
		})
	})
	if err != nil {
		return domain.DatasetVersion{}, err
	}

	return version, nil
}

// markFailed records a real output-attempt failure only when it wins the
// CREATED/PROCESSING -> FAILED transition. A concurrent publisher may already
// have committed READY, in which case there is no state change to announce.
func (s *UploadVersionService) markFailed(ctx context.Context, versionID uuid.UUID, cmd UploadVersionCommand) error {
	return s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		current, err := s.repo.LockVersion(ctx, tx, versionID)
		if err != nil {
			return err
		}
		if current.Status != domain.VersionCreated && current.Status != domain.VersionProcessing {
			return nil
		}
		before := current.Status
		if err := current.MarkFailed(); err != nil {
			return err
		}
		if err := s.repo.Fail(ctx, tx, current); err != nil {
			return err
		}
		event, err := outbox.NewEvent("DATASET_VERSION", current.ID, "DatasetVersionFailed", map[string]any{
			"datasetVersionId": current.ID,
			"datasetId":        current.DatasetID,
			"versionNo":        current.VersionNo,
			"previousStatus":   before,
			"reason":           "object storage write failed",
		})
		if err != nil {
			return fmt.Errorf("create dataset version failure event: %w", err)
		}
		if err := outbox.Append(ctx, tx, event); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			ActorType:  actorType(cmd.ActorID),
			ActorID:    cmd.ActorID,
			Action:     "DATASET_VERSION_FAILED",
			ObjectType: "DATASET_VERSION",
			ObjectID:   current.ID,
			BeforeState: map[string]any{
				"status": before,
			},
			AfterState: map[string]any{
				"status": current.Status,
			},
			Reason:  "object storage write failed",
			TraceID: cmd.TraceID,
		})
	})
}

func countRows(filename, contentType string, content []byte) (int64, error) {
	isCSV := strings.EqualFold(filepath.Ext(filename), ".csv") || strings.Contains(strings.ToLower(contentType), "csv")
	if !isCSV {
		return 0, nil
	}

	reader := csvinput.NewReader(bytes.NewReader(content))
	var records int64
	first := true
	for {
		_, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("parse CSV: %w", err)
		}
		if first {
			first = false
			continue
		}
		records++
	}
	return records, nil
}

func sanitizeFilename(filename string) string {
	filename = filepath.Base(strings.TrimSpace(filename))
	if filename == "." || filename == "" {
		return "dataset.bin"
	}
	return strings.ReplaceAll(filename, " ", "_")
}
