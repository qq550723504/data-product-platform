package lifecyclequeue

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"testing"
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
