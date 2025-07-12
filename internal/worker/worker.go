package worker

import (
	"context"
	"fmt"
	"time"

	"distributed-task-queue/internal/config"
	"distributed-task-queue/internal/database"
	"distributed-task-queue/internal/models"
	"distributed-task-queue/internal/monitoring"
	"distributed-task-queue/internal/queue"
	"distributed-task-queue/pkg/types"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type Worker struct {
	id               string
	taskRepo         database.TaskRepository
	workerRepo       database.WorkerRepository
	queue            *queue.RedisQueue
	metrics          *monitoring.Metrics
	logger           *zap.Logger
	config           config.WorkerConfig
	circuitBreakers  *TaskTypeCircuitBreakers
	errorClassifier  *ErrorClassifier
	backoffCalculator *BackoffCalculator
}

func NewWorker(
	id string,
	taskRepo database.TaskRepository,
	workerRepo database.WorkerRepository,
	queue *queue.RedisQueue,
	metrics *monitoring.Metrics,
	logger *zap.Logger,
	config config.WorkerConfig,
) *Worker {
	worker := &Worker{
		id:         id,
		taskRepo:   taskRepo,
		workerRepo: workerRepo,
		queue:      queue,
		metrics:    metrics,
		logger:     logger.With(zap.String("worker_id", id)),
		config:     config,
	}

	if config.CircuitBreaker.Enabled {
		cbConfig := CircuitBreakerConfig{
			FailureThreshold:  config.CircuitBreaker.FailureThreshold,
			SuccessThreshold:  config.CircuitBreaker.SuccessThreshold,
			Timeout:          config.CircuitBreaker.Timeout,
			MonitoringPeriod: config.CircuitBreaker.MonitoringPeriod,
		}
		worker.circuitBreakers = NewTaskTypeCircuitBreakers(cbConfig, logger)
	}

	if config.ErrorHandling.EnableClassification {
		worker.errorClassifier = NewErrorClassifier(logger)
		worker.backoffCalculator = NewBackoffCalculator(worker.errorClassifier, logger)
	}

	return worker
}

func (w *Worker) Register(ctx context.Context) error {
	workerID, err := uuid.Parse(w.id)
	if err != nil {
		return fmt.Errorf("invalid worker ID: %w", err)
	}

	// Convert task types to JSONB format
	taskTypesJSON := models.JSONB{"types": w.config.TaskTypes}

	worker := &models.WorkerModel{
		ID:           workerID,
		Name:         w.id,
		Status:       types.WorkerStatusIdle,
		LastSeen:     time.Now(),
		TaskTypes:    taskTypesJSON,
		MaxTasks:     w.config.MaxConcurrentTasks,
		ActiveTasks:  0,
		TotalTasks:   0,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		Metadata:     models.JSONB{"version": "1.0"},
	}

	if err := w.workerRepo.Create(ctx, worker); err != nil {
		return fmt.Errorf("failed to register worker: %w", err)
	}

	w.logger.Info("Worker registered successfully")
	return nil
}

func (w *Worker) Start(ctx context.Context) {
	w.logger.Info("Worker starting task processing loop")

	heartbeatTicker := time.NewTicker(w.config.HeartbeatInterval)
	defer heartbeatTicker.Stop()

	pollTicker := time.NewTicker(w.config.PollInterval)
	defer pollTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("Worker stopping due to context cancellation")
			w.shutdown(ctx)
			return
		case <-heartbeatTicker.C:
			w.sendHeartbeat(ctx)
		case <-pollTicker.C:
			w.pollAndProcessTask(ctx)
		}
	}
}

func (w *Worker) sendHeartbeat(ctx context.Context) {
	workerID, err := uuid.Parse(w.id)
	if err != nil {
		w.logger.Error("Invalid worker ID", zap.Error(err))
		return
	}

	if err := w.workerRepo.UpdateHeartbeat(ctx, workerID, types.WorkerStatusIdle); err != nil {
		w.logger.Error("Failed to send heartbeat", zap.Error(err))
	}
}

func (w *Worker) pollAndProcessTask(ctx context.Context) {
	queuedTask, err := w.queue.Dequeue(ctx, w.id)
	if err != nil {
		w.logger.Error("Failed to dequeue task", zap.Error(err))
		return
	}

	if queuedTask == nil {
		return
	}

	task := &types.Task{
		ID:          queuedTask.ID,
		Type:        queuedTask.Type,
		Payload:     queuedTask.Payload,
		Priority:    queuedTask.Priority,
		Status:      types.TaskStatusRunning,
		RetryCount:  queuedTask.RetryCount,
		MaxRetries:  queuedTask.MaxRetries,
		CreatedAt:   queuedTask.EnqueuedAt,
		ScheduledAt: queuedTask.ScheduledAt,
	}

	w.processTask(ctx, task)
}

