package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"distributed-task-queue/pkg/types"
	"github.com/google/uuid"
)

// JSONB represents a PostgreSQL JSONB field
type JSONB map[string]interface{}

// Value implements the driver.Valuer interface for JSONB
func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan implements the sql.Scanner interface for JSONB
func (j *JSONB) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("cannot scan into JSONB")
	}

	return json.Unmarshal(bytes, j)
}

// TaskModel represents the database model for tasks
type TaskModel struct {
	ID          uuid.UUID         `db:"id"`
	Type        string            `db:"type"`
	Payload     JSONB             `db:"payload"`
	Status      types.TaskStatus  `db:"status"`
	Priority    types.TaskPriority `db:"priority"`
	MaxRetries  int               `db:"max_retries"`
	RetryCount  int               `db:"retry_count"`
	CreatedAt   time.Time         `db:"created_at"`
	UpdatedAt   time.Time         `db:"updated_at"`
	StartedAt   *time.Time        `db:"started_at"`
	CompletedAt *time.Time        `db:"completed_at"`
	WorkerID    *string           `db:"worker_id"`
	Result      JSONB             `db:"result"`
	Error       *string           `db:"error"`
	ScheduledAt *time.Time        `db:"scheduled_at"`
	TimeoutSecs *int              `db:"timeout_secs"`
}

// ToTask converts TaskModel to types.Task
func (tm *TaskModel) ToTask() *types.Task {
	task := &types.Task{
		ID:          tm.ID,
		Type:        tm.Type,
		Payload:     map[string]interface{}(tm.Payload),
		Status:      tm.Status,
		Priority:    tm.Priority,
		MaxRetries:  tm.MaxRetries,
		RetryCount:  tm.RetryCount,
		CreatedAt:   tm.CreatedAt,
		UpdatedAt:   tm.UpdatedAt,
		StartedAt:   tm.StartedAt,
		CompletedAt: tm.CompletedAt,
		WorkerID:    tm.WorkerID,
		Result:      map[string]interface{}(tm.Result),
		Error:       tm.Error,
		ScheduledAt: tm.ScheduledAt,
	}

	// Convert timeout from seconds to duration
	if tm.TimeoutSecs != nil {
		timeout := time.Duration(*tm.TimeoutSecs) * time.Second
		task.Timeout = &timeout
	}

	return task
}

// FromTaskRequest creates a TaskModel from types.TaskRequest
func FromTaskRequest(req *types.TaskRequest) *TaskModel {
	now := time.Now()
	
	// Set defaults
	priority := req.Priority
	if priority == "" {
		priority = types.PriorityMedium
	}
	
	maxRetries := req.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}

	task := &TaskModel{
		ID:         uuid.New(),
		Type:       req.Type,
		Payload:    JSONB(req.Payload),
		Status:     types.TaskStatusPending,
		Priority:   priority,
		MaxRetries: maxRetries,
		RetryCount: 0,
		CreatedAt:  now,
		UpdatedAt:  now,
		ScheduledAt: req.ScheduledAt,
	}

	// Convert timeout from duration to seconds
	if req.Timeout != nil {
		timeoutSecs := int(req.Timeout.Seconds())
		task.TimeoutSecs = &timeoutSecs
	}

	return task
}

// WorkerModel represents the database model for workers
type WorkerModel struct {
	ID          uuid.UUID           `db:"id"`
	Name        string              `db:"name"`
	Status      types.WorkerStatus  `db:"status"`
	LastSeen    time.Time           `db:"last_seen"`
	TaskTypes   JSONB               `db:"task_types"` // []string stored as JSON
	CurrentTask *uuid.UUID          `db:"current_task"`
	MaxTasks    int                 `db:"max_tasks"`
	ActiveTasks int                 `db:"active_tasks"`
	TotalTasks  int64               `db:"total_tasks"`
	CreatedAt   time.Time           `db:"created_at"`
	UpdatedAt   time.Time           `db:"updated_at"`
	Metadata    JSONB               `db:"metadata"`
}

// ToWorker converts WorkerModel to types.Worker
func (wm *WorkerModel) ToWorker() *types.Worker {
	worker := &types.Worker{
		ID:          wm.ID,
		Name:        wm.Name,
		Status:      wm.Status,
		LastSeen:    wm.LastSeen,
		CurrentTask: wm.CurrentTask,
		MaxTasks:    wm.MaxTasks,
		ActiveTasks: wm.ActiveTasks,
		TotalTasks:  wm.TotalTasks,
		CreatedAt:   wm.CreatedAt,
		UpdatedAt:   wm.UpdatedAt,
		Metadata:    map[string]interface{}(wm.Metadata),
	}

	// Convert task types from JSONB to []string
	if wm.TaskTypes != nil {
		if taskTypes, ok := wm.TaskTypes["types"].([]interface{}); ok {
			worker.TaskTypes = make([]string, len(taskTypes))
			for i, t := range taskTypes {
				if str, ok := t.(string); ok {
					worker.TaskTypes[i] = str
				}
			}
		}
	}

	return worker
}

// FromWorkerRegistration creates a WorkerModel from types.WorkerRegistration
func FromWorkerRegistration(reg *types.WorkerRegistration) *WorkerModel {
	now := time.Now()
	
	// Convert task types to JSONB format
	taskTypesJSON := JSONB{"types": reg.TaskTypes}

	return &WorkerModel{
		ID:          uuid.New(),
		Name:        reg.Name,
		Status:      types.WorkerStatusIdle,
		LastSeen:    now,
		TaskTypes:   taskTypesJSON,
		MaxTasks:    reg.MaxTasks,
		ActiveTasks: 0,
		TotalTasks:  0,
		CreatedAt:   now,
		UpdatedAt:   now,
		Metadata:    JSONB(reg.Metadata),
	}
} 