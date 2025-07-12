package types

import (
	"time"

	"github.com/google/uuid"
)

// WorkerStatus represents the current state of a worker
type WorkerStatus string

const (
	WorkerStatusIdle    WorkerStatus = "idle"
	WorkerStatusBusy    WorkerStatus = "busy"
	WorkerStatusOffline WorkerStatus = "offline"
)

// Worker represents a worker node in the system
type Worker struct {
	ID         uuid.UUID    `json:"id" db:"id"`
	Name       string       `json:"name" db:"name"`
	Status     WorkerStatus `json:"status" db:"status"`
	LastSeen   time.Time    `json:"last_seen" db:"last_seen"`
	TaskTypes  []string     `json:"task_types" db:"task_types"`
	CurrentTask *uuid.UUID  `json:"current_task,omitempty" db:"current_task"`
	MaxTasks   int          `json:"max_tasks" db:"max_tasks"`
	ActiveTasks int         `json:"active_tasks" db:"active_tasks"`
	TotalTasks  int64       `json:"total_tasks" db:"total_tasks"`
	CreatedAt   time.Time    `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at" db:"updated_at"`
	Metadata    map[string]interface{} `json:"metadata,omitempty" db:"metadata"`
}

// WorkerMetrics represents performance metrics for a worker
type WorkerMetrics struct {
	WorkerID         uuid.UUID `json:"worker_id"`
	TasksCompleted   int64     `json:"tasks_completed"`
	TasksFailed      int64     `json:"tasks_failed"`
	AverageExecTime  float64   `json:"average_exec_time"`
	LastTaskDuration float64   `json:"last_task_duration"`
	CPUUsage         float64   `json:"cpu_usage,omitempty"`
	MemoryUsage      float64   `json:"memory_usage,omitempty"`
	Timestamp        time.Time `json:"timestamp"`
}

// WorkerRegistration represents a worker registration request
type WorkerRegistration struct {
	Name      string   `json:"name" binding:"required"`
	TaskTypes []string `json:"task_types" binding:"required"`
	MaxTasks  int      `json:"max_tasks" binding:"required"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// WorkerHeartbeat represents a worker heartbeat
type WorkerHeartbeat struct {
	WorkerID    uuid.UUID    `json:"worker_id" binding:"required"`
	Status      WorkerStatus `json:"status" binding:"required"`
	ActiveTasks int          `json:"active_tasks"`
	CurrentTask *uuid.UUID   `json:"current_task,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
} 