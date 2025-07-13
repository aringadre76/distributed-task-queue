package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"distributed-task-queue/internal/database"
	"distributed-task-queue/internal/models"
	"distributed-task-queue/internal/monitoring"
	"distributed-task-queue/internal/queue"
	"distributed-task-queue/internal/worker"
	"distributed-task-queue/pkg/types"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Valid task types that the system supports
var validTaskTypes = map[string]bool{
	"test_task":        true,
	"image_processing": true,
	"email_sending":    true,
	"data_etl":         true,
}

type TaskHandler struct {
	taskRepo           database.TaskRepository
	workerRepo         database.WorkerRepository
	queue              *queue.RedisQueue
	metrics            *monitoring.Metrics
	healthCheck        *monitoring.HealthChecker
	logger             *zap.Logger
	batchProcessor     *queue.BatchProcessor
	dependencyManager  *queue.DependencyManager
	parallelExecutor   *worker.ParallelExecutor
}

func NewTaskHandler(
	taskRepo database.TaskRepository,
	workerRepo database.WorkerRepository,
	queue *queue.RedisQueue,
	metrics *monitoring.Metrics,
	healthCheck *monitoring.HealthChecker,
	logger *zap.Logger,
) *TaskHandler {
	return &TaskHandler{
		taskRepo:    taskRepo,
		workerRepo:  workerRepo,
		queue:       queue,
		metrics:     metrics,
		healthCheck: healthCheck,
		logger:      logger,
	}
}

func (h *TaskHandler) SetBatchProcessor(bp *queue.BatchProcessor) {
	h.batchProcessor = bp
}

func (h *TaskHandler) SetDependencyManager(dm *queue.DependencyManager) {
	h.dependencyManager = dm
}

func (h *TaskHandler) SetParallelExecutor(pe *worker.ParallelExecutor) {
	h.parallelExecutor = pe
}

func (h *TaskHandler) SubmitTask(c *gin.Context) {
	var req types.TaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format", "details": err.Error()})
		return
	}

	// Validate task type
	if !validTaskTypes[req.Type] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task type", "details": fmt.Sprintf("Task type '%s' is not supported. Valid types: %v", req.Type, getValidTaskTypes())})
		return
	}

	taskModel := models.FromTaskRequest(&req)

	if err := h.taskRepo.Create(c.Request.Context(), taskModel); err != nil {
		h.logger.Error("Failed to create task in database", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create task"})
		return
	}

	task := taskModel.ToTask()

	if err := h.queue.Enqueue(c.Request.Context(), task); err != nil {
		h.logger.Error("Failed to enqueue task", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to enqueue task"})
		return
	}

	h.metrics.TaskSubmitted()

	response := types.TaskResponse{
		Task:    *task,
		Message: "Task submitted successfully",
	}

	h.logger.Info("Task submitted",
		zap.String("task_id", task.ID.String()),
		zap.String("type", task.Type),
		zap.String("priority", string(task.Priority)))

	c.JSON(http.StatusCreated, response)
}

// getValidTaskTypes returns a slice of valid task types for error messages
func getValidTaskTypes() []string {
	types := make([]string, 0, len(validTaskTypes))
	for taskType := range validTaskTypes {
		types = append(types, taskType)
	}
	return types
}

func (h *TaskHandler) GetTask(c *gin.Context) {
	taskIDStr := c.Param("id")
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID format"})
		return
	}

	taskModel, err := h.taskRepo.GetByID(c.Request.Context(), taskID)
	if err != nil {
		if err.Error() == "task not found: "+taskID.String() {
			c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
			return
		}
		h.logger.Error("Failed to get task", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve task"})
		return
	}

	task := taskModel.ToTask()
	response := types.TaskResponse{
		Task: *task,
	}

	c.JSON(http.StatusOK, response)
}

func (h *TaskHandler) ListTasks(c *gin.Context) {
	statusStr := c.DefaultQuery("status", "")
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("page_size", "20")

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	pageSize, err := strconv.Atoi(pageSizeStr)
	if err != nil || pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var status types.TaskStatus
	if statusStr != "" {
		status = types.TaskStatus(statusStr)
		validStatuses := map[types.TaskStatus]bool{
			types.TaskStatusPending:   true,
			types.TaskStatusRunning:   true,
			types.TaskStatusCompleted: true,
			types.TaskStatusFailed:    true,
			types.TaskStatusCancelled: true,
		}
		if !validStatuses[status] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status"})
			return
		}
	} else {
		status = types.TaskStatusPending
	}

	taskModels, total, err := h.taskRepo.ListByStatus(c.Request.Context(), status, page, pageSize)
	if err != nil {
		h.logger.Error("Failed to list tasks", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list tasks"})
		return
	}

	tasks := make([]types.Task, len(taskModels))
	for i, taskModel := range taskModels {
		tasks[i] = *taskModel.ToTask()
	}

	totalPages := (total + pageSize - 1) / pageSize

	response := types.TaskListResponse{
		Tasks:      tasks,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}

	c.JSON(http.StatusOK, response)
}

func (h *TaskHandler) CancelTask(c *gin.Context) {
	taskIDStr := c.Param("id")
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID format"})
		return
	}

	if err := h.taskRepo.UpdateStatus(c.Request.Context(), taskID, types.TaskStatusCancelled, nil); err != nil {
		if err.Error() == "task not found: "+taskID.String() {
			c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
			return
		}
		h.logger.Error("Failed to cancel task", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cancel task"})
		return
	}

	h.logger.Info("Task cancelled", zap.String("task_id", taskID.String()))
	c.JSON(http.StatusOK, gin.H{"message": "Task cancelled successfully"})
}

func (h *TaskHandler) GetQueueStats(c *gin.Context) {
	stats, err := h.queue.GetQueueStats(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to get queue stats", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get queue statistics"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"queue_stats": stats})
}

