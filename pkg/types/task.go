package types

import (
	"time"

	"github.com/google/uuid"
)

// TaskStatus represents the current state of a task
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

// TaskPriority represents task execution priority
type TaskPriority string

const (
	PriorityLow    TaskPriority = "low"
	PriorityMedium TaskPriority = "medium"
	PriorityHigh   TaskPriority = "high"
)

// Task represents a unit of work in the queue
type Task struct {
	ID          uuid.UUID              `json:"id" db:"id"`
	Type        string                 `json:"type" db:"type"`
	Payload     map[string]interface{} `json:"payload" db:"payload"`
	Status      TaskStatus             `json:"status" db:"status"`
	Priority    TaskPriority           `json:"priority" db:"priority"`
	MaxRetries  int                    `json:"max_retries" db:"max_retries"`
	RetryCount  int                    `json:"retry_count" db:"retry_count"`
	CreatedAt   time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at" db:"updated_at"`
	StartedAt   *time.Time             `json:"started_at,omitempty" db:"started_at"`
	CompletedAt *time.Time             `json:"completed_at,omitempty" db:"completed_at"`
	WorkerID    *string                `json:"worker_id,omitempty" db:"worker_id"`
	Result      map[string]interface{} `json:"result,omitempty" db:"result"`
	Error       *string                `json:"error,omitempty" db:"error"`
	ScheduledAt *time.Time             `json:"scheduled_at,omitempty" db:"scheduled_at"`
	Timeout     *time.Duration         `json:"timeout,omitempty" db:"timeout"`
}

// TaskRequest represents a request to create a new task
type TaskRequest struct {
	Type        string                 `json:"type" binding:"required"`
	Payload     map[string]interface{} `json:"payload" binding:"required"`
	Priority    TaskPriority           `json:"priority,omitempty"`
	MaxRetries  int                    `json:"max_retries,omitempty"`
	ScheduledAt *time.Time             `json:"scheduled_at,omitempty"`
	Timeout     *time.Duration         `json:"timeout,omitempty"`
}

// TaskResponse represents the response when querying task status
type TaskResponse struct {
	Task    Task   `json:"task"`
	Message string `json:"message,omitempty"`
}

// TaskListResponse represents a paginated list of tasks
type TaskListResponse struct {
	Tasks      []Task `json:"tasks"`
	Total      int    `json:"total"`
	Page       int    `json:"page"`
	PageSize   int    `json:"page_size"`
	TotalPages int    `json:"total_pages"`
} 