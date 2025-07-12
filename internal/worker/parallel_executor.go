package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"distributed-task-queue/pkg/types"
	"go.uber.org/zap"
)

type ParallelExecutor struct {
	maxConcurrency int
	activeJobs     map[string]*JobExecution
	jobQueue       chan *JobExecution
	workers        []*ExecutorWorker
	logger         *zap.Logger
	mutex          sync.RWMutex
	stopChan       chan struct{}
	running        bool
}

type JobExecution struct {
	Task      *types.Task
	Context   context.Context
	Cancel    context.CancelFunc
	StartTime time.Time
	Result    chan ExecutionResult
}

type ExecutionResult struct {
	Result map[string]interface{}
	Error  error
}

type ExecutorWorker struct {
	id       int
	executor *ParallelExecutor
	logger   *zap.Logger
}

func NewParallelExecutor(maxConcurrency int, logger *zap.Logger) *ParallelExecutor {
	return &ParallelExecutor{
		maxConcurrency: maxConcurrency,
		activeJobs:     make(map[string]*JobExecution),
		jobQueue:       make(chan *JobExecution, maxConcurrency*2),
		workers:        make([]*ExecutorWorker, maxConcurrency),
		logger:         logger,
		stopChan:       make(chan struct{}),
	}
}

func (pe *ParallelExecutor) Start(ctx context.Context) error {
	pe.mutex.Lock()
	if pe.running {
		pe.mutex.Unlock()
		return nil
	}
	pe.running = true
	pe.mutex.Unlock()

	pe.logger.Info("Starting parallel executor",
		zap.Int("max_concurrency", pe.maxConcurrency))

	for i := 0; i < pe.maxConcurrency; i++ {
		worker := &ExecutorWorker{
			id:       i,
			executor: pe,
			logger:   pe.logger.With(zap.Int("worker_id", i)),
		}
		pe.workers[i] = worker
		go worker.run(ctx)
	}

	return nil
}

func (pe *ParallelExecutor) Stop() {
	pe.mutex.Lock()
	if !pe.running {
		pe.mutex.Unlock()
		return
	}
	pe.running = false
	pe.mutex.Unlock()

	close(pe.stopChan)

	pe.mutex.Lock()
	for _, job := range pe.activeJobs {
		job.Cancel()
	}
	pe.mutex.Unlock()

	pe.logger.Info("Parallel executor stopped")
}

func (pe *ParallelExecutor) ExecuteAsync(ctx context.Context, task *types.Task) (*JobExecution, error) {
	if !pe.running {
		return nil, ErrExecutorStopped
	}

	jobCtx, cancel := context.WithCancel(ctx)
	job := &JobExecution{
		Task:      task,
		Context:   jobCtx,
		Cancel:    cancel,
		StartTime: time.Now(),
		Result:    make(chan ExecutionResult, 1),
	}

	pe.mutex.Lock()
	pe.activeJobs[task.ID.String()] = job
	pe.mutex.Unlock()

	select {
	case pe.jobQueue <- job:
		pe.logger.Debug("Task queued for parallel execution",
			zap.String("task_id", task.ID.String()),
			zap.String("task_type", task.Type))
		return job, nil
	case <-ctx.Done():
		pe.removeJob(task.ID.String())
		cancel()
		return nil, ctx.Err()
	}
}

func (pe *ParallelExecutor) ExecuteSync(ctx context.Context, task *types.Task) (map[string]interface{}, error) {
	job, err := pe.ExecuteAsync(ctx, task)
	if err != nil {
		return nil, err
	}

	select {
	case result := <-job.Result:
		return result.Result, result.Error
	case <-ctx.Done():
		job.Cancel()
		return nil, ctx.Err()
	}
}

func (pe *ParallelExecutor) GetActiveJobs() []*JobExecution {
	pe.mutex.RLock()
	defer pe.mutex.RUnlock()

	jobs := make([]*JobExecution, 0, len(pe.activeJobs))
	for _, job := range pe.activeJobs {
		jobs = append(jobs, job)
	}
	return jobs
}

func (pe *ParallelExecutor) GetStats() map[string]interface{} {
	pe.mutex.RLock()
	defer pe.mutex.RUnlock()

	return map[string]interface{}{
		"max_concurrency": pe.maxConcurrency,
		"active_jobs":     len(pe.activeJobs),
		"queue_size":      len(pe.jobQueue),
		"running":         pe.running,
		"utilization":     float64(len(pe.activeJobs)) / float64(pe.maxConcurrency) * 100,
	}
}

