package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"distributed-task-queue/pkg/types"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type RedisQueue struct {
	client *redis.Client
	logger *zap.Logger
}

type QueuedTask struct {
	ID          uuid.UUID              `json:"id"`
	Type        string                 `json:"type"`
	Payload     map[string]interface{} `json:"payload"`
	Priority    types.TaskPriority     `json:"priority"`
	EnqueuedAt  time.Time              `json:"enqueued_at"`
	RetryCount  int                    `json:"retry_count"`
	MaxRetries  int                    `json:"max_retries"`
	ScheduledAt *time.Time             `json:"scheduled_at,omitempty"`
	Timeout     *time.Duration         `json:"timeout,omitempty"`
}

const (
	HighPriorityQueue   = "queue:high"
	MediumPriorityQueue = "queue:medium"
	LowPriorityQueue    = "queue:low"
	ProcessingQueue     = "queue:processing"
	DeadLetterQueue     = "queue:dead_letter"
	DelayedQueue        = "queue:delayed"
)

func NewRedisQueue(client *redis.Client, logger *zap.Logger) *RedisQueue {
	return &RedisQueue{
		client: client,
		logger: logger,
	}
}

func (q *RedisQueue) Enqueue(ctx context.Context, task *types.Task) error {
	queuedTask := &QueuedTask{
		ID:          task.ID,
		Type:        task.Type,
		Payload:     task.Payload,
		Priority:    task.Priority,
		EnqueuedAt:  time.Now(),
		RetryCount:  task.RetryCount,
		MaxRetries:  task.MaxRetries,
		ScheduledAt: task.ScheduledAt,
		Timeout:     task.Timeout,
	}

	data, err := json.Marshal(queuedTask)
	if err != nil {
		return fmt.Errorf("failed to marshal task: %w", err)
	}

	var queueName string
	if task.ScheduledAt != nil && task.ScheduledAt.After(time.Now()) {
		queueName = DelayedQueue
		score := float64(task.ScheduledAt.Unix())
		err = q.client.ZAdd(ctx, queueName, &redis.Z{
			Score:  score,
			Member: data,
		}).Err()
	} else {
		switch task.Priority {
		case types.PriorityHigh:
			queueName = HighPriorityQueue
		case types.PriorityMedium:
			queueName = MediumPriorityQueue
		case types.PriorityLow:
			queueName = LowPriorityQueue
		default:
			queueName = MediumPriorityQueue
		}
		err = q.client.LPush(ctx, queueName, data).Err()
	}

	if err != nil {
		q.logger.Error("Failed to enqueue task",
			zap.Error(err),
			zap.String("task_id", task.ID.String()),
			zap.String("queue", queueName))
		return fmt.Errorf("failed to enqueue task: %w", err)
	}

	q.logger.Info("Task enqueued",
		zap.String("task_id", task.ID.String()),
		zap.String("type", task.Type),
		zap.String("priority", string(task.Priority)),
		zap.String("queue", queueName))

	return nil
}

func (q *RedisQueue) Dequeue(ctx context.Context, workerID string) (*QueuedTask, error) {
	queues := []string{HighPriorityQueue, MediumPriorityQueue, LowPriorityQueue}

	for _, queueName := range queues {
		result, err := q.client.BRPopLPush(ctx, queueName, ProcessingQueue, time.Second).Result()
		if err != nil {
			if err == redis.Nil {
				continue
			}
			q.logger.Error("Failed to dequeue from queue",
				zap.Error(err),
				zap.String("queue", queueName),
				zap.String("worker_id", workerID))
			continue
		}

		var task QueuedTask
		if err := json.Unmarshal([]byte(result), &task); err != nil {
			q.logger.Error("Failed to unmarshal task",
				zap.Error(err),
				zap.String("worker_id", workerID))
			continue
		}

		q.logger.Info("Task dequeued",
			zap.String("task_id", task.ID.String()),
			zap.String("type", task.Type),
			zap.String("worker_id", workerID),
			zap.String("queue", queueName))

		return &task, nil
	}

	return nil, nil
}

func (q *RedisQueue) CompleteTask(ctx context.Context, taskID uuid.UUID) error {
	script := `
		local processing_queue = KEYS[1]
		local task_id = ARGV[1]
		
		local tasks = redis.call('LRANGE', processing_queue, 0, -1)
		for i, task_data in ipairs(tasks) do
			local task = cjson.decode(task_data)
			if task.id == task_id then
				redis.call('LREM', processing_queue, 1, task_data)
				return 1
			end
		end
		return 0
	`

	result, err := q.client.Eval(ctx, script, []string{ProcessingQueue}, taskID.String()).Result()
	if err != nil {
		q.logger.Error("Failed to complete task",
			zap.Error(err),
			zap.String("task_id", taskID.String()))
		return fmt.Errorf("failed to complete task: %w", err)
	}

	if result.(int64) == 0 {
		q.logger.Warn("Task not found in processing queue",
			zap.String("task_id", taskID.String()))
	} else {
		q.logger.Info("Task completed and removed from processing queue",
			zap.String("task_id", taskID.String()))
	}

	return nil
}

