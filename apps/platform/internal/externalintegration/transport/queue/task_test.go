package externalqueue

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

type runner struct {
	calls atomic.Int32
	done  chan struct{}
}

func (r *runner) RunPublication(context.Context, uuid.UUID, uuid.UUID) error {
	r.calls.Add(1)
	if r.done != nil {
		r.done <- struct{}{}
	}
	return nil
}

type enqueuer struct {
	err            error
	ws, pub, event uuid.UUID
}

func (e *enqueuer) EnqueuePublication(_ context.Context, ws, pub, event uuid.UUID) error {
	e.ws, e.pub, e.event = ws, pub, event
	return e.err
}
func TestStrictTaskAndOutboxObligation(t *testing.T) {
	r := &runner{}
	for _, body := range []string{"{}", `{"workspaceId":"bad","publicationId":"bad"}`, `{"workspaceId":"` + uuid.NewString() + `","publicationId":"` + uuid.NewString() + `","status":"PUBLISHED"}`} {
		if err := Handle(r)(context.Background(), asynq.NewTask(TaskExecute, []byte(body))); !errors.Is(err, d.ErrInvalid) {
			t.Fatalf("invalid task: %v", err)
		}
	}
	ws, pub, id := uuid.New(), uuid.New(), uuid.New()
	raw, _ := json.Marshal(map[string]any{"workspaceId": ws, "objectId": pub})
	sentinel := errors.New("queue unavailable")
	e := &enqueuer{err: sentinel}
	event := outbox.PublishedEvent{ID: id, AggregateType: "ExternalPublication", AggregateID: pub, EventType: "ExternalPublicationExecutionRequested", EventVersion: 1, Payload: raw}
	if err := OutboxHandler(e)(context.Background(), event); !errors.Is(err, sentinel) || e.ws != ws || e.pub != pub || e.event != id {
		t.Fatal("queue failure or exact identifiers lost")
	}
	event.EventType = "ExternalPublicationRequested"
	if err := OutboxHandler(e)(context.Background(), event); !errors.Is(err, d.ErrInvalid) {
		t.Fatal("old retention event was reinterpreted")
	}
}
func TestRedisDuplicateWakeupUsesOneTask(t *testing.T) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		t.Skip("REDIS_ADDR not set")
	}
	// DB 15 is reserved for this synthetic contract; no flush/delete of others.
	opt := asynq.RedisClientOpt{Addr: addr, Password: os.Getenv("REDIS_PASSWORD"), DB: 15}
	client := asynq.NewClient(opt)
	defer client.Close()
	c := NewClient(client)
	ws, pub, id := uuid.New(), uuid.New(), uuid.New()
	for range 2 {
		if err := c.EnqueuePublication(context.Background(), ws, pub, id); err != nil {
			t.Fatal(err)
		}
	}
	inspector := asynq.NewInspector(opt)
	defer inspector.Close()
	info, err := inspector.GetTaskInfo("default", "external/"+id.String())
	if err != nil || info.State != asynq.TaskStatePending {
		t.Fatalf("queued task: %+v %v", info, err)
	}
	r := &runner{done: make(chan struct{}, 2)}
	server := asynq.NewServer(opt, asynq.Config{Concurrency: 1, ShutdownTimeout: time.Second, LogLevel: asynq.ErrorLevel})
	mux := asynq.NewServeMux()
	mux.HandleFunc(TaskExecute, Handle(r))
	if err = server.Start(mux); err != nil {
		t.Fatal(err)
	}
	defer server.Shutdown()
	select {
	case <-r.done:
	case <-time.After(8 * time.Second):
		t.Fatal("queue did not consume wakeup")
	}
	// The queue confirms delivery only. PostgreSQL ownership supplies business dedup.
	time.Sleep(100 * time.Millisecond)
	if r.calls.Load() != 1 {
		t.Fatal("duplicate same-event task")
	}
}
