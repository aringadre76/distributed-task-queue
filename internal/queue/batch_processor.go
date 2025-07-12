package queue

import (
	"context"
	"sync"
	"time"

	"distributed-task-queue/pkg/types"
	"go.uber.org/zap"
)

type BatchProcessor struct {
	queue       *RedisQueue
	logger      *zap.Logger
	batchSize   int
	flushTime   time.Duration
	buffer      []*types.Task
	mutex       sync.Mutex
	stopChan    chan struct{}
	flushChan   chan struct{}
	running     bool
}

type BatchConfig struct {
	BatchSize int
	FlushTime time.Duration
}

func NewBatchProcessor(queue *RedisQueue, logger *zap.Logger, config BatchConfig) *BatchProcessor {
	return &BatchProcessor{
		queue:     queue,
		logger:    logger,
		batchSize: config.BatchSize,
		flushTime: config.FlushTime,
		buffer:    make([]*types.Task, 0, config.BatchSize),
		stopChan:  make(chan struct{}),
		flushChan: make(chan struct{}, 1),
	}
}

func (bp *BatchProcessor) Start(ctx context.Context) error {
	bp.mutex.Lock()
	if bp.running {
		bp.mutex.Unlock()
		return nil
	}
	bp.running = true
	bp.mutex.Unlock()

	bp.logger.Info("Starting batch processor",
		zap.Int("batch_size", bp.batchSize),
		zap.Duration("flush_time", bp.flushTime))

	go bp.flushLoop(ctx)
	return nil
}

func (bp *BatchProcessor) Stop() {
	bp.mutex.Lock()
	if !bp.running {
		bp.mutex.Unlock()
		return
	}
	bp.running = false
	bp.mutex.Unlock()

	close(bp.stopChan)
	bp.flush(context.Background())
	bp.logger.Info("Batch processor stopped")
}

func (bp *BatchProcessor) EnqueueBatch(ctx context.Context, tasks []*types.Task) error {
	bp.mutex.Lock()
	defer bp.mutex.Unlock()

	for _, task := range tasks {
		bp.buffer = append(bp.buffer, task)
		
		if len(bp.buffer) >= bp.batchSize {
			bp.triggerFlush()
		}
	}

	return nil
}

func (bp *BatchProcessor) flushLoop(ctx context.Context) {
	ticker := time.NewTicker(bp.flushTime)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-bp.stopChan:
			return
		case <-ticker.C:
			bp.triggerFlush()
		case <-bp.flushChan:
			bp.flush(ctx)
		}
	}
}

func (bp *BatchProcessor) triggerFlush() {
	select {
	case bp.flushChan <- struct{}{}:
	default:
	}
}

func (bp *BatchProcessor) flush(ctx context.Context) {
	bp.mutex.Lock()
	if len(bp.buffer) == 0 {
		bp.mutex.Unlock()
		return
	}

	tasks := make([]*types.Task, len(bp.buffer))
	copy(tasks, bp.buffer)
	bp.buffer = bp.buffer[:0]
	bp.mutex.Unlock()

	bp.logger.Debug("Flushing batch", zap.Int("task_count", len(tasks)))

	for _, task := range tasks {
		if err := bp.queue.Enqueue(ctx, task); err != nil {
			bp.logger.Error("Failed to enqueue task in batch",
				zap.String("task_id", task.ID.String()),
				zap.Error(err))
		}
	}

	bp.logger.Info("Batch processed", zap.Int("task_count", len(tasks)))
}

func (bp *BatchProcessor) GetStats() map[string]interface{} {
	bp.mutex.Lock()
	defer bp.mutex.Unlock()

	return map[string]interface{}{
		"buffer_size":    len(bp.buffer),
		"batch_size":     bp.batchSize,
		"flush_interval": bp.flushTime,
		"running":        bp.running,
	}
} 