func (pe *ParallelExecutor) removeJob(taskID string) {
	pe.mutex.Lock()
	defer pe.mutex.Unlock()
	delete(pe.activeJobs, taskID)
}

func (ew *ExecutorWorker) run(ctx context.Context) {
	ew.logger.Debug("Executor worker started")
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ew.executor.stopChan:
			return
		case job := <-ew.executor.jobQueue:
			ew.executeJob(job)
		}
	}
}

func (ew *ExecutorWorker) executeJob(job *JobExecution) {
	defer ew.executor.removeJob(job.Task.ID.String())

	ew.logger.Debug("Executing task",
		zap.String("task_id", job.Task.ID.String()),
		zap.String("task_type", job.Task.Type))

	result, err := ew.performTaskExecution(job.Context, job.Task)
	
	select {
	case job.Result <- ExecutionResult{Result: result, Error: err}:
	case <-job.Context.Done():
	}

	duration := time.Since(job.StartTime)
	ew.logger.Debug("Task execution completed",
		zap.String("task_id", job.Task.ID.String()),
		zap.Duration("duration", duration),
		zap.Bool("success", err == nil))
}

func (ew *ExecutorWorker) performTaskExecution(ctx context.Context, task *types.Task) (map[string]interface{}, error) {
	switch task.Type {
	case "test_task":
		return ew.executeTestTask(ctx, task)
	case "image_processing":
		return ew.executeImageProcessing(ctx, task)
	case "email_sending":
		return ew.executeEmailSending(ctx, task)
	case "data_etl":
		return ew.executeDataETL(ctx, task)
	case "parallel_batch":
		return ew.executeParallelBatch(ctx, task)
	default:
		return nil, ErrUnknownTaskType
	}
}

func (ew *ExecutorWorker) executeTestTask(ctx context.Context, task *types.Task) (map[string]interface{}, error) {
	sleepDuration := 1 * time.Second
	if duration, ok := task.Payload["processing_time"]; ok {
		if d, ok := duration.(float64); ok {
			sleepDuration = time.Duration(d) * time.Second
		}
	}

	select {
	case <-time.After(sleepDuration):
		return map[string]interface{}{
			"status":          "completed",
			"processing_time": sleepDuration.String(),
			"worker_id":       ew.id,
			"execution_mode":  "parallel",
			"payload":         task.Payload,
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (ew *ExecutorWorker) executeImageProcessing(ctx context.Context, task *types.Task) (map[string]interface{}, error) {
	select {
	case <-time.After(2 * time.Second):
		return map[string]interface{}{
			"status":         "processed",
			"images_count":   1,
			"worker_id":      ew.id,
			"execution_mode": "parallel",
			"operations":     []string{"resize", "watermark", "optimize"},
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (ew *ExecutorWorker) executeEmailSending(ctx context.Context, task *types.Task) (map[string]interface{}, error) {
	select {
	case <-time.After(500 * time.Millisecond):
		return map[string]interface{}{
			"status":         "sent",
			"emails_sent":    1,
			"worker_id":      ew.id,
			"execution_mode": "parallel",
			"recipient":      task.Payload["recipient"],
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (ew *ExecutorWorker) executeDataETL(ctx context.Context, task *types.Task) (map[string]interface{}, error) {
	select {
	case <-time.After(3 * time.Second):
		return map[string]interface{}{
			"status":         "processed",
			"records_count":  1000,
			"worker_id":      ew.id,
			"execution_mode": "parallel",
			"source":         task.Payload["source"],
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (ew *ExecutorWorker) executeParallelBatch(ctx context.Context, task *types.Task) (map[string]interface{}, error) {
	batchSize := 5
	if size, ok := task.Payload["batch_size"]; ok {
		if s, ok := size.(float64); ok {
			batchSize = int(s)
		}
	}

	select {
	case <-time.After(time.Duration(batchSize*100) * time.Millisecond):
		return map[string]interface{}{
			"status":         "completed",
			"batch_size":     batchSize,
			"worker_id":      ew.id,
			"execution_mode": "parallel_batch",
			"processed_items": batchSize,
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

var (
	ErrExecutorStopped  = fmt.Errorf("parallel executor is stopped")
	ErrUnknownTaskType  = fmt.Errorf("unknown task type")
) 