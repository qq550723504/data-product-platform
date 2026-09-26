package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
)

func main() {
	eventIDRaw := flag.String("event-id", "", "DEAD_LETTER outbox event UUID")
	actorIDRaw := flag.String("actor-id", "", "operator actor UUID")
	idempotencyKey := flag.String("idempotency-key", "", "stable replay request identity")
	reason := flag.String("reason", "", "operator reason for replay")
	traceID := flag.String("trace-id", "", "optional incident/request trace ID")
	flag.Parse()

	dsn := strings.TrimSpace(os.Getenv("POSTGRES_DSN"))
	if dsn == "" {
		fail("POSTGRES_DSN is required")
	}
	eventID, err := uuid.Parse(strings.TrimSpace(*eventIDRaw))
	if err != nil || eventID == uuid.Nil {
		fail("event-id must be a non-nil UUID")
	}
	actorID, err := uuid.Parse(strings.TrimSpace(*actorIDRaw))
	if err != nil || actorID == uuid.Nil {
		fail("actor-id must be a non-nil UUID")
	}
	if strings.TrimSpace(*idempotencyKey) == "" {
		fail("idempotency-key is required")
	}
	if strings.TrimSpace(*reason) == "" {
		fail("reason is required")
	}

	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		fail("open postgres: %v", err)
	}
	defer db.Close()

	fact, err := outbox.NewRequeueService(db).Requeue(ctx, outbox.RequeueOutboxEventCommand{
		EventID:        eventID,
		IdempotencyKey: strings.TrimSpace(*idempotencyKey),
		ActorID:        actorID,
		Reason:         strings.TrimSpace(*reason),
		TraceID:        strings.TrimSpace(*traceID),
	})
	if err != nil {
		fail("requeue outbox event: %v", err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(fact); err != nil {
		fail("encode replay fact: %v", err)
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
