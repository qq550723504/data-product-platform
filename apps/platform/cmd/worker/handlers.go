package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	externalqueue "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/transport/queue"
	lifecyclequeue "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/transport/queue"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	workflowqueue "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/transport/queue"
)

// governanceProjector is the worker's view of the governance projection
// service. Keeping it narrow lets the outbox wiring be validated without a live
// OpenMetadata deployment.
type governanceProjector interface {
	HandleOutboxEvent(ctx context.Context, event outbox.PublishedEvent) error
}

// metadataProjectionHandler adapts the governance projection service to the
// dispatcher handler contract.
func metadataProjectionHandler(service governanceProjector, logger *slog.Logger) outbox.Handler {
	return func(ctx context.Context, event outbox.PublishedEvent) error {
		if err := service.HandleOutboxEvent(ctx, event); err != nil {
			logger.Error(
				"governance projection failed",
				"event_id", event.ID,
				"event_type", event.EventType,
				"aggregate_id", event.AggregateID,
				"attempt", event.Attempts,
				"error", err,
			)
			return err
		}
		return nil
	}
}

// workerHandlerRegistrations builds the handler set for the deployment. A nil
// projector means no governance provider is configured, matching the
// retention-only governance route produced by routing.Routes(false).
func executionQueueHandler(enqueuer workflowqueue.ExecutionEnqueuer) outbox.Handler {
	return func(ctx context.Context, event outbox.PublishedEvent) error {
		if event.AggregateType != "EXECUTION" {
			return fmt.Errorf("execution queue handler received aggregate type %q", event.AggregateType)
		}
		if err := enqueuer.EnqueueExecution(ctx, event.AggregateID); err != nil {
			return fmt.Errorf("enqueue execution %s from outbox event %s: %w", event.AggregateID, event.ID, err)
		}
		return nil
	}
}

func workerHandlerRegistrations(service governanceProjector, enqueuer workflowqueue.ExecutionEnqueuer, external externalqueue.Enqueuer, lifecycle lifecyclequeue.Enqueuer, logger *slog.Logger) []outbox.HandlerRegistration {
	registrations := make([]outbox.HandlerRegistration, 0, 4)
	if enqueuer != nil {
		registrations = append(registrations, outbox.HandlerRegistration{
			Name:   routing.HandlerExecutionQueue,
			Handle: executionQueueHandler(enqueuer),
		})
	}
	if external != nil {
		registrations = append(registrations, outbox.HandlerRegistration{Name: routing.HandlerExternalPublicationQueue, Handle: externalqueue.OutboxHandler(external)})
	}
	if lifecycle != nil {
		registrations = append(registrations, outbox.HandlerRegistration{Name: routing.HandlerLifecycleRecoveryQueue, Handle: lifecyclequeue.OutboxHandler(lifecycle)})
	}
	if service != nil {
		registrations = append(registrations, outbox.HandlerRegistration{
			Name:   routing.HandlerMetadataProjection,
			Handle: metadataProjectionHandler(service, logger),
		})
	}
	return registrations
}

// newOutboxDispatcher builds the dispatcher and validates the routing contract
// against the registered handlers. It fails when a required handler is missing,
// so an obligation can never be completed by an unregistered consumer.
func newOutboxDispatcher(pool *pgxpool.Pool, logger *slog.Logger, service governanceProjector, enqueuer workflowqueue.ExecutionEnqueuer, external externalqueue.Enqueuer, lifecycle lifecyclequeue.Enqueuer) (*outbox.Dispatcher, error) {
	router, err := routing.NewRouter(service != nil)
	if err != nil {
		return nil, err
	}
	return outbox.NewDispatcher(pool, outbox.Config{
		PollInterval: time.Second,
		ConsumerName: "outbox-dispatcher",
		Logger:       logger,
	}, router, workerHandlerRegistrations(service, enqueuer, external, lifecycle, logger)...)
}
