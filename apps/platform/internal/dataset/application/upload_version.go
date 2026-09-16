package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
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

func (s *UploadVersionService) Handle(ctx context.Context, cmd UploadVersionCommand) (domain.DatasetVersion, error) {
	if len(cmd.Content) == 0 {
		return domain.DatasetVersion{}, fmt.Errorf("dataset version content is empty")
	}

	rowCount, err := countRows(cmd.Filename, cmd.ContentType, cmd.Content)
	if err != nil {
		return domain.DatasetVersion{}, err
	}

	var version domain.DatasetVersion
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		allocated, err := s.repo.AllocateVersion(ctx, tx, cmd.DatasetID, cmd.ActorID)
		if err != nil {
			return err
		}
		version = allocated
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
	})
	if err != nil {
		return domain.DatasetVersion{}, err
	}

	filename := sanitizeFilename(cmd.Filename)
	objectName := fmt.Sprintf("datasets/%s/v%06d/%s", version.DatasetID.String(), version.VersionNo, filename)
	storageURI, err := s.store.Put(ctx, objectName, bytes.NewReader(cmd.Content), int64(len(cmd.Content)), cmd.ContentType)
	if err != nil {
		_ = s.tx.Do(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
			return s.repo.SetFailed(ctx, tx, version.ID)
		})
		return domain.DatasetVersion{}, fmt.Errorf("store dataset version: %w", err)
	}

	checksum := fmt.Sprintf("%x", sha256.Sum256(cmd.Content))
	if err := version.MarkReady("OBJECT_STORAGE", storageURI, cmd.ContentType, "SHA256", checksum, rowCount, int64(len(cmd.Content))); err != nil {
		return domain.DatasetVersion{}, err
	}
	version.GeneratedByExecutionID = cmd.GeneratedByExecutionID
	if cmd.Metadata != nil {
		version.Metadata = cmd.Metadata
	}

	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.repo.SetReady(ctx, tx, version); err != nil {
			return err
		}

		event, err := outbox.NewEvent("DATASET_VERSION", version.ID, "DatasetVersionCreated", map[string]any{
			"datasetVersionId":       version.ID,
			"datasetId":              version.DatasetID,
			"versionNo":              version.VersionNo,
			"status":                 version.Status,
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

func countRows(filename, contentType string, content []byte) (int64, error) {
	isCSV := strings.EqualFold(filepath.Ext(filename), ".csv") || strings.Contains(strings.ToLower(contentType), "csv")
	if !isCSV {
		return 0, nil
	}

	reader := csv.NewReader(bytes.NewReader(content))
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
