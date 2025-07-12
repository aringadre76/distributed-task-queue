package queue

import (
	"context"
	"fmt"
	"sync"
	"time"

	"distributed-task-queue/pkg/types"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type DependencyManager struct {
	dependencies map[uuid.UUID][]uuid.UUID
	dependents   map[uuid.UUID][]uuid.UUID
	taskStatus   map[uuid.UUID]types.TaskStatus
	taskData     map[uuid.UUID]*types.Task
	mutex        sync.RWMutex
	logger       *zap.Logger
	queue        *RedisQueue
}

type TaskDependency struct {
	TaskID       uuid.UUID   `json:"task_id"`
	Dependencies []uuid.UUID `json:"dependencies"`
}

type DAGExecution struct {
	ID            uuid.UUID                        `json:"id"`
	Name          string                           `json:"name"`
	Tasks         map[uuid.UUID]*types.Task        `json:"tasks"`
	Dependencies  map[uuid.UUID][]uuid.UUID        `json:"dependencies"`
	Status        DAGStatus                        `json:"status"`
	StartTime     time.Time                        `json:"start_time"`
	EndTime       *time.Time                       `json:"end_time,omitempty"`
	ExecutionPath []uuid.UUID                      `json:"execution_path"`
	Results       map[uuid.UUID]map[string]interface{} `json:"results"`
}

type DAGStatus string

const (
	DAGStatusPending   DAGStatus = "pending"
	DAGStatusRunning   DAGStatus = "running"
	DAGStatusCompleted DAGStatus = "completed"
	DAGStatusFailed    DAGStatus = "failed"
	DAGStatusCancelled DAGStatus = "cancelled"
)

func NewDependencyManager(queue *RedisQueue, logger *zap.Logger) *DependencyManager {
	return &DependencyManager{
		dependencies: make(map[uuid.UUID][]uuid.UUID),
		dependents:   make(map[uuid.UUID][]uuid.UUID),
		taskStatus:   make(map[uuid.UUID]types.TaskStatus),
		taskData:     make(map[uuid.UUID]*types.Task),
		logger:       logger,
		queue:        queue,
	}
}

func (dm *DependencyManager) AddTaskWithDependencies(ctx context.Context, task *types.Task, dependencies []uuid.UUID) error {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()

	if err := dm.validateDependencies(task.ID, dependencies); err != nil {
		return fmt.Errorf("invalid dependencies: %w", err)
	}

	dm.taskData[task.ID] = task
	dm.taskStatus[task.ID] = types.TaskStatusPending
	dm.dependencies[task.ID] = dependencies

	for _, depID := range dependencies {
		if dm.dependents[depID] == nil {
			dm.dependents[depID] = make([]uuid.UUID, 0)
		}
		dm.dependents[depID] = append(dm.dependents[depID], task.ID)
	}

	dm.logger.Info("Task added with dependencies",
		zap.String("task_id", task.ID.String()),
		zap.Int("dependency_count", len(dependencies)))

	if dm.canExecuteTask(task.ID) {
		return dm.enqueueTask(ctx, task)
	}

	return nil
}

func (dm *DependencyManager) CompleteTask(ctx context.Context, taskID uuid.UUID, result map[string]interface{}) error {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()

	dm.taskStatus[taskID] = types.TaskStatusCompleted

	readyTasks := make([]*types.Task, 0)
	for _, dependentID := range dm.dependents[taskID] {
		if dm.canExecuteTask(dependentID) {
			if task, exists := dm.taskData[dependentID]; exists {
				readyTasks = append(readyTasks, task)
			}
		}
	}

	for _, task := range readyTasks {
		if err := dm.enqueueTask(ctx, task); err != nil {
			dm.logger.Error("Failed to enqueue dependent task",
				zap.String("task_id", task.ID.String()),
				zap.Error(err))
		}
	}

	dm.logger.Info("Task completed, ready tasks enqueued",
		zap.String("completed_task", taskID.String()),
		zap.Int("ready_tasks", len(readyTasks)))

	return nil
}

func (dm *DependencyManager) FailTask(ctx context.Context, taskID uuid.UUID, err error) error {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()

	dm.taskStatus[taskID] = types.TaskStatusFailed

	failedTasks := dm.getAllDependents(taskID)
	for _, dependentID := range failedTasks {
		dm.taskStatus[dependentID] = types.TaskStatusFailed
	}

	dm.logger.Warn("Task failed, cascading failure to dependents",
		zap.String("failed_task", taskID.String()),
		zap.Int("cascaded_failures", len(failedTasks)),
		zap.Error(err))

	return nil
}

func (dm *DependencyManager) ExecuteDAG(ctx context.Context, dag *DAGExecution) error {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()

	if err := dm.validateDAG(dag); err != nil {
		return fmt.Errorf("invalid DAG: %w", err)
	}

	dag.Status = DAGStatusRunning
	dag.StartTime = time.Now()
	dag.ExecutionPath = make([]uuid.UUID, 0)
	dag.Results = make(map[uuid.UUID]map[string]interface{})

	for taskID, task := range dag.Tasks {
		dm.taskData[taskID] = task
		dm.taskStatus[taskID] = types.TaskStatusPending
		if deps, exists := dag.Dependencies[taskID]; exists {
			dm.dependencies[taskID] = deps
			for _, depID := range deps {
				if dm.dependents[depID] == nil {
					dm.dependents[depID] = make([]uuid.UUID, 0)
				}
				dm.dependents[depID] = append(dm.dependents[depID], taskID)
			}
		}
	}

	entryTasks := dm.findEntryTasks(dag)
	for _, task := range entryTasks {
		if err := dm.enqueueTask(ctx, task); err != nil {
			return fmt.Errorf("failed to enqueue entry task: %w", err)
		}
	}

	dm.logger.Info("DAG execution started",
		zap.String("dag_id", dag.ID.String()),
		zap.String("dag_name", dag.Name),
		zap.Int("total_tasks", len(dag.Tasks)),
		zap.Int("entry_tasks", len(entryTasks)))

	return nil
}

func (dm *DependencyManager) GetTaskStatus(taskID uuid.UUID) (types.TaskStatus, bool) {
	dm.mutex.RLock()
	defer dm.mutex.RUnlock()
	
	status, exists := dm.taskStatus[taskID]
	return status, exists
}

func (dm *DependencyManager) GetDependencyStats() map[string]interface{} {
	dm.mutex.RLock()
	defer dm.mutex.RUnlock()

	statusCounts := make(map[types.TaskStatus]int)
	for _, status := range dm.taskStatus {
		statusCounts[status]++
	}

	return map[string]interface{}{
		"total_tasks":      len(dm.taskData),
		"status_breakdown": statusCounts,
		"dependency_count": len(dm.dependencies),
		"dependent_count":  len(dm.dependents),
	}
}

func (dm *DependencyManager) canExecuteTask(taskID uuid.UUID) bool {
	dependencies, exists := dm.dependencies[taskID]
	if !exists {
		return true
	}

	for _, depID := range dependencies {
		if status, exists := dm.taskStatus[depID]; !exists || status != types.TaskStatusCompleted {
			return false
		}
	}

	return true
}

func (dm *DependencyManager) enqueueTask(ctx context.Context, task *types.Task) error {
	dm.taskStatus[task.ID] = types.TaskStatusPending
	return dm.queue.Enqueue(ctx, task)
}

func (dm *DependencyManager) validateDependencies(taskID uuid.UUID, dependencies []uuid.UUID) error {
	for _, depID := range dependencies {
		if depID == taskID {
			return fmt.Errorf("task cannot depend on itself")
		}
	}

	visited := make(map[uuid.UUID]bool)
	if dm.hasCycle(taskID, dependencies, visited) {
		return fmt.Errorf("circular dependency detected")
	}

	return nil
}

func (dm *DependencyManager) hasCycle(taskID uuid.UUID, dependencies []uuid.UUID, visited map[uuid.UUID]bool) bool {
	if visited[taskID] {
		return true
	}

	visited[taskID] = true

	for _, depID := range dependencies {
		if depDependencies, exists := dm.dependencies[depID]; exists {
			if dm.hasCycle(depID, depDependencies, visited) {
				return true
			}
		}
	}

	visited[taskID] = false
	return false
}

func (dm *DependencyManager) getAllDependents(taskID uuid.UUID) []uuid.UUID {
	result := make([]uuid.UUID, 0)
	visited := make(map[uuid.UUID]bool)
	dm.collectDependents(taskID, &result, visited)
	return result
}

func (dm *DependencyManager) collectDependents(taskID uuid.UUID, result *[]uuid.UUID, visited map[uuid.UUID]bool) {
	if visited[taskID] {
		return
	}
	visited[taskID] = true

	for _, dependentID := range dm.dependents[taskID] {
		*result = append(*result, dependentID)
		dm.collectDependents(dependentID, result, visited)
	}
}

func (dm *DependencyManager) validateDAG(dag *DAGExecution) error {
	if len(dag.Tasks) == 0 {
		return fmt.Errorf("DAG must contain at least one task")
	}

	for taskID := range dag.Tasks {
		if dependencies, exists := dag.Dependencies[taskID]; exists {
			for _, depID := range dependencies {
				if _, exists := dag.Tasks[depID]; !exists {
					return fmt.Errorf("dependency task %s not found in DAG", depID.String())
				}
			}
		}
	}

	visited := make(map[uuid.UUID]bool)
	for taskID := range dag.Tasks {
		if dm.hasDAGCycle(taskID, dag.Dependencies, visited) {
			return fmt.Errorf("DAG contains cycles")
		}
	}

	return nil
}

func (dm *DependencyManager) hasDAGCycle(taskID uuid.UUID, dependencies map[uuid.UUID][]uuid.UUID, visited map[uuid.UUID]bool) bool {
	if visited[taskID] {
		return true
	}

	visited[taskID] = true

	if deps, exists := dependencies[taskID]; exists {
		for _, depID := range deps {
			if dm.hasDAGCycle(depID, dependencies, visited) {
				return true
			}
		}
	}

	visited[taskID] = false
	return false
}

func (dm *DependencyManager) findEntryTasks(dag *DAGExecution) []*types.Task {
	entryTasks := make([]*types.Task, 0)
	
	for taskID, task := range dag.Tasks {
		if dependencies, exists := dag.Dependencies[taskID]; !exists || len(dependencies) == 0 {
			entryTasks = append(entryTasks, task)
		}
	}
	
	return entryTasks
} 