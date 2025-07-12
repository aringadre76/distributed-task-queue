package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"distributed-task-queue/internal/config"
	"distributed-task-queue/internal/database"
	"distributed-task-queue/internal/monitoring"
	"distributed-task-queue/internal/queue"
	"distributed-task-queue/pkg/types"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type WorkerPoolManager struct {
	workers           map[string]*Worker
	workerConfigs     map[string]config.WorkerConfig
	taskRepo          database.TaskRepository
	workerRepo        database.WorkerRepository
	queue             *queue.RedisQueue
	metrics           *monitoring.Metrics
	logger            *zap.Logger
	config            config.AutoScalingConfig
	mutex             sync.RWMutex
	lastScaleUp       time.Time
	lastScaleDown     time.Time
	queueDepthHistory []int
	running           bool
	stopChan          chan struct{}
}

type PoolStats struct {
	ActiveWorkers     int                        `json:"active_workers"`
	TotalWorkers      int                        `json:"total_workers"`
	QueueDepth        int                        `json:"queue_depth"`
	AverageQueueDepth float64                    `json:"average_queue_depth"`
	Workers           map[string]*WorkerStatus   `json:"workers"`
	LastScaleAction   string                     `json:"last_scale_action"`
	LastScaleTime     time.Time                  `json:"last_scale_time"`
}

type WorkerStatus struct {
	ID            string                 `json:"id"`
	Status        types.WorkerStatus     `json:"status"`
	ActiveTasks   int                    `json:"active_tasks"`
	MaxTasks      int                    `json:"max_tasks"`
	LastSeen      time.Time              `json:"last_seen"`
	TaskTypes     []string               `json:"task_types"`
	Utilization   float64                `json:"utilization"`
}

func NewWorkerPoolManager(
	taskRepo database.TaskRepository,
	workerRepo database.WorkerRepository,
	queue *queue.RedisQueue,
	metrics *monitoring.Metrics,
	logger *zap.Logger,
	autoScalingConfig config.AutoScalingConfig,
) *WorkerPoolManager {
	return &WorkerPoolManager{
		workers:           make(map[string]*Worker),
		workerConfigs:     make(map[string]config.WorkerConfig),
		taskRepo:          taskRepo,
		workerRepo:        workerRepo,
		queue:             queue,
		metrics:           metrics,
		logger:            logger,
		config:            autoScalingConfig,
		queueDepthHistory: make([]int, 0),
		stopChan:          make(chan struct{}),
	}
}

func (wpm *WorkerPoolManager) Start(ctx context.Context) error {
	if !wpm.config.Enabled {
		wpm.logger.Info("Auto-scaling disabled, not starting pool manager")
		return nil
	}

	wpm.mutex.Lock()
	wpm.running = true
	wpm.mutex.Unlock()

	wpm.logger.Info("Starting worker pool manager",
		zap.Int("min_workers", wpm.config.MinWorkers),
		zap.Int("max_workers", wpm.config.MaxWorkers))

	if err := wpm.ensureMinimumWorkers(ctx); err != nil {
		return fmt.Errorf("failed to ensure minimum workers: %w", err)
	}

	go wpm.monitoringLoop(ctx)

	return nil
}

func (wpm *WorkerPoolManager) Stop() {
	wpm.mutex.Lock()
	if !wpm.running {
		wpm.mutex.Unlock()
		return
	}
	wpm.running = false
	wpm.mutex.Unlock()

	close(wpm.stopChan)
	wpm.logger.Info("Worker pool manager stopped")
}

func (wpm *WorkerPoolManager) AddWorker(ctx context.Context, workerConfig config.WorkerConfig) error {
	wpm.mutex.Lock()
	defer wpm.mutex.Unlock()

	workerID := uuid.New().String()
	worker := NewWorker(
		workerID,
		wpm.taskRepo,
		wpm.workerRepo,
		wpm.queue,
		wpm.metrics,
		wpm.logger,
		workerConfig,
	)

	if err := worker.Register(ctx); err != nil {
		return fmt.Errorf("failed to register worker: %w", err)
	}

	wpm.workers[workerID] = worker
	wpm.workerConfigs[workerID] = workerConfig

	go worker.Start(ctx)

	wpm.logger.Info("Worker added to pool",
		zap.String("worker_id", workerID),
		zap.Int("total_workers", len(wpm.workers)))

	return nil
}

