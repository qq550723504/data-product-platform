package application

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
)

type BindEngineActorCommand struct {
	WorkspaceID      uuid.UUID
	ExternalActorRef string
	CoreActorRef     string
	ActorID          *uuid.UUID
	TraceID          string
}

func (s *EngineService) BindActor(
	ctx context.Context,
	cmd BindEngineActorCommand,
) (annotationdomain.EngineActorBinding, error) {
	if err := s.configured(); err != nil {
		return annotationdomain.EngineActorBinding{}, err
	}
	cmd.ExternalActorRef = strings.TrimSpace(cmd.ExternalActorRef)
	cmd.CoreActorRef = strings.TrimSpace(cmd.CoreActorRef)
	binding := annotationdomain.EngineActorBinding{
		ID: uuid.NewSHA1(
			uuid.NameSpaceURL,
			[]byte(
				"annotation-engine-actor:"+
					cmd.WorkspaceID.String()+":"+
					s.engine.Provider()+":"+
					s.engine.InstanceRef()+":"+
					cmd.ExternalActorRef,
			),
		),
		WorkspaceID:      cmd.WorkspaceID,
		Provider:         s.engine.Provider(),
		ProviderInstance: s.engine.InstanceRef(),
		ExternalActorRef: cmd.ExternalActorRef,
		CoreActorRef:     cmd.CoreActorRef,
		CreatedAt:        time.Now().UTC(),
		CreatedBy:        cmd.ActorID,
	}
	if err := binding.Validate(); err != nil {
		return annotationdomain.EngineActorBinding{}, err
	}

	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		created, err := s.repo.InsertEngineActorBinding(ctx, tx, binding)
		if err != nil {
			return err
		}
		if !created {
			return nil
		}
		if _, err := evidence.Append(
			ctx,
			tx,
			evidence.Record{
				WorkspaceID:  binding.WorkspaceID,
				EvidenceType: "ANNOTATION_ENGINE_ACTOR_BOUND",
				Title:        "Annotation engine actor identity bound",
				SourceType:   "CORE",
				Metadata: map[string]any{
					"provider":         binding.Provider,
					"providerInstance": binding.ProviderInstance,
					"externalActorRef": binding.ExternalActorRef,
					"coreActorRef":     binding.CoreActorRef,
				},
				CreatedBy: cmd.ActorID,
			},
			evidence.Relation{
				ObjectType:   "ANNOTATION_ENGINE_ACTOR_BINDING",
				ObjectID:     binding.ID,
				RelationType: "IDENTITY_BINDING_EVIDENCE",
			},
		); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &binding.WorkspaceID,
			ActorType:   actorType(cmd.ActorID),
			ActorID:     cmd.ActorID,
			Action:      "ANNOTATION_ENGINE_ACTOR_BOUND",
			ObjectType:  "ANNOTATION_ENGINE_ACTOR_BINDING",
			ObjectID:    binding.ID,
			AfterState: map[string]any{
				"provider":         binding.Provider,
				"providerInstance": binding.ProviderInstance,
				"externalActorRef": binding.ExternalActorRef,
				"coreActorRef":     binding.CoreActorRef,
			},
			TraceID: cmd.TraceID,
		})
	})
	return binding, err
}
