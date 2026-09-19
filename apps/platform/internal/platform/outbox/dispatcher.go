package outbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HandlerRegistration binds a handler name to its implementation. The name is
// the per-handler confirmation identity recorded in outbox_event_consumption.
type HandlerRegistration struct {
	Name   string
	Handle Handler
}

// Dispatcher is the single logical outbox consumer. It claims one event at a
// time and fans it out to every handler the event's frozen obligation requires,
// recording one confirmation per handler. The event is marked PUBLISHED only
// once all required handlers have confirmed.
//
// The obligation is read from the event row, not from whichever routing table
// this process happens to run. An event recorded under one routing version keeps
// that obligation even when a later process starts with a different deployment
// profile: it is never re-interpreted as retention-only. When the frozen
// obligation names a handler this process has not registered, the claim fails
// loudly instead of publishing.
//
// A handler that already confirmed is never invoked again, so a partially
// handled event resumes where it stopped instead of replaying the whole fan-out.
// Confirmations are claim-token guarded: a holder whose lease expired cannot add
// confirmations for the successor's event. The engine is at-least-once: a
// handler that ran but crashed before its confirmation commits runs again, so
// handlers must make their own business side effects idempotent. The
// confirmation ledger is not a business-fact deduplication key.
type Dispatcher struct {
	publisher *Publisher
	router    *Router
	handlers  map[string]Handler
	logger    *slog.Logger
}

// NewDispatcher validates the routing contract against the registered handlers
// and refuses to start when a required handler is missing.
func NewDispatcher(pool *pgxpool.Pool, cfg Config, router *Router, handlers ...HandlerRegistration) (*Dispatcher, error) {
	if router == nil {
		return nil, fmt.Errorf("outbox dispatcher requires a routing table")
	}
	registered := make(map[string]Handler, len(handlers))
	for _, registration := range handlers {
		name := strings.TrimSpace(registration.Name)
		if name == "" {
			return nil, fmt.Errorf("outbox handler name must not be empty")
		}
		if registration.Handle == nil {
			return nil, fmt.Errorf("outbox handler %q has no implementation", name)
		}
		if _, dup := registered[name]; dup {
			return nil, fmt.Errorf("outbox handler %q is registered more than once", name)
		}
		registered[name] = registration.Handle
	}
	if err := router.validate(registered); err != nil {
		return nil, err
	}
	resolved := cfg.withDefaults()
	return &Dispatcher{
		publisher: NewPublisher(pool, cfg),
		router:    router,
		handlers:  registered,
		logger:    resolved.Logger,
	}, nil
}

// RouterVersion identifies the routing contract this dispatcher applies to
// events that do not carry a frozen obligation yet.
func (d *Dispatcher) RouterVersion() string { return d.router.Version() }