func (wpm *WorkerPoolManager) RemoveWorker(ctx context.Context, workerID string) error {
	wpm.mutex.Lock()
	defer wpm.mutex.Unlock()

	worker, exists := wpm.workers[workerID]
	if !exists {
		return fmt.Errorf("worker not found: %s", workerID)
	}

	worker.shutdown(ctx)
	delete(wpm.workers, workerID)
	delete(wpm.workerConfigs, workerID)

	wpm.logger.Info("Worker removed from pool",
		zap.String("worker_id", workerID),
		zap.Int("total_workers", len(wpm.workers)))

	return nil
}

func (wpm *WorkerPoolManager) GetStats(ctx context.Context) (*PoolStats, error) {
	wpm.mutex.RLock()
	defer wpm.mutex.RUnlock()

	queueDepth, err := wpm.getQueueDepth(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get queue depth: %w", err)
	}

	avgQueueDepth := wpm.calculateAverageQueueDepth()

	workerStatuses := make(map[string]*WorkerStatus)
	activeWorkers := 0

	dbWorkers, err := wpm.workerRepo.ListActive(ctx)
	if err != nil {
		wpm.logger.Warn("Failed to list active workers from database", zap.Error(err))
	} else {
		for _, worker := range dbWorkers {
			if worker.Status == types.WorkerStatusIdle || worker.Status == types.WorkerStatusBusy {
				activeWorkers++
			}

			utilization := 0.0
			if worker.MaxTasks > 0 {
				utilization = float64(worker.ActiveTasks) / float64(worker.MaxTasks) * 100
			}

			taskTypes := make([]string, 0)
			if worker.TaskTypes != nil {
				if types, ok := worker.TaskTypes["types"].([]interface{}); ok {
					for _, t := range types {
						if taskType, ok := t.(string); ok {
							taskTypes = append(taskTypes, taskType)
						}
					}
				}
			}

			workerStatuses[worker.ID.String()] = &WorkerStatus{
				ID:          worker.ID.String(),
				Status:      worker.Status,
				ActiveTasks: worker.ActiveTasks,
				MaxTasks:    worker.MaxTasks,
				LastSeen:    worker.LastSeen,
				TaskTypes:   taskTypes,
				Utilization: utilization,
			}
		}
	}

	lastAction := "none"
	lastTime := time.Time{}
	if !wpm.lastScaleUp.IsZero() && (wpm.lastScaleDown.IsZero() || wpm.lastScaleUp.After(wpm.lastScaleDown)) {
		lastAction = "scale_up"
		lastTime = wpm.lastScaleUp
	} else if !wpm.lastScaleDown.IsZero() {
		lastAction = "scale_down"
		lastTime = wpm.lastScaleDown
	}

	return &PoolStats{
		ActiveWorkers:     activeWorkers,
		TotalWorkers:      len(wpm.workers),
		QueueDepth:        queueDepth,
		AverageQueueDepth: avgQueueDepth,
		Workers:           workerStatuses,
		LastScaleAction:   lastAction,
		LastScaleTime:     lastTime,
	}, nil
}

func (wpm *WorkerPoolManager) monitoringLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-wpm.stopChan:
			return
		case <-ticker.C:
			if err := wpm.evaluateScaling(ctx); err != nil {
				wpm.logger.Error("Failed to evaluate scaling", zap.Error(err))
			}
		}
	}
}

func (wpm *WorkerPoolManager) evaluateScaling(ctx context.Context) error {
	queueDepth, err := wpm.getQueueDepth(ctx)
	if err != nil {
		return fmt.Errorf("failed to get queue depth: %w", err)
	}

	wpm.updateQueueDepthHistory(queueDepth)

	avgQueueDepth := wpm.calculateAverageQueueDepth()
	currentWorkers := len(wpm.workers)

	wpm.logger.Debug("Evaluating scaling",
		zap.Int("queue_depth", queueDepth),
		zap.Float64("avg_queue_depth", avgQueueDepth),
		zap.Int("current_workers", currentWorkers),
		zap.Float64("scale_up_threshold", wpm.config.ScaleUpThreshold),
		zap.Float64("scale_down_threshold", wpm.config.ScaleDownThreshold))

	if avgQueueDepth > wpm.config.ScaleUpThreshold && currentWorkers < wpm.config.MaxWorkers {
		if time.Since(wpm.lastScaleUp) >= wpm.config.ScaleUpCooldown {
			return wpm.scaleUp(ctx)
		}
	} else if avgQueueDepth < wpm.config.ScaleDownThreshold && currentWorkers > wpm.config.MinWorkers {
		if time.Since(wpm.lastScaleDown) >= wpm.config.ScaleDownCooldown {
			return wpm.scaleDown(ctx)
		}
	}

	return nil
}

