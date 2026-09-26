package outbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
)

const requeueCommandType = "OUTBOX.REQUEUE_EVENT"

var (
	ErrReplayInvalidCommand      = errors.New("invalid outbox replay command")
	ErrReplayEventNotFound       = errors.New("outbox event not found")
	ErrReplayNotDeadLetter       = errors.New("outbox event is not dead-lettered")
	ErrReplayIdempotencyConflict = errors.New("outbox replay idempotency conflict")
	ErrReplayUnsupportedVersion  = errors.New("outbox event version is not supported")
)

type RequeueOutboxEventCommand struct {
	EventID        uuid.UUID
	IdempotencyKey string
	ActorID        uuid.UUID
	Reason         string
	TraceID        string
}

type ReplayFact struct {
	ID                     uuid.UUID
	EventID                uuid.UUID
	IdempotencyKey         string
	RequestFingerprint     string
	ActorID                uuid.UUID
	Reason                 string
	PreviousAttempts       int
	PreviousLastError      string
	PreviousDeadLetteredAt time.Time
	CreatedAt              time.Time
}

type RequeueService struct {
	pool *pgxpool.Pool
}

func NewRequeueService(pool *pgxpool.Pool) *RequeueService {
	return &RequeueService{pool: pool}
}

func (s *RequeueService) Requeue(ctx context.Context, cmd RequeueOutboxEventCommand) (ReplayFact, error) {
	if s == nil || s.pool == nil {
		return ReplayFact{}, ErrReplayInvalidCommand
	}
	cmd.IdempotencyKey = strings.TrimSpace(cmd.IdempotencyKey)
	cmd.Reason = strings.TrimSpace(cmd.Reason)
	if cmd.EventID == uuid.Nil || cmd.ActorID == uuid.Nil || cmd.IdempotencyKey == "" || cmd.Reason == "" {
		return ReplayFact{}, ErrReplayInvalidCommand
	}
	fingerprint, err := replayFingerprint(cmd)
	if err != nil {
		return ReplayFact{}, err
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ReplayFact{}, fmt.Errorf("begin outbox replay transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var (
		status         string
		eventVersion   int16
		attempts       int
		lastError      *string
		deadLetteredAt *time.Time
	)
	err = tx.QueryRow(ctx, "SELECT status, event_version, attempts, last_error, dead_lettered_at FROM outbox_event WHERE id=$1 FOR UPDATE", cmd.EventID).Scan(
		&status, &eventVersion, &attempts, &lastError, &deadLetteredAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReplayFact{}, ErrReplayEventNotFound
	}
	if err != nil {
		return ReplayFact{}, fmt.Errorf("lock dead-lettered outbox event: %w", err)
	}

	// Serialize every replay intent on the original event first. A concurrent
	// same-key request waits here, then observes the replay fact committed by
	// the winner and returns it instead of misclassifying the now-PENDING event.
	if existing, found, err := findReplayTx(ctx, tx, cmd.EventID, cmd.IdempotencyKey); err != nil {
		return ReplayFact{}, err
	} else if found {
		if existing.RequestFingerprint != fingerprint {
			return ReplayFact{}, ErrReplayIdempotencyConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return ReplayFact{}, fmt.Errorf("commit outbox replay idempotent read: %w", err)
		}
		return existing, nil
	}

	if int(eventVersion) > MaxSupportedEventVersion {
		return ReplayFact{}, fmt.Errorf("%w: event version %d exceeds max supported %d", ErrReplayUnsupportedVersion, eventVersion, MaxSupportedEventVersion)
	}
	if status != statusDeadLetter || deadLetteredAt == nil {
		return ReplayFact{}, ErrReplayNotDeadLetter
	}

	now := time.Now().UTC()
	fact := ReplayFact{
		ID:                     uuid.New(),
		EventID:                cmd.EventID,
		IdempotencyKey:         cmd.IdempotencyKey,
		RequestFingerprint:     fingerprint,
		ActorID:                cmd.ActorID,
		Reason:                 cmd.Reason,
		PreviousAttempts:       attempts,
		PreviousDeadLetteredAt: deadLetteredAt.UTC(),
		CreatedAt:              now,
	}
	if lastError != nil {
		fact.PreviousLastError = *lastError
	}

	_, err = tx.Exec(ctx, "INSERT INTO outbox_event_replay (id, event_id, idempotency_key, request_fingerprint, actor_id, reason, previous_attempts, previous_last_error, previous_dead_lettered_at, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)",
		fact.ID, fact.EventID, fact.IdempotencyKey, fact.RequestFingerprint, fact.ActorID, fact.Reason,
		fact.PreviousAttempts, nullableReplayString(fact.PreviousLastError), fact.PreviousDeadLetteredAt, fact.CreatedAt,
	)
	if err != nil {
		return ReplayFact{}, fmt.Errorf("insert outbox replay fact: %w", err)
	}

	tag, err := tx.Exec(ctx, "UPDATE outbox_event SET status=$2, available_at=$3, attempt_base=attempts, claim_token=NULL, claimed_by=NULL, claimed_at=NULL WHERE id=$1 AND status=$4",
		cmd.EventID, statusPending, now, statusDeadLetter,
	)
	if err != nil {
		return ReplayFact{}, fmt.Errorf("requeue dead-lettered outbox event: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ReplayFact{}, ErrReplayNotDeadLetter
	}

	if err := audit.Append(ctx, tx, audit.Event{
		ActorType:  "USER",
		ActorID:    &cmd.ActorID,
		Action:     "OUTBOX_EVENT_REQUEUED",
		ObjectType: "OUTBOX_EVENT",
		ObjectID:   cmd.EventID,
		BeforeState: map[string]any{
			"status":         statusDeadLetter,
			"attempts":       attempts,
			"lastError":      fact.PreviousLastError,
			"deadLetteredAt": fact.PreviousDeadLetteredAt,
		},
		AfterState: map[string]any{
			"status":      statusPending,
			"attemptBase": attempts,
			"replayId":    fact.ID,
		},
		Reason:  cmd.Reason,
		TraceID: cmd.TraceID,
		Metadata: map[string]any{
			"commandType":    requeueCommandType,
			"idempotencyKey": cmd.IdempotencyKey,
			"replayId":       fact.ID,
		},
		OccurredAt: now,
	}); err != nil {
		return ReplayFact{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return ReplayFact{}, fmt.Errorf("commit outbox replay: %w", err)
	}
	return fact, nil
}

func findReplayTx(ctx context.Context, tx pgx.Tx, eventID uuid.UUID, key string) (ReplayFact, bool, error) {
	var fact ReplayFact
	var lastError *string
	err := tx.QueryRow(ctx, "SELECT id, event_id, idempotency_key, request_fingerprint, actor_id, reason, previous_attempts, previous_last_error, previous_dead_lettered_at, created_at FROM outbox_event_replay WHERE event_id=$1 AND idempotency_key=$2 FOR UPDATE", eventID, key).Scan(
		&fact.ID, &fact.EventID, &fact.IdempotencyKey, &fact.RequestFingerprint, &fact.ActorID, &fact.Reason,
		&fact.PreviousAttempts, &lastError, &fact.PreviousDeadLetteredAt, &fact.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReplayFact{}, false, nil
	}
	if err != nil {
		return ReplayFact{}, false, fmt.Errorf("find outbox replay fact: %w", err)
	}
	if lastError != nil {
		fact.PreviousLastError = *lastError
	}
	return fact, true, nil
}

func replayFingerprint(cmd RequeueOutboxEventCommand) (string, error) {
	payload, err := json.Marshal(struct {
		EventID uuid.UUID `json:"eventId"`
		ActorID uuid.UUID `json:"actorId"`
		Reason  string    `json:"reason"`
	}{
		EventID: cmd.EventID,
		ActorID: cmd.ActorID,
		Reason:  strings.TrimSpace(cmd.Reason),
	})
	if err != nil {
		return "", fmt.Errorf("marshal outbox replay fingerprint: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func nullableReplayString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