func (w *Worker) processTask(ctx context.Context, task *types.Task) {
	startTime := time.Now()
	w.logger.Info("Processing task", 
		zap.String("task_id", task.ID.String()), 
		zap.String("task_type", task.Type))

	if w.circuitBreakers != nil {
		circuitBreaker := w.circuitBreakers.GetCircuitBreaker(task.Type)
		if !circuitBreaker.CanExecute() {
			w.logger.Warn("Circuit breaker open for task type, skipping task",
				zap.String("task_id", task.ID.String()),
				zap.String("task_type", task.Type))
			
			err := fmt.Errorf("circuit breaker open for task type: %s", task.Type)
			w.handleTaskFailure(ctx, task, err)
			return
		}
	}

	if err := w.taskRepo.UpdateStatus(ctx, task.ID, types.TaskStatusRunning, &w.id); err != nil {
		w.logger.Error("Failed to update task status to running", 
			zap.String("task_id", task.ID.String()), 
			zap.Error(err))
		return
	}

	result, err := w.executeTask(ctx, task)
	
	if err != nil {
		if w.circuitBreakers != nil {
			circuitBreaker := w.circuitBreakers.GetCircuitBreaker(task.Type)
			circuitBreaker.RecordFailure(err)
		}
		w.handleTaskFailure(ctx, task, err)
	} else {
		if w.circuitBreakers != nil {
			circuitBreaker := w.circuitBreakers.GetCircuitBreaker(task.Type)
			circuitBreaker.RecordSuccess()
		}
		w.handleTaskSuccess(ctx, task, result)
	}

	duration := time.Since(startTime)
	w.metrics.TaskCompleted(duration)
	w.logger.Info("Task processing completed", 
		zap.String("task_id", task.ID.String()),
		zap.Duration("duration", duration))
}

func (w *Worker) executeTask(ctx context.Context, task *types.Task) (map[string]interface{}, error) {
	var timeout time.Duration
	if task.Timeout != nil {
		timeout = *task.Timeout
	} else {
		timeout = w.config.TaskTimeout
	}

	taskCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch task.Type {
	case "test_task":
		return w.executeTestTask(taskCtx, task)
	case "image_processing":
		return w.executeImageProcessing(taskCtx, task)
	case "email_sending":
		return w.executeEmailSending(taskCtx, task)
	case "data_etl":
		return w.executeDataETL(taskCtx, task)
	case "parallel_batch":
		return w.executeParallelBatch(taskCtx, task)
	default:
		return nil, fmt.Errorf("unknown task type: %s", task.Type)
	}
}

