package lifecyclequeue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
)

const TaskRecover = "lifecycle:disposition:reconcile"

type Enqueuer interface {
	EnqueueRecovery(context.Context, uuid.UUID, uuid.UUID) error
}
type Runner interface {
	RunRecovery(context.Context, uuid.UUID, uuid.UUID) error
}
type Client struct{ client *asynq.Client }

func NewClient(c *asynq.Client) *Client { return &Client{c} }

type payload struct {
	WorkspaceID uuid.UUID `json:"workspaceId"`
	WorkID      uuid.UUID `json:"workId"`
}

func (c *Client) EnqueueRecovery(ctx context.Context, w, id uuid.UUID) error {
	if c == nil || c.client == nil || w == uuid.Nil || id == uuid.Nil {
		return lc.ErrStorageScope
	}
	b, err := json.Marshal(payload{w, id})
	if err != nil {
		return err
	}
	_, err = c.client.EnqueueContext(ctx, asynq.NewTask(TaskRecover, b), asynq.TaskID("lifecycle/recovery/"+id.String()), asynq.MaxRetry(lc.MaxRecoveryRuns-1), asynq.Timeout(30*time.Second))
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil
	}
	return err
}
func Handle(r Runner) asynq.HandlerFunc {
	return func(ctx context.Context, t *asynq.Task) error {
		if t.Type() != TaskRecover || len(t.Payload()) > 1024 {
			return lc.ErrStorageScope
		}
		var p payload
		d := json.NewDecoder(bytes.NewReader(t.Payload()))
		d.DisallowUnknownFields()
		if d.Decode(&p) != nil || p.WorkspaceID == uuid.Nil || p.WorkID == uuid.Nil {
			return lc.ErrStorageScope
		}
		var tail any
		if d.Decode(&tail) != io.EOF {
			return lc.ErrStorageScope
		}
		if r == nil {
			return lc.ErrStorageUnsupported
		}
		return r.RunRecovery(ctx, p.WorkspaceID, p.WorkID)
	}
}
func OutboxHandler(q Enqueuer) outbox.Handler {
	return func(ctx context.Context, e outbox.PublishedEvent) error {
		if q == nil || e.ID == uuid.Nil || e.AggregateID == uuid.Nil || e.EventType != lc.ReconciliationRequestedEvent || e.EventVersion != 1 || e.AggregateType != "LIFECYCLE_RECONCILIATION_WORK" {
			return lc.ErrStorageScope
		}
		var p struct {
			WorkspaceID uuid.UUID `json:"workspace_id"`
			FactID      uuid.UUID `json:"fact_id"`
		}
		if json.Unmarshal(e.Payload, &p) != nil || p.WorkspaceID == uuid.Nil || p.FactID != e.AggregateID {
			return lc.ErrStorageScope
		}
		return q.EnqueueRecovery(ctx, p.WorkspaceID, p.FactID)
	}
}
