-- Initial schema for Distributed Task Queue System
-- Migration: 001_initial_schema.sql

-- Create extension for UUID generation
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Create custom types
CREATE TYPE task_status AS ENUM ('pending', 'running', 'completed', 'failed', 'cancelled');
CREATE TYPE task_priority AS ENUM ('low', 'medium', 'high');
CREATE TYPE worker_status AS ENUM ('idle', 'busy', 'offline');

-- Tasks table
CREATE TABLE tasks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    type VARCHAR(255) NOT NULL,
    payload JSONB NOT NULL,
    status task_status NOT NULL DEFAULT 'pending',
    priority task_priority NOT NULL DEFAULT 'medium',
    max_retries INTEGER NOT NULL DEFAULT 3,
    retry_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    worker_id VARCHAR(255),
    result JSONB,
    error TEXT,
    scheduled_at TIMESTAMP WITH TIME ZONE,
    timeout_secs INTEGER
);

-- Workers table
CREATE TABLE workers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL UNIQUE,
    status worker_status NOT NULL DEFAULT 'idle',
    last_seen TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    task_types JSONB NOT NULL,
    current_task UUID REFERENCES tasks(id),
    max_tasks INTEGER NOT NULL DEFAULT 1,
    active_tasks INTEGER NOT NULL DEFAULT 0,
    total_tasks BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    metadata JSONB
);

-- Worker metrics table (for storing historical performance data)
CREATE TABLE worker_metrics (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    worker_id UUID NOT NULL REFERENCES workers(id) ON DELETE CASCADE,
    tasks_completed BIGINT NOT NULL DEFAULT 0,
    tasks_failed BIGINT NOT NULL DEFAULT 0,
    average_exec_time FLOAT NOT NULL DEFAULT 0,
    last_task_duration FLOAT NOT NULL DEFAULT 0,
    cpu_usage FLOAT,
    memory_usage FLOAT,
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Task dependencies table (for future DAG support)
CREATE TABLE task_dependencies (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    depends_on_task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(task_id, depends_on_task_id)
);

-- Queue statistics table (for monitoring queue depth over time)
CREATE TABLE queue_stats (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    queue_name VARCHAR(255) NOT NULL,
    pending_count INTEGER NOT NULL DEFAULT 0,
    running_count INTEGER NOT NULL DEFAULT 0,
    completed_count INTEGER NOT NULL DEFAULT 0,
    failed_count INTEGER NOT NULL DEFAULT 0,
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Indexes for performance

-- Tasks indexes
CREATE INDEX idx_tasks_status ON tasks(status);
CREATE INDEX idx_tasks_priority ON tasks(priority);
CREATE INDEX idx_tasks_type ON tasks(type);
CREATE INDEX idx_tasks_created_at ON tasks(created_at);
CREATE INDEX idx_tasks_scheduled_at ON tasks(scheduled_at) WHERE scheduled_at IS NOT NULL;
CREATE INDEX idx_tasks_worker_id ON tasks(worker_id) WHERE worker_id IS NOT NULL;
CREATE INDEX idx_tasks_status_priority ON tasks(status, priority);

-- Workers indexes
CREATE INDEX idx_workers_status ON workers(status);
CREATE INDEX idx_workers_last_seen ON workers(last_seen);
CREATE INDEX idx_workers_name ON workers(name);

-- Worker metrics indexes
CREATE INDEX idx_worker_metrics_worker_id ON worker_metrics(worker_id);
CREATE INDEX idx_worker_metrics_timestamp ON worker_metrics(timestamp);

-- Queue stats indexes
CREATE INDEX idx_queue_stats_queue_name ON queue_stats(queue_name);
CREATE INDEX idx_queue_stats_timestamp ON queue_stats(timestamp);

-- Task dependencies indexes
CREATE INDEX idx_task_dependencies_task_id ON task_dependencies(task_id);
CREATE INDEX idx_task_dependencies_depends_on ON task_dependencies(depends_on_task_id);

-- Functions and triggers

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Triggers for updated_at
CREATE TRIGGER update_tasks_updated_at BEFORE UPDATE ON tasks
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_workers_updated_at BEFORE UPDATE ON workers
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Function to update worker task counts
CREATE OR REPLACE FUNCTION update_worker_task_count()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        -- Task assigned to worker
        IF NEW.status = 'running' AND NEW.worker_id IS NOT NULL THEN
            UPDATE workers 
            SET active_tasks = active_tasks + 1,
                current_task = NEW.id,
                last_seen = NOW()
            WHERE name = NEW.worker_id;
        END IF;
        RETURN NEW;
    ELSIF TG_OP = 'UPDATE' THEN
        -- Task status changed
        IF OLD.status = 'running' AND NEW.status IN ('completed', 'failed', 'cancelled') THEN
            -- Task completed/failed
            UPDATE workers 
            SET active_tasks = active_tasks - 1,
                total_tasks = total_tasks + 1,
                current_task = NULL,
                last_seen = NOW()
            WHERE name = NEW.worker_id;
        ELSIF OLD.status != 'running' AND NEW.status = 'running' AND NEW.worker_id IS NOT NULL THEN
            -- Task started
            UPDATE workers 
            SET active_tasks = active_tasks + 1,
                current_task = NEW.id,
                last_seen = NOW()
            WHERE name = NEW.worker_id;
        END IF;
        RETURN NEW;
    END IF;
    RETURN NULL;
END;
$$ language 'plpgsql';

-- Trigger for worker task count updates
CREATE TRIGGER update_worker_task_count_trigger
    AFTER INSERT OR UPDATE ON tasks
    FOR EACH ROW EXECUTE FUNCTION update_worker_task_count();

-- Views for monitoring

-- Current queue depth by priority
CREATE VIEW v_queue_depth AS
SELECT 
    priority,
    COUNT(*) as pending_count
FROM tasks 
WHERE status = 'pending'
GROUP BY priority;

-- Worker performance summary
CREATE VIEW v_worker_performance AS
SELECT 
    w.id,
    w.name,
    w.status,
    w.active_tasks,
    w.total_tasks,
    w.max_tasks,
    EXTRACT(EPOCH FROM (NOW() - w.last_seen)) as seconds_since_last_seen,
    CASE 
        WHEN w.max_tasks > 0 THEN (w.active_tasks::FLOAT / w.max_tasks::FLOAT) * 100
        ELSE 0 
    END as utilization_percent
FROM workers w;

-- Task processing statistics
CREATE VIEW v_task_stats AS
SELECT 
    type,
    COUNT(*) as total_tasks,
    COUNT(*) FILTER (WHERE status = 'pending') as pending,
    COUNT(*) FILTER (WHERE status = 'running') as running,
    COUNT(*) FILTER (WHERE status = 'completed') as completed,
    COUNT(*) FILTER (WHERE status = 'failed') as failed,
    COUNT(*) FILTER (WHERE status = 'cancelled') as cancelled,
    AVG(EXTRACT(EPOCH FROM (completed_at - started_at))) FILTER (WHERE status = 'completed') as avg_execution_time_seconds
FROM tasks
GROUP BY type;

-- Recent task activity (last 24 hours)
CREATE VIEW v_recent_activity AS
SELECT 
    DATE_TRUNC('hour', created_at) as hour,
    COUNT(*) as tasks_submitted,
    COUNT(*) FILTER (WHERE status = 'completed') as tasks_completed,
    COUNT(*) FILTER (WHERE status = 'failed') as tasks_failed
FROM tasks 
WHERE created_at >= NOW() - INTERVAL '24 hours'
GROUP BY DATE_TRUNC('hour', created_at)
ORDER BY hour; 