func (w *Worker) executeTestTask(ctx context.Context, task *types.Task) (map[string]interface{}, error) {
	w.logger.Info("Executing test task", zap.String("task_id", task.ID.String()))
	
	sleepDuration := 2 * time.Second
	if duration, ok := task.Payload["sleep_duration"]; ok {
		if d, ok := duration.(float64); ok {
			sleepDuration = time.Duration(d) * time.Second
		}
	}

	select {
	case <-time.After(sleepDuration):
		result := map[string]interface{}{
			"status":          "completed",
			"processing_time": sleepDuration.String(),
			"worker_id":       w.id,
			"message":         fmt.Sprintf("Test task completed by worker %s", w.id),
			"payload":         task.Payload,
		}
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (w *Worker) executeImageProcessing(ctx context.Context, task *types.Task) (map[string]interface{}, error) {
	w.logger.Info("Executing image processing task", zap.String("task_id", task.ID.String()))
	
	select {
	case <-time.After(3 * time.Second):
		result := map[string]interface{}{
			"status":       "processed",
			"images_count": 1,
			"worker_id":    w.id,
			"operations":   []string{"resize", "watermark"},
		}
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (w *Worker) executeEmailSending(ctx context.Context, task *types.Task) (map[string]interface{}, error) {
	w.logger.Info("Executing email sending task", zap.String("task_id", task.ID.String()))
	
	select {
	case <-time.After(1 * time.Second):
		result := map[string]interface{}{
			"status":      "sent",
			"emails_sent": 1,
			"worker_id":   w.id,
			"recipient":   task.Payload["recipient"],
		}
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (w *Worker) executeDataETL(ctx context.Context, task *types.Task) (map[string]interface{}, error) {
	w.logger.Info("Executing data ETL task", zap.String("task_id", task.ID.String()))
	
	select {
	case <-time.After(5 * time.Second):
		result := map[string]interface{}{
			"status":        "processed",
			"records_count": 1000,
			"worker_id":     w.id,
			"source":        task.Payload["source"],
		}
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (w *Worker) executeParallelBatch(ctx context.Context, task *types.Task) (map[string]interface{}, error) {
	w.logger.Info("Executing parallel batch task", zap.String("task_id", task.ID.String()))
	
	batchSize := 10
	if size, ok := task.Payload["batch_size"]; ok {
		if s, ok := size.(float64); ok {
			batchSize = int(s)
		}
	}
	
	processingTime := 3 * time.Second
	if duration, ok := task.Payload["processing_time"]; ok {
		if d, ok := duration.(float64); ok {
			processingTime = time.Duration(d) * time.Second
		}
	}
	
	select {
	case <-time.After(processingTime):
		result := map[string]interface{}{
			"status":         "batch_processed",
			"batch_size":     batchSize,
			"processing_time": processingTime.String(),
			"worker_id":      w.id,
			"items_processed": batchSize,
			"parallel_execution": true,
		}
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (w *Worker) handleTaskSuccess(ctx context.Context, task *types.Task, result map[string]interface{}) {
	w.logger.Info("Task completed successfully", zap.String("task_id", task.ID.String()))

	if err := w.taskRepo.UpdateStatus(ctx, task.ID, types.TaskStatusCompleted, &w.id); err != nil {
		w.logger.Error("Failed to update task status to completed", 
			zap.String("task_id", task.ID.String()), 
			zap.Error(err))
	}

	if err := w.taskRepo.UpdateResult(ctx, task.ID, result, nil); err != nil {
		w.logger.Error("Failed to update task result", 
			zap.String("task_id", task.ID.String()), 
			zap.Error(err))
	}

	if err := w.queue.CompleteTask(ctx, task.ID); err != nil {
		w.logger.Error("Failed to complete task in queue", 
			zap.String("task_id", task.ID.String()), 
			zap.Error(err))
	}
}

func (w *Worker) handleTaskFailure(ctx context.Context, task *types.Task, taskErr error) {
	w.logger.Error("Task failed", 
		zap.String("task_id", task.ID.String()), 
		zap.Error(taskErr))

	if err := w.taskRepo.IncrementRetryCount(ctx, task.ID); err != nil {
		w.logger.Error("Failed to increment retry count", 
			zap.String("task_id", task.ID.String()), 
			zap.Error(err))
	}

	task.RetryCount++
	
	errorMsg := taskErr.Error()
	var errorType string
	var shouldRetry bool
	var retryDelay time.Duration

	if w.errorClassifier != nil {
		classification := w.errorClassifier.ClassifyError(taskErr)
		errorType = string(classification.Type)
		shouldRetry = w.errorClassifier.ShouldRetry(taskErr, task.RetryCount)
		if shouldRetry {
			retryDelay = w.backoffCalculator.CalculateDelay(ctx, taskErr, task.RetryCount)
		}
	} else {
		shouldRetry = task.RetryCount < task.MaxRetries
		errorType = "unclassified"
	}

	errorResult := map[string]interface{}{
		"error":        errorMsg,
		"error_type":   errorType,
		"retry_count":  task.RetryCount,
		"worker_id":    w.id,
		"failed_at":    time.Now(),
		"retry_delay":  retryDelay.String(),
	}

	if err := w.taskRepo.UpdateResult(ctx, task.ID, errorResult, &errorMsg); err != nil {
		w.logger.Error("Failed to update task error result", 
			zap.String("task_id", task.ID.String()), 
			zap.Error(err))
	}

	if shouldRetry {
		w.logger.Info("Retrying task with backoff", 
			zap.String("task_id", task.ID.String()), 
			zap.Int("retry_count", task.RetryCount),
			zap.String("error_type", errorType),
			zap.Duration("retry_delay", retryDelay))

		if retryDelay > 0 {
			scheduledAt := time.Now().Add(retryDelay)
			if err := w.taskRepo.UpdateScheduledAt(ctx, task.ID, &scheduledAt); err != nil {
				w.logger.Error("Failed to update scheduled time for retry", 
					zap.String("task_id", task.ID.String()), 
					zap.Error(err))
			}
		}

		if err := w.taskRepo.UpdateStatus(ctx, task.ID, types.TaskStatusPending, nil); err != nil {
			w.logger.Error("Failed to update task status for retry", 
				zap.String("task_id", task.ID.String()), 
				zap.Error(err))
		}

		w.metrics.TaskRetried()
	} else {
		w.logger.Warn("Task exceeded max retries or marked as non-retryable, marking as failed", 
			zap.String("task_id", task.ID.String()), 
			zap.Int("retry_count", task.RetryCount),
			zap.String("error_type", errorType))

		if err := w.taskRepo.UpdateStatus(ctx, task.ID, types.TaskStatusFailed, &w.id); err != nil {
			w.logger.Error("Failed to update task status to failed", 
				zap.String("task_id", task.ID.String()), 
				zap.Error(err))
		}

		w.metrics.TaskFailed()
	}

	if err := w.queue.FailTask(ctx, task.ID, shouldRetry); err != nil {
		w.logger.Error("Failed to handle task failure in queue", 
			zap.String("task_id", task.ID.String()), 
			zap.Error(err))
	}
}

func (w *Worker) shutdown(ctx context.Context) {
	w.logger.Info("Worker shutting down...")

	workerID, err := uuid.Parse(w.id)
	if err != nil {
		w.logger.Error("Invalid worker ID during shutdown", zap.Error(err))
		return
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := w.workerRepo.UpdateHeartbeat(shutdownCtx, workerID, types.WorkerStatusOffline); err != nil {
		w.logger.Error("Failed to update worker status to offline", zap.Error(err))
	}

	w.logger.Info("Worker shutdown complete")
} 