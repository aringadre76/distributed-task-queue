package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"distributed-task-queue/internal/models"
	"distributed-task-queue/pkg/types"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type TaskRepository interface {
	Create(ctx context.Context, task *models.TaskModel) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.TaskModel, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status types.TaskStatus, workerID *string) error
	UpdateResult(ctx context.Context, id uuid.UUID, result map[string]interface{}, err *string) error
	UpdateScheduledAt(ctx context.Context, id uuid.UUID, scheduledAt *time.Time) error
	ListPending(ctx context.Context, limit int) ([]*models.TaskModel, error)
	ListByStatus(ctx context.Context, status types.TaskStatus, page, pageSize int) ([]*models.TaskModel, int, error)
	IncrementRetryCount(ctx context.Context, id uuid.UUID) error
	GetDelayedTasks(ctx context.Context, before time.Time, limit int) ([]*types.Task, error)
	HealthCheck(ctx context.Context) error
}

type WorkerRepository interface {
	Create(ctx context.Context, worker *models.WorkerModel) error
	GetByName(ctx context.Context, name string) (*models.WorkerModel, error)
	UpdateHeartbeat(ctx context.Context, workerID uuid.UUID, status types.WorkerStatus) error
	ListActive(ctx context.Context) ([]*models.WorkerModel, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type taskRepository struct {
	db     *DB
	logger *zap.Logger
}

type workerRepository struct {
	db     *DB
	logger *zap.Logger
}

func NewTaskRepository(db *DB, logger *zap.Logger) TaskRepository {
	return &taskRepository{db: db, logger: logger}
}

func NewWorkerRepository(db *DB, logger *zap.Logger) WorkerRepository {
	return &workerRepository{db: db, logger: logger}
}

func (r *taskRepository) Create(ctx context.Context, task *models.TaskModel) error {
	query := `
		INSERT INTO tasks (id, type, payload, status, priority, max_retries, retry_count, 
		                  created_at, updated_at, scheduled_at, timeout_secs)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	_, err := r.db.ExecContext(ctx, query,
		task.ID, task.Type, task.Payload, task.Status, task.Priority,
		task.MaxRetries, task.RetryCount, task.CreatedAt, task.UpdatedAt,
		task.ScheduledAt, task.TimeoutSecs)

	if err != nil {
		r.logger.Error("Failed to create task", zap.Error(err), zap.String("task_id", task.ID.String()))
		return fmt.Errorf("failed to create task: %w", err)
	}

	r.logger.Info("Task created", zap.String("task_id", task.ID.String()), zap.String("type", task.Type))
	return nil
}

func (r *taskRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.TaskModel, error) {
	query := `
		SELECT id, type, payload, status, priority, max_retries, retry_count,
		       created_at, updated_at, started_at, completed_at, worker_id,
		       result, error, scheduled_at, timeout_secs
		FROM tasks WHERE id = $1`

	task := &models.TaskModel{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&task.ID, &task.Type, &task.Payload, &task.Status, &task.Priority,
		&task.MaxRetries, &task.RetryCount, &task.CreatedAt, &task.UpdatedAt,
		&task.StartedAt, &task.CompletedAt, &task.WorkerID, &task.Result,
		&task.Error, &task.ScheduledAt, &task.TimeoutSecs)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("task not found: %s", id)
		}
		r.logger.Error("Failed to get task", zap.Error(err), zap.String("task_id", id.String()))
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	return task, nil
}

func (r *taskRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status types.TaskStatus, workerID *string) error {
	var query string
	var args []interface{}

	now := time.Now()

	switch status {
	case types.TaskStatusRunning:
		query = `UPDATE tasks SET status = $1, worker_id = $2, started_at = $3, updated_at = $4 WHERE id = $5`
		args = []interface{}{status, workerID, now, now, id}
	case types.TaskStatusCompleted, types.TaskStatusFailed, types.TaskStatusCancelled:
		query = `UPDATE tasks SET status = $1, completed_at = $2, updated_at = $3 WHERE id = $4`
		args = []interface{}{status, now, now, id}
	default:
		query = `UPDATE tasks SET status = $1, updated_at = $2 WHERE id = $3`
		args = []interface{}{status, now, id}
	}

	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		r.logger.Error("Failed to update task status", zap.Error(err), zap.String("task_id", id.String()))
		return fmt.Errorf("failed to update task status: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("task not found: %s", id)
	}

	r.logger.Info("Task status updated", 
		zap.String("task_id", id.String()), 
		zap.String("status", string(status)))
	return nil
}

func (r *taskRepository) UpdateResult(ctx context.Context, id uuid.UUID, result map[string]interface{}, errMsg *string) error {
	query := `UPDATE tasks SET result = $1, error = $2, updated_at = $3 WHERE id = $4`
	
	now := time.Now()
	_, err := r.db.ExecContext(ctx, query, models.JSONB(result), errMsg, now, id)
	if err != nil {
		r.logger.Error("Failed to update task result", zap.Error(err), zap.String("task_id", id.String()))
		return fmt.Errorf("failed to update task result: %w", err)
	}

	return nil
}

func (r *taskRepository) UpdateScheduledAt(ctx context.Context, id uuid.UUID, scheduledAt *time.Time) error {
	query := `UPDATE tasks SET scheduled_at = $1, updated_at = $2 WHERE id = $3`
	
	now := time.Now()
	_, err := r.db.ExecContext(ctx, query, scheduledAt, now, id)
	if err != nil {
		r.logger.Error("Failed to update task scheduled_at", zap.Error(err), zap.String("task_id", id.String()))
		return fmt.Errorf("failed to update task scheduled_at: %w", err)
	}

	r.logger.Debug("Task scheduled_at updated", 
		zap.String("task_id", id.String()), 
		zap.Any("scheduled_at", scheduledAt))
	return nil
}

func (r *taskRepository) ListPending(ctx context.Context, limit int) ([]*models.TaskModel, error) {
	query := `
		SELECT id, type, payload, status, priority, max_retries, retry_count,
		       created_at, updated_at, started_at, completed_at, worker_id,
		       result, error, scheduled_at, timeout_secs
		FROM tasks 
		WHERE status = 'pending' 
		  AND (scheduled_at IS NULL OR scheduled_at <= NOW())
		ORDER BY priority DESC, created_at ASC
		LIMIT $1`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		r.logger.Error("Failed to list pending tasks", zap.Error(err))
		return nil, fmt.Errorf("failed to list pending tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*models.TaskModel
	for rows.Next() {
		task := &models.TaskModel{}
		err := rows.Scan(
			&task.ID, &task.Type, &task.Payload, &task.Status, &task.Priority,
			&task.MaxRetries, &task.RetryCount, &task.CreatedAt, &task.UpdatedAt,
			&task.StartedAt, &task.CompletedAt, &task.WorkerID, &task.Result,
			&task.Error, &task.ScheduledAt, &task.TimeoutSecs)
		if err != nil {
			r.logger.Error("Failed to scan task", zap.Error(err))
			return nil, fmt.Errorf("failed to scan task: %w", err)
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}

func (r *taskRepository) ListByStatus(ctx context.Context, status types.TaskStatus, page, pageSize int) ([]*models.TaskModel, int, error) {
	offset := (page - 1) * pageSize

	countQuery := `SELECT COUNT(*) FROM tasks WHERE status = $1`
	var total int
	err := r.db.QueryRowContext(ctx, countQuery, status).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count tasks: %w", err)
	}

	query := `
		SELECT id, type, payload, status, priority, max_retries, retry_count,
		       created_at, updated_at, started_at, completed_at, worker_id,
		       result, error, scheduled_at, timeout_secs
		FROM tasks 
		WHERE status = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.db.QueryContext(ctx, query, status, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*models.TaskModel
	for rows.Next() {
		task := &models.TaskModel{}
		err := rows.Scan(
			&task.ID, &task.Type, &task.Payload, &task.Status, &task.Priority,
			&task.MaxRetries, &task.RetryCount, &task.CreatedAt, &task.UpdatedAt,
			&task.StartedAt, &task.CompletedAt, &task.WorkerID, &task.Result,
			&task.Error, &task.ScheduledAt, &task.TimeoutSecs)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan task: %w", err)
		}
		tasks = append(tasks, task)
	}

	return tasks, total, nil
}

func (r *taskRepository) IncrementRetryCount(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE tasks SET retry_count = retry_count + 1, updated_at = NOW() WHERE id = $1`
	
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		r.logger.Error("Failed to increment retry count", zap.Error(err), zap.String("task_id", id.String()))
		return fmt.Errorf("failed to increment retry count: %w", err)
	}

	return nil
}

func (r *taskRepository) GetDelayedTasks(ctx context.Context, before time.Time, limit int) ([]*types.Task, error) {
	query := `
		SELECT id, type, payload, priority, status, worker_id, retry_count, max_retries, 
		       timeout_secs, created_at, scheduled_at, started_at, completed_at, result
		FROM tasks 
		WHERE status = $1 AND scheduled_at <= $2
		ORDER BY scheduled_at ASC
		LIMIT $3`

	rows, err := r.db.QueryContext(ctx, query, types.TaskStatusPending, before, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query delayed tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*types.Task
	for rows.Next() {
		dbTask := &models.TaskModel{}

		err := rows.Scan(
			&dbTask.ID,
			&dbTask.Type,
			&dbTask.Payload,
			&dbTask.Priority,
			&dbTask.Status,
			&dbTask.WorkerID,
			&dbTask.RetryCount,
			&dbTask.MaxRetries,
			&dbTask.TimeoutSecs,
			&dbTask.CreatedAt,
			&dbTask.ScheduledAt,
			&dbTask.StartedAt,
			&dbTask.CompletedAt,
			&dbTask.Result,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan delayed task: %w", err)
		}

		task := dbTask.ToTask()
		tasks = append(tasks, task)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating delayed tasks: %w", err)
	}

	return tasks, nil
}

func (r *taskRepository) HealthCheck(ctx context.Context) error {
	return r.db.PingContext(ctx)
}

func (r *workerRepository) Create(ctx context.Context, worker *models.WorkerModel) error {
	query := `
		INSERT INTO workers (id, name, status, last_seen, task_types, max_tasks, 
		                    active_tasks, total_tasks, created_at, updated_at, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	_, err := r.db.ExecContext(ctx, query,
		worker.ID, worker.Name, worker.Status, worker.LastSeen, worker.TaskTypes,
		worker.MaxTasks, worker.ActiveTasks, worker.TotalTasks,
		worker.CreatedAt, worker.UpdatedAt, worker.Metadata)

	if err != nil {
		r.logger.Error("Failed to create worker", zap.Error(err), zap.String("worker_name", worker.Name))
		return fmt.Errorf("failed to create worker: %w", err)
	}

	r.logger.Info("Worker registered", zap.String("worker_name", worker.Name))
	return nil
}

func (r *workerRepository) GetByName(ctx context.Context, name string) (*models.WorkerModel, error) {
	query := `
		SELECT id, name, status, last_seen, task_types, current_task, max_tasks,
		       active_tasks, total_tasks, created_at, updated_at, metadata
		FROM workers WHERE name = $1`

	worker := &models.WorkerModel{}
	err := r.db.QueryRowContext(ctx, query, name).Scan(
		&worker.ID, &worker.Name, &worker.Status, &worker.LastSeen, &worker.TaskTypes,
		&worker.CurrentTask, &worker.MaxTasks, &worker.ActiveTasks, &worker.TotalTasks,
		&worker.CreatedAt, &worker.UpdatedAt, &worker.Metadata)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("worker not found: %s", name)
		}
		r.logger.Error("Failed to get worker", zap.Error(err), zap.String("worker_name", name))
		return nil, fmt.Errorf("failed to get worker: %w", err)
	}

	return worker, nil
}

func (r *workerRepository) UpdateHeartbeat(ctx context.Context, workerID uuid.UUID, status types.WorkerStatus) error {
	query := `UPDATE workers SET status = $1, last_seen = NOW(), updated_at = NOW() WHERE id = $2`
	
	_, err := r.db.ExecContext(ctx, query, status, workerID)
	if err != nil {
		r.logger.Error("Failed to update worker heartbeat", zap.Error(err), zap.String("worker_id", workerID.String()))
		return fmt.Errorf("failed to update worker heartbeat: %w", err)
	}

	return nil
}

func (r *workerRepository) ListActive(ctx context.Context) ([]*models.WorkerModel, error) {
	query := `
		SELECT id, name, status, last_seen, task_types, current_task, max_tasks,
		       active_tasks, total_tasks, created_at, updated_at, metadata
		FROM workers 
		WHERE status IN ('idle', 'busy') 
		  AND last_seen > NOW() - INTERVAL '5 minutes'
		ORDER BY last_seen DESC`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		r.logger.Error("Failed to list active workers", zap.Error(err))
		return nil, fmt.Errorf("failed to list active workers: %w", err)
	}
	defer rows.Close()

	var workers []*models.WorkerModel
	for rows.Next() {
		worker := &models.WorkerModel{}
		err := rows.Scan(
			&worker.ID, &worker.Name, &worker.Status, &worker.LastSeen, &worker.TaskTypes,
			&worker.CurrentTask, &worker.MaxTasks, &worker.ActiveTasks, &worker.TotalTasks,
			&worker.CreatedAt, &worker.UpdatedAt, &worker.Metadata)
		if err != nil {
			r.logger.Error("Failed to scan worker", zap.Error(err))
			return nil, fmt.Errorf("failed to scan worker: %w", err)
		}
		workers = append(workers, worker)
	}

	return workers, nil
}

func (r *workerRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM workers WHERE id = $1`
	
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		r.logger.Error("Failed to delete worker", zap.Error(err), zap.String("worker_id", id.String()))
		return fmt.Errorf("failed to delete worker: %w", err)
	}

	return nil
} 