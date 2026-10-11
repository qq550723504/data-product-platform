package externalqueue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"io"
)

const TaskExecute = "external:publication:run"

type Enqueuer interface {
	EnqueuePublication(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error
}
type Client struct{ client *asynq.Client }

func NewClient(c *asynq.Client) *Client { return &Client{client: c} }

type payload struct {
	WorkspaceID   uuid.UUID `json:"workspaceId"`
	PublicationID uuid.UUID `json:"publicationId"`
}

func (c *Client) EnqueuePublication(ctx context.Context, ws, pub, event uuid.UUID) error {
	if ws == uuid.Nil || pub == uuid.Nil || event == uuid.Nil {
		return d.ErrInvalid
	}
	raw, err := json.Marshal(payload{ws, pub})
	if err != nil {
		return err
	}
	_, err = c.client.EnqueueContext(ctx, asynq.NewTask(TaskExecute, raw), asynq.TaskID("external/"+event.String()), asynq.MaxRetry(5))
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil
	}
	return err
}

type Runner interface {
	RunPublication(context.Context, uuid.UUID, uuid.UUID) error
}

func Handle(r Runner) asynq.HandlerFunc {
	return func(ctx context.Context, task *asynq.Task) error {
		if task.Type() != TaskExecute || len(task.Payload()) > 1024 {
			return d.ErrInvalid
		}
		var p payload
		dec := json.NewDecoder(bytes.NewReader(task.Payload()))
		dec.DisallowUnknownFields()
		if dec.Decode(&p) != nil || p.WorkspaceID == uuid.Nil || p.PublicationID == uuid.Nil {
			return d.ErrInvalid
		}
		var tail any
		if dec.Decode(&tail) != io.EOF {
			return d.ErrInvalid
		}
		return r.RunPublication(ctx, p.WorkspaceID, p.PublicationID)
	}
}
func OutboxHandler(c Enqueuer) outbox.Handler {
	return func(ctx context.Context, event outbox.PublishedEvent) error {
		if event.EventType != "ExternalPublicationExecutionRequested" || event.EventVersion != 1 || event.AggregateType != "ExternalPublication" || event.ID == uuid.Nil || event.AggregateID == uuid.Nil {
			return d.ErrInvalid
		}
		var p struct {
			WorkspaceID uuid.UUID `json:"workspaceId"`
			ObjectID    uuid.UUID `json:"objectId"`
		}
		if json.Unmarshal(event.Payload, &p) != nil || p.WorkspaceID == uuid.Nil || p.ObjectID != event.AggregateID {
			return d.ErrInvalid
		}
		return c.EnqueuePublication(ctx, p.WorkspaceID, event.AggregateID, event.ID)
	}
}