func (h *TaskHandler) GetWorkers(c *gin.Context) {
	workerModels, err := h.workerRepo.ListActive(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to list workers", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list workers"})
		return
	}

	workers := make([]types.Worker, len(workerModels))
	for i, workerModel := range workerModels {
		workers[i] = *workerModel.ToWorker()
	}

	c.JSON(http.StatusOK, gin.H{"workers": workers})
}

func (h *TaskHandler) RegisterWorker(c *gin.Context) {
	var req types.WorkerRegistration
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format", "details": err.Error()})
		return
	}

	workerModel := models.FromWorkerRegistration(&req)

	if err := h.workerRepo.Create(c.Request.Context(), workerModel); err != nil {
		h.logger.Error("Failed to register worker", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register worker"})
		return
	}

	worker := workerModel.ToWorker()

	h.logger.Info("Worker registered",
		zap.String("worker_id", worker.ID.String()),
		zap.String("worker_name", worker.Name))

	c.JSON(http.StatusCreated, gin.H{
		"worker": worker,
		"message": "Worker registered successfully",
	})
}

func (h *TaskHandler) UpdateWorkerHeartbeat(c *gin.Context) {
	var req types.WorkerHeartbeat
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format", "details": err.Error()})
		return
	}

	if err := h.workerRepo.UpdateHeartbeat(c.Request.Context(), req.WorkerID, req.Status); err != nil {
		h.logger.Error("Failed to update worker heartbeat", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update heartbeat"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Heartbeat updated"})
}

func (h *TaskHandler) HealthCheck(c *gin.Context) {
	status := h.healthCheck.CheckHealth(c.Request.Context())

	if status.Status == "healthy" {
		c.JSON(http.StatusOK, status)
	} else {
		c.JSON(http.StatusServiceUnavailable, status)
	}
}

