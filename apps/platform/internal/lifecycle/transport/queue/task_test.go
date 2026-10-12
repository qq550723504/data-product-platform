package lifecyclequeue

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"os"
	"testing"
	"time"
)

type probe struct {
	calls int
	w, id uuid.UUID
}

func (p *probe) RunRecovery(_ context.Context, w, id uuid.UUID) error {
	p.calls++
	p.w = w
	p.id = id
	return nil
}
func (p *probe) EnqueueRecovery(ctx context.Context, w, id uuid.UUID) error {
	return p.RunRecovery(ctx, w, id)
}
func TestRecoveryEnvelopeCannotSupplyAuthorityOrStorageTarget(t *testing.T) {
	w, id := uuid.New(), uuid.New()
	raw, _ := json.Marshal(payload{w, id})
	p := &probe{}
	h := Handle(p)
	for _, b := range [][]byte{append(raw, []byte(` {}`)...), []byte(`{"workspaceId":"` + w.String() + `","workId":"` + id.String() + `","delete":true}`), []byte(`{"workspaceId":"` + w.String() + `","workId":"` + id.String() + `","consumer":"admin"}`)} {
		if h(context.Background(), asynq.NewTask(TaskRecover, b)) == nil {
			t.Fatal("untrusted envelope accepted")
		}
	}
	if p.calls != 0 {
		t.Fatal("invalid input reached service")
	}
	if err := h(context.Background(), asynq.NewTask(TaskRecover, raw)); err != nil || p.w != w || p.id != id {
		t.Fatal(err)
	}
	if err := Handle(nil)(context.Background(), asynq.NewTask(TaskRecover, raw)); err != lc.ErrStorageUnsupported {
		t.Fatal("unconfigured handler acknowledged")
	}
}
func TestRecoveryOutboxRequiresExactFrozenWorkIdentity(t *testing.T) {
	w, id := uuid.New(), uuid.New()
	p := &probe{}
	b, _ := json.Marshal(map[string]any{"workspace_id": w, "fact_id": id})
	e := outbox.PublishedEvent{ID: uuid.New(), AggregateID: id, AggregateType: "LIFECYCLE_RECONCILIATION_WORK", EventType: lc.ReconciliationRequestedEvent, EventVersion: 1, Payload: b}
	if err := OutboxHandler(p)(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	e.AggregateID = uuid.New()
	if OutboxHandler(p)(context.Background(), e) == nil || p.calls != 1 {
		t.Fatal("payload redirected work")
	}
}

func TestRedisRecoveryDeadlineCoversFullBoundedLookupSequence(t *testing.T) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		t.Skip("REDIS_ADDR not set; real queue policy NOT_RUN")
	}
	// Reuse Asynq's real persisted task policy, with isolated synthetic task IDs.
	// Do not flush or delete any queue data; duplicate wakeups retain one task.
	opt := asynq.RedisClientOpt{Addr: addr, Password: os.Getenv("REDIS_PASSWORD"), DB: 15}
	client := asynq.NewClient(opt)
	defer client.Close()
	c := NewClient(client)
	w, id := uuid.New(), uuid.New()
	for range 2 {
		if err := c.EnqueueRecovery(context.Background(), w, id); err != nil {
			t.Fatal(err)
		}
	}
	inspector := asynq.NewInspector(opt)
	defer inspector.Close()
	info, err := inspector.GetTaskInfo("default", "lifecycle/recovery/"+id.String())
	if err != nil {
		t.Fatal(err)
	}
	// Inventory+exact HEAD, source/target retention+hold, and byte verification.
	// Include their durable observation budget rather than comparing a literal
	// configuration value: shortening a task below this sequence must fail.
	const fullLookupBudget = 7 * (30 + 5) * time.Second
	if info.Timeout <= fullLookupBudget || info.Timeout > 10*time.Minute || info.MaxRetry != lc.MaxRecoveryRuns-1 || info.State != asynq.TaskStatePending {
		t.Fatalf("recovery task cannot finish its bounded sequence: %+v", info)
	}
	var p payload
	if err := json.Unmarshal(info.Payload, &p); err != nil || p.WorkspaceID != w || p.WorkID != id {
		t.Fatal("persisted task changed exact work scope", p, err)
	}
}
