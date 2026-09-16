package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PublishedEvent struct {
	ID            uuid.UUID
	AggregateType string
	AggregateID   uuid.UUID
	EventType     string
	Payload       json.RawMessage
	Attempts      int
}

type Handler func(context.Context, PublishedEvent) error

type Publisher struct {
	pool         *pgxpool.Pool
	pollInterval time.Duration
	claimTTL     time.Duration
}

func NewPublisher(pool *pgxpool.Pool, pollInterval time.Duration) *Publisher {
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	return &Publisher{
		pool:         pool,
		pollInterval: pollInterval,
		claimTTL:     30 * time.Second,
	}
}

func (p *Publisher) Run(ctx context.Context, handler Handler) error {
	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()

	for {
		if err := p.publishOne(ctx, handler); err != nil && !errors.Is(err, context.Canceled) {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (p *Publisher) publishOne(ctx context.Context, handler Handler) error {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin outbox claim transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var event PublishedEvent
	err = tx.QueryRow(ctx, `
		SELECT id, aggregate_type, aggregate_id, event_type, payload, attempts
		FROM outbox_event
		WHERE status IN ('PENDING', 'FAILED', 'PROCESSING')
		  AND available_at <= now()
		ORDER BY created_at
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`).Scan(
		&event.ID,
		&event.AggregateType,
		&event.AggregateID,
		&event.EventType,
		&event.Payload,
		&event.Attempts,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return tx.Commit(ctx)
	}
	if err != nil {
		return fmt.Errorf("claim outbox event: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE outbox_event
		SET status = 'PROCESSING',
		    attempts = attempts + 1,
		    available_at = $2,
		    last_error = NULL
		WHERE id = $1
	`, event.ID, time.Now().UTC().Add(p.claimTTL))
	if err != nil {
		return fmt.Errorf("mark outbox processing: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit outbox claim: %w", err)
	}

	event.Attempts++
	if err := handler(ctx, event); err != nil {
		_, updateErr := p.pool.Exec(ctx, `
			UPDATE outbox_event
			SET status = 'FAILED',
			    last_error = $2,
			    available_at = now() + interval '5 seconds'
			WHERE id = $1
		`, event.ID, err.Error())
		if updateErr != nil {
			return fmt.Errorf("handler failed: %v; update outbox failure: %w", err, updateErr)
		}
		return nil
	}

	_, err = p.pool.Exec(ctx, `
		UPDATE outbox_event
		SET status = 'PUBLISHED',
		    published_at = now(),
		    available_at = now(),
		    last_error = NULL
		WHERE id = $1
	`, event.ID)
	if err != nil {
		return fmt.Errorf("mark outbox published: %w", err)
	}
	return nil
}