// HandlerNames returns the registered handler names in sorted order. It is used
// for startup diagnostics so the deployed handler set is auditable.
func (d *Dispatcher) HandlerNames() []string {
	names := make([]string, 0, len(d.handlers))
	for name := range d.handlers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Run dispatches events until the context is cancelled or a fatal database
// condition is hit. Handler failures and transient database errors are retried
// with backoff and never stop the loop.
func (d *Dispatcher) Run(ctx context.Context) error {
	ticker := time.NewTicker(d.publisher.cfg.PollInterval)
	defer ticker.Stop()

	for {
		err := d.DispatchOnce(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				return ctx.Err()
			}
			if isFatalOutboxError(err) {
				return fmt.Errorf("outbox dispatcher stopped on fatal database condition: %w", err)
			}
			d.logger.Warn("outbox dispatch tick failed; will retry", "error", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// DispatchOnce performs at most one claim + fan-out cycle. It returns nil when
// no event is claimable.
func (d *Dispatcher) DispatchOnce(ctx context.Context) error {
	// The resolver is only consulted for events that were recorded before their
	// obligation was known; it freezes that obligation inside the claim
	// transaction, so two instances started from different profiles cannot
	// disagree about what the event owes.
	c, err := d.publisher.claimOneRouted(ctx, d.router)
	if err != nil {
		return err
	}
	if c == nil {
		return nil
	}

	if c.Event.EventVersion > MaxSupportedEventVersion {
		versionErr := fmt.Errorf(
			"unsupported outbox event version %d (max supported %d); routing version %s cannot parse this payload",
			c.Event.EventVersion, MaxSupportedEventVersion, d.router.Version(),
		)
		return d.publisher.failClaim(ctx, c, versionErr)
	}

	// The obligation comes from the event, never from this process's routing
	// table. An event with no frozen obligation is one whose type is undeclared.
	if c.routingVersion == "" {
		return d.publisher.failClaim(ctx, c, fmt.Errorf(
			"outbox event %s of type %q has no frozen routing obligation and type %q is not declared in routing version %s; refusing to complete it implicitly",
			c.Event.ID, c.Event.EventType, c.Event.EventType, d.router.Version(),
		))
	}
	required := c.requiredHandlers
	if c.routingVersion != d.router.Version() {
		d.logger.Info("dispatching event under a foreign routing version; honoring the frozen obligation",
			"event_id", c.Event.ID,
			"event_type", c.Event.EventType,
			"frozen_routing_version", c.routingVersion,
			"process_routing_version", d.router.Version(),
			"required_handlers", required,
		)
	}

	for _, name := range required {
		if err := ctx.Err(); err != nil {
			// Leave the event PROCESSING; its lease expires and the next claim
			// holder resumes from the persisted confirmations.
			return err
		}
		confirmed, err := c.handlerConfirmed(ctx, name)
		if err != nil {
			return err
		}
		if confirmed {
			continue
		}
		handle, ok := d.handlers[name]
		if !ok {
			// The frozen obligation outranks this process's handler set. Fail
			// the claim instead of publishing an unpaid obligation.
			return d.publisher.failClaim(ctx, c, fmt.Errorf(
				"outbox event %s obligation (routing version %s) requires handler %q, which this process has not registered",
				c.Event.ID, c.routingVersion, name,
			))
		}
		if err := handle(ctx, c.Event); err != nil {
			return d.publisher.failClaim(ctx, c, fmt.Errorf("outbox handler %s failed: %w", name, err))
		}
		recorded, err := c.confirmHandler(ctx, name)
		if err != nil {
			return err
		}
		if !recorded {
			d.logger.Warn("outbox dispatch lease lost before handler confirmation; leaving the event to the current claim holder",
				"event_id", c.Event.ID,
				"event_type", c.Event.EventType,
				"handler", name,
				"routing_version", c.routingVersion,
			)
			return nil
		}
	}

	published, err := c.publish(ctx, required)
	if err != nil {
		return err
	}
	if !published {
		d.logger.Warn("outbox dispatch lease lost before publish; leaving the event to the current claim holder",
			"event_id", c.Event.ID,
			"event_type", c.Event.EventType,
			"routing_version", c.routingVersion,
		)
		return nil
	}
	d.logger.Info("outbox event dispatched to all required handlers",
		"event_id", c.Event.ID,
		"event_type", c.Event.EventType,
		"aggregate_type", c.Event.AggregateType,
		"aggregate_id", c.Event.AggregateID,
		"handlers", required,
		"routing_version", c.routingVersion,
	)
	return nil
}

// handlerConfirmed reports whether this handler already recorded a confirmation
// for the event. Confirmations survive partial dispatch failures, so a retry
// only runs the handlers that have not confirmed yet.
func (c *claim) handlerConfirmed(ctx context.Context, handlerName string) (bool, error) {
	var confirmed bool
	err := c.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM outbox_event_consumption
			WHERE consumer_name = $1 AND event_id = $2
		)
	`, handlerName, c.Event.ID).Scan(&confirmed)
	if err != nil {
		return false, fmt.Errorf("check outbox handler confirmation: %w", err)
	}
	return confirmed, nil
}

// confirmHandler records one handler's confirmation. It is conditional on the
// claim token, so a holder whose lease expired cannot add confirmations for a
// successor's event.
func (c *claim) confirmHandler(ctx context.Context, handlerName string) (bool, error) {
	tx, err := c.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, fmt.Errorf("begin outbox confirmation transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	tag, err := tx.Exec(ctx, `
		INSERT INTO outbox_event_consumption (consumer_name, event_id)
		SELECT $1, e.id
		FROM outbox_event e
		WHERE e.id = $2
		  AND e.status = $3
		  AND e.claim_token = $4
		ON CONFLICT (consumer_name, event_id) DO NOTHING
	`, handlerName, c.Event.ID, statusProcessing, c.token)
	if err != nil {
		return false, fmt.Errorf("record outbox handler confirmation: %w", err)
	}
	if tag.RowsAffected() != 1 {
		// Either the lease was lost or the confirmation already exists. Do not
		// mistake an already-confirmed handler for a lost lease.
		confirmed, checkErr := c.handlerConfirmed(ctx, handlerName)
		if checkErr != nil {
			return false, checkErr
		}
		if !confirmed {
			return false, nil
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit outbox handler confirmation: %w", err)
	}
	return true, nil
}

// publish marks the event PUBLISHED once every required handler has confirmed.
// The write is conditional on the claim token, and it re-checks the
// confirmations inside the same transaction so a successor's confirmations can
// never be attributed to this claim.
func (c *claim) publish(ctx context.Context, required []string) (bool, error) {
	tx, err := c.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, fmt.Errorf("begin outbox publish transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if len(required) > 0 {
		var confirmed int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM outbox_event_consumption
			WHERE event_id = $1 AND consumer_name = ANY($2)
		`, c.Event.ID, required).Scan(&confirmed); err != nil {
			return false, fmt.Errorf("verify outbox handler confirmations: %w", err)
		}
		if confirmed != len(required) {
			return false, nil
		}
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
		return false, fmt.Errorf("commit outbox publish: %w", err)
	}
	return true, nil
}
