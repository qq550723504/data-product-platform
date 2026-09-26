package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MaxSupportedEventVersion is the highest payload structure version this binary
// understands. Events with a higher version fail dispatch with a diagnosable
// error (and eventually dead-letter) instead of being silently skipped or
// misparsed under a newer envelope.
const MaxSupportedEventVersion = 1

const (
	statusPending    = "PENDING"
	statusProcessing = "PROCESSING"
	statusPublished  = "PUBLISHED"
	statusFailed     = "FAILED"
	statusDeadLetter = "DEAD_LETTER"
)

type PublishedEvent struct {
	ID            uuid.UUID
	AggregateType string
	AggregateID   uuid.UUID
	EventType     string
	EventVersion  int
	Payload       json.RawMessage
	Attempts      int
}

type Handler func(context.Context, PublishedEvent) error

// Config controls dispatch timing and ownership.
type Config struct {
	// PollInterval is how often the publisher looks for claimable events.
	PollInterval time.Duration
	// ClaimTTL is the dispatch lease. A claim older than its lease may be
	// taken over by another publisher; the old holder's token stops working.
	ClaimTTL time.Duration
	// FailureBase is the first retry delay; it doubles per attempt up to
	// FailureCap, plus jitter.
	FailureBase time.Duration
	FailureCap  time.Duration
	// MaxAttempts is the number of dispatch attempts after which a repeatedly
	// failing event enters DEAD_LETTER. Dead letter means "automatic dispatch
	// stopped, an operator must look", not "the business action failed".
	MaxAttempts int
	// ConsumerName identifies this dispatcher in outbox_event_consumption.
	// Dispatch confirmations are per consumer: one consumer finishing does not
	// imply any other consumer finished.
	ConsumerName string
	Logger       *slog.Logger
}

func (c Config) withDefaults() Config {
	if c.PollInterval <= 0 {
		c.PollInterval = time.Second
	}
	if c.ClaimTTL <= 0 {
		c.ClaimTTL = 30 * time.Second
	}
	if c.FailureBase <= 0 {
		c.FailureBase = 5 * time.Second
	}
	if c.FailureCap <= 0 {
		c.FailureCap = 5 * time.Minute
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 12
	}
	if c.ConsumerName == "" {
		c.ConsumerName = "default"
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	return c
}

// nextFailureDelay computes the retry delay after a failed attempt. attempts is
// the number of dispatch attempts already made (>= 1).
func nextFailureDelay(attempts int, cfg Config) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	delay := cfg.FailureBase
	for i := 1; i < attempts; i++ {
		delay *= 2
		if delay >= cfg.FailureCap {
			return cfg.FailureCap
		}
	}
	if delay > cfg.FailureCap {
		delay = cfg.FailureCap
	}
	if jitter := cfg.FailureBase / 4; jitter > 0 {
		delay += time.Duration(rand.Int64N(int64(jitter)))
	}
	return delay
}

type Publisher struct {
	pool *pgxpool.Pool
	cfg  Config
}

func NewPublisher(pool *pgxpool.Pool, cfg Config) *Publisher {
	return &Publisher{pool: pool, cfg: cfg.withDefaults()}
}

// claim is one held dispatch lease. token must accompany every terminal write;
// a claim whose lease was taken over can no longer change the event state.
type claim struct {
	Event       PublishedEvent
	token       uuid.UUID
	pool        *pgxpool.Pool
	cfg         Config
	attemptBase int

	// routingVersion and requiredHandlers are the event's frozen obligation.
	routingVersion   string
	requiredHandlers []string
}

