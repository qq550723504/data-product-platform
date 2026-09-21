package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
)

type Service struct {
	tx   *transaction.Manager
	repo *infrastructure.PostgresRepository
}

func NewService(tx *transaction.Manager, repo *infrastructure.PostgresRepository) *Service {
	return &Service{tx: tx, repo: repo}
}

type CreateAuthorizationCommand struct {
	WorkspaceID uuid.UUID
	Code        string
	GrantorRef  string
	GranteeRef  string
	Purpose     string
	ValidFrom   *time.Time
	ValidTo     *time.Time
	Metadata    map[string]any
	Resources   []domain.ResourceGrantSpec
	ActorID     *uuid.UUID
	TraceID     string
}

type TransitionCommand struct {
	AuthorizationID uuid.UUID
	ActorID         *uuid.UUID
	TraceID         string
	At              time.Time
}

type CreateSnapshotCommand struct {
	WorkspaceID      uuid.UUID
	ProductReleaseID *uuid.UUID
	Purpose          string
	ConsumerRef      string
	AsOf             time.Time
	AuthorizationIDs []uuid.UUID
	ActorID          *uuid.UUID
	TraceID          string
}

func (s *Service) Create(ctx context.Context, cmd CreateAuthorizationCommand) (domain.Authorization, error) {
	authorization, err := domain.NewAuthorization(
		cmd.WorkspaceID,
		cmd.Code,
		cmd.GrantorRef,
		cmd.GranteeRef,
		cmd.Purpose,
		cmd.ValidFrom,
		cmd.ValidTo,
		cmd.Metadata,
		cmd.Resources,
		cmd.ActorID,
	)
	if err != nil {
		return domain.Authorization{}, err
	}
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.repo.InsertAuthorization(ctx, tx, authorization); err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, "AUTHORIZATION", authorization.ID, "AuthorizationCreated", map[string]any{
			"authorizationId": authorization.ID,
			"code":            authorization.Code,
			"purpose":         authorization.Purpose,
			"status":          authorization.Status,
		}); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &authorization.WorkspaceID,
			ActorType:   actorType(cmd.ActorID),
			ActorID:     cmd.ActorID,
			Action:      "AUTHORIZATION_CREATED",
			ObjectType:  "AUTHORIZATION",
			ObjectID:    authorization.ID,
			AfterState: map[string]any{
				"code":          authorization.Code,
				"purpose":       authorization.Purpose,
				"status":        authorization.Status,
				"resourceCount": len(authorization.Resources),
			},
			TraceID: cmd.TraceID,
		})
	})
	return authorization, err
}

func (s *Service) Submit(ctx context.Context, cmd TransitionCommand) (domain.Authorization, error) {
	return s.transition(ctx, cmd, "AUTHORIZATION_SUBMITTED", "AuthorizationSubmitted", func(a *domain.Authorization) error {
		return a.Submit(cmd.ActorID)
	})
}

func (s *Service) Approve(ctx context.Context, cmd TransitionCommand) (domain.Authorization, error) {
	return s.transition(ctx, cmd, "AUTHORIZATION_APPROVED", "AuthorizationApproved", func(a *domain.Authorization) error {
		return a.Approve(cmd.ActorID)
	})
}

func (s *Service) Activate(ctx context.Context, cmd TransitionCommand) (domain.Authorization, error) {
	at := cmd.At
	if at.IsZero() {
		at = time.Now().UTC()
	}
	return s.transition(ctx, cmd, "AUTHORIZATION_ACTIVATED", "AuthorizationActivated", func(a *domain.Authorization) error {
		return a.Activate(at, cmd.ActorID)
	})
}

func (s *Service) Suspend(ctx context.Context, cmd TransitionCommand) (domain.Authorization, error) {
	return s.transition(ctx, cmd, "AUTHORIZATION_SUSPENDED", "AuthorizationSuspended", func(a *domain.Authorization) error {
		return a.Suspend(cmd.ActorID)
	})
}

func (s *Service) Revoke(ctx context.Context, cmd TransitionCommand) (domain.Authorization, error) {
	return s.transition(ctx, cmd, "AUTHORIZATION_REVOKED", "AuthorizationRevoked", func(a *domain.Authorization) error {
		return a.Revoke(cmd.ActorID)
	})
}

