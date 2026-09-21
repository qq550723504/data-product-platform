package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/certification/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/certification/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

type ProfileService struct {
	tx   *transaction.Manager
	repo *infrastructure.ProfileRepository
}

func NewProfileService(tx *transaction.Manager, repo *infrastructure.ProfileRepository) *ProfileService {
	return &ProfileService{tx: tx, repo: repo}
}

type CreateProfileCommand struct {
	WorkspaceID uuid.UUID
	Profile     domain.CertificationProfile
	ActorID     *uuid.UUID
	TraceID     string
}

func (s *ProfileService) Create(ctx context.Context, cmd CreateProfileCommand) (domain.ProfileSnapshot, error) {
	snapshot, err := cmd.Profile.SnapshotForWorkspace(cmd.WorkspaceID, cmd.ActorID)
	if err != nil {
		return domain.ProfileSnapshot{}, err
	}
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.repo.InsertProfile(ctx, tx, snapshot); err != nil {
			return err
		}
		event, err := outbox.NewEvent("CERTIFICATION_PROFILE", snapshot.ID, "CertificationProfileCreated", map[string]any{
			"profileId":     snapshot.ID,
			"profileRef":    snapshot.ProfileRef,
			"version":       snapshot.Version,
			"contentSha256": snapshot.ContentSHA256,
			"workspaceId":   snapshot.WorkspaceID,
		})
		if err != nil {
			return fmt.Errorf("create CertificationProfileCreated event: %w", err)
		}
		if err := outbox.Append(ctx, tx, event); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &snapshot.WorkspaceID,
			ActorType:   actorType(cmd.ActorID),
			ActorID:     cmd.ActorID,
			Action:      "CERTIFICATION_PROFILE_CREATED",
			ObjectType:  "CERTIFICATION_PROFILE",
			ObjectID:    snapshot.ID,
			AfterState: map[string]any{
				"profileRef":    snapshot.ProfileRef,
				"version":       snapshot.Version,
				"contentSha256": snapshot.ContentSHA256,
			},
			TraceID: cmd.TraceID,
		})
	})
	return snapshot, err
}

func actorType(actorID *uuid.UUID) string {
	if actorID == nil {
		return "SYSTEM"
	}
	return "USER"
}