// Run dispatches events until the context is cancelled or a fatal database
// condition (missing table, missing privilege, incompatible schema) is hit.
// Handler failures and transient database errors are retried with backoff and
// never stop the loop: a poison event dead-letters, and a flapping connection
// must not take the worker down.
func (p *Publisher) Run(ctx context.Context, handler Handler) error {
	ticker := time.NewTicker(p.cfg.PollInterval)
	defer ticker.Stop()

	for {
		err := p.publishOnce(ctx, handler)
		if err != nil {
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				return ctx.Err()
			}
			if isFatalOutboxError(err) {
				return fmt.Errorf("outbox publisher stopped on fatal database condition: %w", err)
			}
			p.cfg.Logger.Warn("outbox dispatch tick failed; will retry", "error", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// publishOnce performs at most one claim + dispatch cycle. It returns
// (nil, nil) when no event is claimable.
func (p *Publisher) publishOnce(ctx context.Context, handler Handler) error {
	c, err := p.claimOne(ctx)
	if err != nil {
		return err
	}
	if c == nil {
		return nil
	}

	if c.Event.EventVersion > MaxSupportedEventVersion {
		versionErr := fmt.Errorf(
			"unsupported outbox event version %d (max supported %d); consumer %s cannot parse this payload",
			c.Event.EventVersion, MaxSupportedEventVersion, p.cfg.ConsumerName,
		)
		if err := p.failClaim(ctx, c, versionErr); err != nil {
			return err
		}
		return nil
	}

	if err := handler(ctx, c.Event); err != nil {
		if failErr := p.failClaim(ctx, c, err); failErr != nil {
			return failErr
		}
		return nil
	}

	ok, err := p.completeClaim(ctx, c)
	if err != nil {
		return err
	}
	if !ok {
		p.cfg.Logger.Warn("outbox dispatch lease lost before completion; result left to the current claim holder",
			"event_id", c.Event.ID,
			"event_type", c.Event.EventType,
			"consumer", p.cfg.ConsumerName,
		)
	}
	return nil
}

func (p *Publisher) claimOne(ctx context.Context) (*claim, error) {
	return p.claimOneRouted(ctx)
}

// claimOneRouted claims the next event together with the routing obligation
// that was frozen when the event was appended.
func (p *Publisher) claimOneRouted(ctx context.Context) (*claim, error) {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin outbox claim transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	token := uuid.New()
	now := time.Now().UTC()
	var (
		event            PublishedEvent
		version          int16
		routingVersion   string
		requiredHandlers []string
		attemptBase      int
	)
	err = tx.QueryRow(ctx, `
		UPDATE outbox_event
		SET status = $1,
		    attempts = attempts + 1,
		    available_at = $2,
		    claim_token = $3,
		    claimed_by = $4,
		    claimed_at = now()
		WHERE id = (
			SELECT id FROM outbox_event
			WHERE status IN ($1, $5, $6)
			  AND available_at <= now()
			ORDER BY created_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		RETURNING id, aggregate_type, aggregate_id, event_type, event_version, payload, attempts,
		          routing_version, required_handlers, attempt_base
	`, statusProcessing, now.Add(p.cfg.ClaimTTL), token, p.cfg.ConsumerName,
		statusPending, statusFailed).Scan(
		&event.ID,
		&event.AggregateType,
		&event.AggregateID,
		&event.EventType,
		&version,
		&event.Payload,
		&event.Attempts,
		&routingVersion,
		&requiredHandlers,
		&attemptBase,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim outbox event: %w", err)
	}
	event.EventVersion = int(version)

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit outbox claim: %w", err)
	}

	return &claim{
		Event:            event,
		token:            token,
		pool:             p.pool,
		cfg:              p.cfg,
		attemptBase:      attemptBase,
		routingVersion:   routingVersion,
		requiredHandlers: requiredHandlers,
	}, nil
}

// completeClaim marks the event PUBLISHED and records the consumer dispatch
// confirmation in one transaction, both conditional on the claim token. A lost
// lease rolls the whole transaction back: the current holder owns the outcome.
func (p *Publisher) completeClaim(ctx context.Context, c *claim) (bool, error) {
	tx, err := c.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, fmt.Errorf("begin outbox completion transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO outbox_event_consumption (consumer_name, event_id)
		VALUES ($1, $2)
		ON CONFLICT (consumer_name, event_id) DO NOTHING
	`, c.cfg.ConsumerName, c.Event.ID); err != nil {
		return false, fmt.Errorf("record outbox consumption: %w", err)
	}

	tag, err := tx.Exec(ctx, `
		UPDATE outbox_event
		SET status = $2,
		    published_at = now(),
		    available_at = now(),
		    last_error = NULL
		WHERE id = $1
		  AND status = $3
		  AND claim_token = $4
	`, c.Event.ID, statusPublished, statusProcessing, c.token)
	if err != nil {
		return false, fmt.Errorf("mark outbox published: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return false, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit outbox completion: %w", err)
	}
	return true, nil
}

// failClaim records a handler failure against the held lease. Repeatedly
// failing events dead-letter after MaxAttempts instead of retrying forever.
func (p *Publisher) failClaim(ctx context.Context, c *claim, cause error) error {
	outcome, err := c.Fail(ctx, cause)
	if err != nil {
		return err
	}
	switch outcome {
	case deadLetterOutcome:
		p.cfg.Logger.Error("outbox event dead-lettered after repeated dispatch failures",
			"event_id", c.Event.ID,
			"event_type", c.Event.EventType,
			"aggregate_type", c.Event.AggregateType,
			"aggregate_id", c.Event.AggregateID,
			"attempts", c.Event.Attempts,
			"consumer", c.cfg.ConsumerName,
			"error", cause.Error(),
		)
	case lostOutcome:
		p.cfg.Logger.Warn("outbox dispatch lease lost before failure could be recorded",
			"event_id", c.Event.ID,
			"consumer", c.cfg.ConsumerName,
		)
	default:
		p.cfg.Logger.Warn("outbox event dispatch failed; scheduled for retry",
			"event_id", c.Event.ID,
			"event_type", c.Event.EventType,
			"attempts", c.Event.Attempts,
			"consumer", c.cfg.ConsumerName,
			"error", cause.Error(),
		)
	}
	return nil
}

const (
	failedOutcome     = "failed"
	deadLetterOutcome = "dead-lettered"
	lostOutcome       = "lost"
)

func failureDelayForClaim(c *claim) time.Duration {
	return nextFailureDelay(c.Event.Attempts-c.attemptBase, c.cfg)
}

// Fail conditionally records a dispatch failure. It returns which outcome was
// applied: failed (retry scheduled), dead-lettered, or lost (another claim
// holder owns the event now).
func (c *claim) Fail(ctx context.Context, cause error) (string, error) {
	if cause == nil {
		cause = errors.New("outbox dispatch failed")
	}
	generationAttempts := c.Event.Attempts - c.attemptBase
	delay := failureDelayForClaim(c)
	status := statusFailed
	deadLettered := false
	if generationAttempts >= c.cfg.MaxAttempts {
		status = statusDeadLetter
		deadLettered = true
		delay = 0
	}

	tag, err := c.pool.Exec(ctx, `
		UPDATE outbox_event
		SET status = $2,
		    last_error = $3,
		    available_at = CASE WHEN $5 THEN now() ELSE now() + make_interval(secs => $4) END,
		    dead_lettered_at = CASE WHEN $5 THEN now() ELSE dead_lettered_at END,
		    claim_token = NULL
		WHERE id = $1
		  AND status = $6
		  AND claim_token = $7
	`, c.Event.ID, status, cause.Error(), delay.Seconds(), deadLettered, statusProcessing, c.token)
	if err != nil {
		return "", fmt.Errorf("record outbox dispatch failure: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return lostOutcome, nil
	}
	if deadLettered {
		return deadLetterOutcome, nil
	}
	return failedOutcome, nil
}

// isFatalOutboxError reports whether the error describes a database condition
// that retrying cannot fix: a missing table or column, insufficient privilege,
// or an invalid catalog/schema name. Those must surface as a publisher failure
// instead of looping forever while pretending to make progress.
func isFatalOutboxError(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	if len(pgErr.Code) < 2 {
		return false
	}
	switch pgErr.Code[:2] {
	case "42", // syntax or access rule violation: undefined table/column, insufficient privilege
		"28", // invalid authorization specification
		"3D", // invalid catalog name
		"3F": // invalid schema name
		return true
	}
	return false
}
