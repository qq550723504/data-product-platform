package workflowqueue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

const TaskExecute = "workflow:execution:run"

type ExecutionEnqueuer interface {
	EnqueueExecution(ctx context.Context, executionID uuid.UUID) error
}

type executionPayload struct {
	ExecutionID uuid.UUID `json:"executionId"`
}

type Client struct {
	client *asynq.Client
}

func NewClient(client *asynq.Client) *Client {
	return &Client{client: client}
}

func (c *Client) EnqueueExecution(ctx context.Context, executionID uuid.UUID) error {
	if executionID == uuid.Nil {
		return fmt.Errorf("execution id is required")
	}
	payload, err := json.Marshal(executionPayload{ExecutionID: executionID})
	if err != nil {
		return fmt.Errorf("marshal execution task: %w", err)
	}
	task := asynq.NewTask(TaskExecute, payload, asynq.MaxRetry(5))
	if _, err := c.client.EnqueueContext(ctx, task); err != nil {
		return fmt.Errorf("enqueue workflow execution: %w", err)
	}
	return nil
}

func ExecutionID(task *asynq.Task) (uuid.UUID, error) {
	var payload executionPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return uuid.Nil, fmt.Errorf("decode execution task: %w", err)
	}
	if payload.ExecutionID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("execution task is missing executionId")
	}
	return payload.ExecutionID, nil
}