func (h *TaskHandler) GetCircuitBreakerStats(c *gin.Context) {
	ctx := c.Request.Context()

	workers, err := h.workerRepo.ListActive(ctx)
	if err != nil {
		h.logger.Error("Failed to list active workers", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get circuit breaker stats"})
		return
	}

	circuitBreakerStats := make(map[string]map[string]interface{})
	
	for _, worker := range workers {
		circuitBreakerStats[worker.ID.String()] = map[string]interface{}{
			"worker_id":    worker.ID.String(),
			"worker_name":  worker.Name,
			"status":       worker.Status,
			"last_seen":    worker.LastSeen,
			"task_types":   worker.TaskTypes,
			"max_tasks":    worker.MaxTasks,
			"active_tasks": worker.ActiveTasks,
			"total_tasks":  worker.TotalTasks,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"circuit_breakers": circuitBreakerStats,
		"total_workers":    len(workers),
		"timestamp":        time.Now(),
	})
}

func (h *TaskHandler) GetWorkerPoolStats(c *gin.Context) {
	ctx := c.Request.Context()

	workers, err := h.workerRepo.ListActive(ctx)
	if err != nil {
		h.logger.Error("Failed to list active workers", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get worker pool stats"})
		return
	}

	queueStats, err := h.queue.GetQueueStats(ctx)
	if err != nil {
		h.logger.Error("Failed to get queue stats", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get queue stats"})
		return
	}

	totalPending := queueStats["total_pending"]

	activeWorkers := 0
	totalUtilization := 0.0
	workerDetails := make([]map[string]interface{}, 0)

	for _, worker := range workers {
		if worker.Status == types.WorkerStatusIdle || worker.Status == types.WorkerStatusBusy {
			activeWorkers++
		}

		utilization := 0.0
		if worker.MaxTasks > 0 {
			utilization = float64(worker.ActiveTasks) / float64(worker.MaxTasks) * 100
		}
		totalUtilization += utilization

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

		workerDetails = append(workerDetails, map[string]interface{}{
			"id":            worker.ID.String(),
			"name":          worker.Name,
			"status":        worker.Status,
			"active_tasks":  worker.ActiveTasks,
			"max_tasks":     worker.MaxTasks,
			"total_tasks":   worker.TotalTasks,
			"last_seen":     worker.LastSeen,
			"task_types":    taskTypes,
			"utilization":   utilization,
		})
	}

	avgUtilization := 0.0
	if len(workers) > 0 {
		avgUtilization = totalUtilization / float64(len(workers))
	}

	poolStats := map[string]interface{}{
		"active_workers":      activeWorkers,
		"total_workers":       len(workers),
		"queue_depth":         totalPending,
		"average_utilization": avgUtilization,
		"workers":             workerDetails,
		"timestamp":           time.Now(),
		"queue_breakdown": map[string]int64{
			"high_priority":    queueStats["high_priority"],
			"medium_priority":  queueStats["medium_priority"],
			"low_priority":     queueStats["low_priority"],
			"processing":       queueStats["processing"],
			"dead_letter":      queueStats["dead_letter"],
			"delayed":          queueStats["delayed"],
		},
		"auto_scaling": map[string]interface{}{
			"enabled":     false,
			"min_workers": 1,
			"max_workers": 10,
		},
	}

	c.JSON(http.StatusOK, poolStats)
}

func (h *TaskHandler) RedirectToMetrics(c *gin.Context) {
	c.Redirect(http.StatusMovedPermanently, "/metrics")
}

func MetricsMiddleware(metrics *monitoring.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		duration := time.Since(start)
		status := strconv.Itoa(c.Writer.Status())

		metrics.HTTPRequest(c.Request.Method, c.FullPath(), status, duration)
	}
} 

func (h *TaskHandler) SubmitBatchTasks(c *gin.Context) {
	var req struct {
		Tasks []types.TaskRequest `json:"tasks"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.Tasks) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no tasks provided"})
		return
	}

	if len(req.Tasks) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "batch size too large, maximum 100 tasks"})
		return
	}

	tasks := make([]*types.Task, 0, len(req.Tasks))
	taskIDs := make([]string, 0, len(req.Tasks))

	for _, taskReq := range req.Tasks {
		taskModel := models.FromTaskRequest(&taskReq)
		
		if err := h.taskRepo.Create(c.Request.Context(), taskModel); err != nil {
			h.logger.Error("Failed to create task in batch",
				zap.String("task_id", taskModel.ID.String()),
				zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create tasks"})
			return
		}

		task := taskModel.ToTask()
		tasks = append(tasks, task)
		taskIDs = append(taskIDs, task.ID.String())
	}

	ctx := c.Request.Context()

	if h.batchProcessor != nil {
		if err := h.batchProcessor.EnqueueBatch(ctx, tasks); err != nil {
			h.logger.Error("Failed to enqueue batch tasks", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enqueue tasks"})
			return
		}
	} else {
		for _, task := range tasks {
			if err := h.queue.Enqueue(ctx, task); err != nil {
				h.logger.Error("Failed to enqueue task in batch",
					zap.String("task_id", task.ID.String()),
					zap.Error(err))
			}
		}
	}

	h.logger.Info("Batch tasks submitted",
		zap.Int("task_count", len(tasks)),
		zap.Strings("task_ids", taskIDs))

	c.JSON(http.StatusCreated, gin.H{
		"message":    "batch tasks submitted successfully",
		"task_count": len(tasks),
		"task_ids":   taskIDs,
	})
}

func (h *TaskHandler) CreateTaskWithDependencies(c *gin.Context) {
	var req struct {
		types.TaskRequest
		Dependencies []string `json:"dependencies,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	taskModel := models.FromTaskRequest(&req.TaskRequest)
	
	dependencies := make([]uuid.UUID, 0, len(req.Dependencies))
	for _, depStr := range req.Dependencies {
		depID, err := uuid.Parse(depStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid dependency ID: %s", depStr)})
			return
		}
		dependencies = append(dependencies, depID)
	}

	ctx := c.Request.Context()

	if err := h.taskRepo.Create(ctx, taskModel); err != nil {
		h.logger.Error("Failed to create task with dependencies",
			zap.String("task_id", taskModel.ID.String()),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create task"})
		return
	}

	task := taskModel.ToTask()

	if h.dependencyManager != nil {
		if err := h.dependencyManager.AddTaskWithDependencies(ctx, task, dependencies); err != nil {
			h.logger.Error("Failed to add task with dependencies",
				zap.String("task_id", task.ID.String()),
				zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add task dependencies"})
			return
		}
	} else {
		if err := h.queue.Enqueue(ctx, task); err != nil {
			h.logger.Error("Failed to enqueue task",
				zap.String("task_id", task.ID.String()),
				zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enqueue task"})
			return
		}
	}

	h.logger.Info("Task with dependencies created",
		zap.String("task_id", task.ID.String()),
		zap.Int("dependency_count", len(dependencies)))

	c.JSON(http.StatusCreated, gin.H{
		"task": gin.H{
			"id":               task.ID,
			"type":             task.Type,
			"priority":         task.Priority,
			"status":           task.Status,
			"dependencies":     req.Dependencies,
			"dependency_count": len(dependencies),
			"created_at":       task.CreatedAt,
		},
	})
}

func (h *TaskHandler) ExecuteDAG(c *gin.Context) {
	var req struct {
		Name         string                      `json:"name" binding:"required"`
		Tasks        map[string]types.TaskRequest `json:"tasks" binding:"required"`
		Dependencies map[string][]string         `json:"dependencies,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.Tasks) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "DAG must contain at least one task"})
		return
	}

	dag := &queue.DAGExecution{
		ID:           uuid.New(),
		Name:         req.Name,
		Tasks:        make(map[uuid.UUID]*types.Task),
		Dependencies: make(map[uuid.UUID][]uuid.UUID),
		Status:       queue.DAGStatusPending,
	}

	taskIDMap := make(map[string]uuid.UUID)
	
	for taskKey, taskReq := range req.Tasks {
		taskModel := models.FromTaskRequest(&taskReq)
		task := taskModel.ToTask()

		dag.Tasks[task.ID] = task
		taskIDMap[taskKey] = task.ID
	}

	for taskKey, depKeys := range req.Dependencies {
		taskID, exists := taskIDMap[taskKey]
		if !exists {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("task not found: %s", taskKey)})
			return
		}

		dependencies := make([]uuid.UUID, 0, len(depKeys))
		for _, depKey := range depKeys {
			depID, exists := taskIDMap[depKey]
			if !exists {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("dependency task not found: %s", depKey)})
				return
			}
			dependencies = append(dependencies, depID)
		}

		dag.Dependencies[taskID] = dependencies
	}

	ctx := c.Request.Context()

	for _, task := range dag.Tasks {
		taskModel := &models.TaskModel{
			ID:          task.ID,
			Type:        task.Type,
			Payload:     models.JSONB(task.Payload),
			Priority:    task.Priority,
			Status:      task.Status,
			MaxRetries:  task.MaxRetries,
			RetryCount:  task.RetryCount,
			CreatedAt:   task.CreatedAt,
			UpdatedAt:   time.Now(),
		}
		if task.ScheduledAt != nil {
			taskModel.ScheduledAt = task.ScheduledAt
		}
		
		if err := h.taskRepo.Create(ctx, taskModel); err != nil {
			h.logger.Error("Failed to create DAG task",
				zap.String("task_id", task.ID.String()),
				zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create DAG tasks"})
			return
		}
	}

	if h.dependencyManager != nil {
		if err := h.dependencyManager.ExecuteDAG(ctx, dag); err != nil {
			h.logger.Error("Failed to execute DAG",
				zap.String("dag_id", dag.ID.String()),
				zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to execute DAG"})
			return
		}
	} else {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "dependency manager not available"})
		return
	}

	h.logger.Info("DAG execution started",
		zap.String("dag_id", dag.ID.String()),
		zap.String("dag_name", dag.Name),
		zap.Int("task_count", len(dag.Tasks)))

	c.JSON(http.StatusAccepted, gin.H{
		"dag": gin.H{
			"id":         dag.ID,
			"name":       dag.Name,
			"status":     dag.Status,
			"task_count": len(dag.Tasks),
			"start_time": dag.StartTime,
		},
	})
}

func (h *TaskHandler) GetBatchProcessorStats(c *gin.Context) {
	if h.batchProcessor == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "batch processor not available"})
		return
	}

	stats := h.batchProcessor.GetStats()
	c.JSON(http.StatusOK, gin.H{"batch_processor": stats})
}

func (h *TaskHandler) GetDependencyStats(c *gin.Context) {
	if h.dependencyManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "dependency manager not available"})
		return
	}

	stats := h.dependencyManager.GetDependencyStats()
	c.JSON(http.StatusOK, gin.H{"dependencies": stats})
}

func (h *TaskHandler) GetParallelExecutorStats(c *gin.Context) {
	if h.parallelExecutor == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "parallel executor not available"})
		return
	}

	stats := h.parallelExecutor.GetStats()
	activeJobs := h.parallelExecutor.GetActiveJobs()
	
	jobStats := make([]gin.H, 0, len(activeJobs))
	for _, job := range activeJobs {
		jobStats = append(jobStats, gin.H{
			"task_id":    job.Task.ID,
			"task_type":  job.Task.Type,
			"start_time": job.StartTime,
			"duration":   time.Since(job.StartTime),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"parallel_executor": stats,
		"active_jobs":       jobStats,
	})
} 