package queue

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"
)

const TaskHealthPing = "platform:health:ping"

type Config struct {
	Addr     string
	Password string
	DB       int
}

func RedisClientOpt(cfg Config) asynq.RedisClientOpt {
	return asynq.RedisClientOpt{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	}
}

func NewClient(cfg Config) *asynq.Client {
	return asynq.NewClient(RedisClientOpt(cfg))
}

func NewServer(cfg Config) *asynq.Server {
	return asynq.NewServer(
		RedisClientOpt(cfg),
		asynq.Config{Concurrency: 4},
	)
}

func EnqueueHealthPing(ctx context.Context, client *asynq.Client) error {
	task := asynq.NewTask(TaskHealthPing, nil)
	if _, err := client.EnqueueContext(ctx, task); err != nil {
		return fmt.Errorf("enqueue health ping: %w", err)
	}
	return nil
}