func (q *RedisQueue) FailTask(ctx context.Context, taskID uuid.UUID, shouldRetry bool) error {
	script := `
		local processing_queue = KEYS[1]
		local retry_queue = KEYS[2]
		local dead_letter_queue = KEYS[3]
		local task_id = ARGV[1]
		local should_retry = ARGV[2]
		
		local tasks = redis.call('LRANGE', processing_queue, 0, -1)
		for i, task_data in ipairs(tasks) do
			local task = cjson.decode(task_data)
			if task.id == task_id then
				redis.call('LREM', processing_queue, 1, task_data)
				
				if should_retry == "true" then
					task.retry_count = task.retry_count + 1
					local updated_task = cjson.encode(task)
					
					if task.priority == "high" then
						redis.call('LPUSH', 'queue:high', updated_task)
					elseif task.priority == "medium" then
						redis.call('LPUSH', 'queue:medium', updated_task)
					else
						redis.call('LPUSH', 'queue:low', updated_task)
					end
				else
					redis.call('LPUSH', dead_letter_queue, task_data)
				end
				return 1
			end
		end
		return 0
	`

	retryStr := "false"
	if shouldRetry {
		retryStr = "true"
	}

	targetQueue := MediumPriorityQueue
	result, err := q.client.Eval(ctx, script,
		[]string{ProcessingQueue, targetQueue, DeadLetterQueue},
		taskID.String(), retryStr).Result()

	if err != nil {
		q.logger.Error("Failed to handle task failure",
			zap.Error(err),
			zap.String("task_id", taskID.String()))
		return fmt.Errorf("failed to handle task failure: %w", err)
	}

	if result.(int64) == 0 {
		q.logger.Warn("Task not found in processing queue",
			zap.String("task_id", taskID.String()))
	} else {
		if shouldRetry {
			q.logger.Info("Task failed and requeued for retry",
				zap.String("task_id", taskID.String()))
		} else {
			q.logger.Info("Task failed and moved to dead letter queue",
				zap.String("task_id", taskID.String()))
		}
	}

	return nil
}

func (q *RedisQueue) ProcessDelayedTasks(ctx context.Context) error {
	now := float64(time.Now().Unix())
	
	result, err := q.client.ZRangeByScoreWithScores(ctx, DelayedQueue, &redis.ZRangeBy{
		Min: "0",
		Max: fmt.Sprintf("%f", now),
	}).Result()

	if err != nil {
		return fmt.Errorf("failed to get delayed tasks: %w", err)
	}

	for _, item := range result {
		taskData := item.Member.(string)
		
		var task QueuedTask
		if err := json.Unmarshal([]byte(taskData), &task); err != nil {
			q.logger.Error("Failed to unmarshal delayed task", zap.Error(err))
			continue
		}

		var queueName string
		switch task.Priority {
		case types.PriorityHigh:
			queueName = HighPriorityQueue
		case types.PriorityMedium:
			queueName = MediumPriorityQueue
		case types.PriorityLow:
			queueName = LowPriorityQueue
		default:
			queueName = MediumPriorityQueue
		}

		pipe := q.client.TxPipeline()
		pipe.ZRem(ctx, DelayedQueue, taskData)
		pipe.LPush(ctx, queueName, taskData)
		
		if _, err := pipe.Exec(ctx); err != nil {
			q.logger.Error("Failed to move delayed task to queue",
				zap.Error(err),
				zap.String("task_id", task.ID.String()))
			continue
		}

		q.logger.Info("Delayed task moved to queue",
			zap.String("task_id", task.ID.String()),
			zap.String("queue", queueName))
	}

	return nil
}

func (q *RedisQueue) GetQueueStats(ctx context.Context) (map[string]int64, error) {
	pipe := q.client.Pipeline()
	
	highCmd := pipe.LLen(ctx, HighPriorityQueue)
	mediumCmd := pipe.LLen(ctx, MediumPriorityQueue)
	lowCmd := pipe.LLen(ctx, LowPriorityQueue)
	processingCmd := pipe.LLen(ctx, ProcessingQueue)
	deadLetterCmd := pipe.LLen(ctx, DeadLetterQueue)
	delayedCmd := pipe.ZCard(ctx, DelayedQueue)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get queue stats: %w", err)
	}

	stats := map[string]int64{
		"high_priority":  highCmd.Val(),
		"medium_priority": mediumCmd.Val(),
		"low_priority":   lowCmd.Val(),
		"processing":     processingCmd.Val(),
		"dead_letter":    deadLetterCmd.Val(),
		"delayed":        delayedCmd.Val(),
	}

	total := stats["high_priority"] + stats["medium_priority"] + stats["low_priority"]
	stats["total_pending"] = total

	return stats, nil
}

func (q *RedisQueue) HealthCheck(ctx context.Context) error {
	_, err := q.client.Ping(ctx).Result()
	if err != nil {
		return fmt.Errorf("redis health check failed: %w", err)
	}
	return nil
} 