func (wpm *WorkerPoolManager) scaleUp(ctx context.Context) error {
	wpm.mutex.Lock()
	defer wpm.mutex.Unlock()

	if len(wpm.workers) >= wpm.config.MaxWorkers {
		return nil
	}

	defaultConfig := config.WorkerConfig{
		MaxConcurrentTasks: 5,
		TaskTypes:          []string{"test_task", "image_processing", "email_sending", "data_etl"},
		PollInterval:       time.Second,
		HeartbeatInterval:  30 * time.Second,
		TaskTimeout:        10 * time.Minute,
	}

	workerID := uuid.New().String()
	worker := NewWorker(
		workerID,
		wpm.taskRepo,
		wpm.workerRepo,
		wpm.queue,
		wpm.metrics,
		wpm.logger,
		defaultConfig,
	)

	if err := worker.Register(ctx); err != nil {
		return fmt.Errorf("failed to register new worker: %w", err)
	}

	wpm.workers[workerID] = worker
	wpm.workerConfigs[workerID] = defaultConfig
	wpm.lastScaleUp = time.Now()

	go worker.Start(ctx)

	wpm.logger.Info("Scaled up worker pool",
		zap.String("worker_id", workerID),
		zap.Int("total_workers", len(wpm.workers)))

	return nil
}

func (wpm *WorkerPoolManager) scaleDown(ctx context.Context) error {
	wpm.mutex.Lock()
	defer wpm.mutex.Unlock()

	if len(wpm.workers) <= wpm.config.MinWorkers {
		return nil
	}

	var workerToRemove string
	for workerID := range wpm.workers {
		workerToRemove = workerID
		break
	}

	if workerToRemove == "" {
		return nil
	}

	worker := wpm.workers[workerToRemove]
	worker.shutdown(ctx)
	
	delete(wpm.workers, workerToRemove)
	delete(wpm.workerConfigs, workerToRemove)
	wpm.lastScaleDown = time.Now()

	wpm.logger.Info("Scaled down worker pool",
		zap.String("worker_id", workerToRemove),
		zap.Int("total_workers", len(wpm.workers)))

	return nil
}

func (wpm *WorkerPoolManager) getQueueDepth(ctx context.Context) (int, error) {
	stats, err := wpm.queue.GetQueueStats(ctx)
	if err != nil {
		return 0, err
	}

	totalPending := int(stats["total_pending"])
	return totalPending, nil
}

func (wpm *WorkerPoolManager) updateQueueDepthHistory(depth int) {
	wpm.queueDepthHistory = append(wpm.queueDepthHistory, depth)

	maxHistorySize := int(wpm.config.QueueDepthWindow.Minutes())
	if len(wpm.queueDepthHistory) > maxHistorySize {
		wpm.queueDepthHistory = wpm.queueDepthHistory[len(wpm.queueDepthHistory)-maxHistorySize:]
	}
}

func (wpm *WorkerPoolManager) calculateAverageQueueDepth() float64 {
	if len(wpm.queueDepthHistory) == 0 {
		return 0.0
	}

	sum := 0
	for _, depth := range wpm.queueDepthHistory {
		sum += depth
	}

	return float64(sum) / float64(len(wpm.queueDepthHistory))
}

func (wpm *WorkerPoolManager) ensureMinimumWorkers(ctx context.Context) error {
	currentWorkers := len(wpm.workers)
	
	for currentWorkers < wpm.config.MinWorkers {
		defaultConfig := config.WorkerConfig{
			MaxConcurrentTasks: 5,
			TaskTypes:          []string{"test_task", "image_processing", "email_sending", "data_etl"},
			PollInterval:       time.Second,
			HeartbeatInterval:  30 * time.Second,
			TaskTimeout:        10 * time.Minute,
		}

		if err := wpm.AddWorker(ctx, defaultConfig); err != nil {
			return fmt.Errorf("failed to add minimum worker: %w", err)
		}

		currentWorkers++
	}

	return nil
} 