func (s *Service) Expire(ctx context.Context, authorizationID uuid.UUID, at time.Time, traceID string) (domain.Authorization, error) {
	cmd := TransitionCommand{AuthorizationID: authorizationID, At: at, TraceID: traceID}
	return s.transition(ctx, cmd, "AUTHORIZATION_EXPIRED", "AuthorizationExpired", func(a *domain.Authorization) error {
		return a.Expire(at)
	})
}

func (s *Service) transition(ctx context.Context, cmd TransitionCommand, auditAction, eventType string, apply func(*domain.Authorization) error) (domain.Authorization, error) {
	authorization, err := s.repo.GetAuthorization(ctx, cmd.AuthorizationID)
	if err != nil {
		return domain.Authorization{}, err
	}
	before := authorization.Status
	if err := apply(&authorization); err != nil {
		return domain.Authorization{}, err
	}
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.repo.SaveAuthorizationState(ctx, tx, authorization); err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, "AUTHORIZATION", authorization.ID, eventType, map[string]any{
			"authorizationId": authorization.ID,
			"before":          before,
			"after":           authorization.Status,
		}); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &authorization.WorkspaceID,
			ActorType:   actorType(cmd.ActorID),
			ActorID:     cmd.ActorID,
			Action:      auditAction,
			ObjectType:  "AUTHORIZATION",
			ObjectID:    authorization.ID,
			BeforeState: map[string]any{"status": before},
			AfterState:  map[string]any{"status": authorization.Status},
			TraceID:     cmd.TraceID,
		})
	})
	return authorization, err
}

func (s *Service) CreateSnapshot(ctx context.Context, cmd CreateSnapshotCommand) (domain.RightsSnapshot, error) {
	if cmd.AsOf.IsZero() {
		cmd.AsOf = time.Now().UTC()
	}
	if len(cmd.AuthorizationIDs) == 0 {
		return domain.RightsSnapshot{}, domain.ErrInvalidRightsSnapshot
	}
	authorizations := make([]domain.Authorization, 0, len(cmd.AuthorizationIDs))
	for _, authorizationID := range cmd.AuthorizationIDs {
		authorization, err := s.repo.GetAuthorization(ctx, authorizationID)
		if err != nil {
			return domain.RightsSnapshot{}, err
		}
		authorizations = append(authorizations, authorization)
	}
	snapshot, err := domain.NewRightsSnapshot(
		cmd.WorkspaceID,
		cmd.ProductReleaseID,
		cmd.Purpose,
		cmd.ConsumerRef,
		cmd.AsOf,
		authorizations,
		cmd.ActorID,
	)
	if err != nil {
		return domain.RightsSnapshot{}, err
	}
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := deliveryfence.Lock(ctx, tx, snapshot.WorkspaceID); err != nil {
			return err
		}
		if err := s.repo.InsertSnapshot(ctx, tx, snapshot); err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, "RIGHTS_SNAPSHOT", snapshot.ID, "RightsSnapshotCreated", map[string]any{
			"rightsSnapshotId": snapshot.ID,
			"productReleaseId": snapshot.ProductReleaseID,
			"purpose":          snapshot.Purpose,
		}); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &snapshot.WorkspaceID,
			ActorType:   actorType(cmd.ActorID),
			ActorID:     cmd.ActorID,
			Action:      "RIGHTS_SNAPSHOT_CREATED",
			ObjectType:  "RIGHTS_SNAPSHOT",
			ObjectID:    snapshot.ID,
			AfterState: map[string]any{
				"purpose":            snapshot.Purpose,
				"consumerRef":        snapshot.ConsumerRef,
				"authorizationCount": len(snapshot.Manifest.Authorizations),
			},
			TraceID: cmd.TraceID,
		})
	})
	if err == nil {
		if stored, readErr := s.repo.GetSnapshot(ctx, snapshot.ID); readErr == nil {
			snapshot = stored
		} else {
			err = readErr
		}
	}
	return snapshot, err
}

func appendEvent(ctx context.Context, tx pgx.Tx, aggregateType string, aggregateID uuid.UUID, eventType string, payload map[string]any) error {
	event, err := outbox.NewEvent(aggregateType, aggregateID, eventType, payload)
	if err != nil {
		return fmt.Errorf("create %s event: %w", eventType, err)
	}
	return outbox.Append(ctx, tx, event)
}

func actorType(actorID *uuid.UUID) string {
	if actorID == nil {
		return "SYSTEM"
	}
	return "USER"
}
