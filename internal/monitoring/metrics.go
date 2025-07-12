package monitoring

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

type Metrics struct {
	// Task metrics
	TasksSubmitted   prometheus.Counter
	TasksCompleted   prometheus.Counter
	TasksFailed      prometheus.Counter
	TasksRetried     prometheus.Counter
	TaskDuration     prometheus.Histogram
	TasksInQueue     *prometheus.GaugeVec
	TasksProcessing  prometheus.Gauge

	// Worker metrics
	WorkersActive    prometheus.Gauge
	WorkerTasksTotal *prometheus.CounterVec
	WorkerUptime     *prometheus.GaugeVec

	// Queue metrics
	QueueDepth       *prometheus.GaugeVec
	QueueThroughput  *prometheus.CounterVec

	// System metrics
	HTTPRequests     *prometheus.CounterVec
	HTTPDuration     *prometheus.HistogramVec
	DatabaseConns    *prometheus.GaugeVec
	RedisConns       prometheus.Gauge

	logger *zap.Logger
	server *http.Server
}

func NewMetrics(logger *zap.Logger) *Metrics {
	return &Metrics{
		// Task metrics
		TasksSubmitted: promauto.NewCounter(prometheus.CounterOpts{
			Name: "dtq_tasks_submitted_total",
			Help: "Total number of tasks submitted",
		}),
		TasksCompleted: promauto.NewCounter(prometheus.CounterOpts{
			Name: "dtq_tasks_completed_total",
			Help: "Total number of tasks completed successfully",
		}),
		TasksFailed: promauto.NewCounter(prometheus.CounterOpts{
			Name: "dtq_tasks_failed_total",
			Help: "Total number of tasks that failed",
		}),
		TasksRetried: promauto.NewCounter(prometheus.CounterOpts{
			Name: "dtq_tasks_retried_total",
			Help: "Total number of task retries",
		}),
		TaskDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "dtq_task_duration_seconds",
			Help:    "Task execution duration in seconds",
			Buckets: prometheus.ExponentialBuckets(0.1, 2, 10),
		}),
		TasksInQueue: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "dtq_tasks_in_queue",
			Help: "Number of tasks in queue by priority",
		}, []string{"priority"}),
		TasksProcessing: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "dtq_tasks_processing",
			Help: "Number of tasks currently being processed",
		}),

		// Worker metrics
		WorkersActive: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "dtq_workers_active",
			Help: "Number of active workers",
		}),
		WorkerTasksTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "dtq_worker_tasks_total",
			Help: "Total number of tasks processed by worker",
		}, []string{"worker_id", "status"}),
		WorkerUptime: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "dtq_worker_uptime_seconds",
			Help: "Worker uptime in seconds",
		}, []string{"worker_id"}),

		// Queue metrics
		QueueDepth: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "dtq_queue_depth",
			Help: "Number of items in queue",
		}, []string{"queue"}),
		QueueThroughput: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "dtq_queue_throughput_total",
			Help: "Total queue throughput",
		}, []string{"queue", "operation"}),

		// System metrics
		HTTPRequests: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "dtq_http_requests_total",
			Help: "Total number of HTTP requests",
		}, []string{"method", "endpoint", "status"}),
		HTTPDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "dtq_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "endpoint"}),
		DatabaseConns: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "dtq_database_connections",
			Help: "Number of database connections",
		}, []string{"state"}),
		RedisConns: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "dtq_redis_connections",
			Help: "Number of Redis connections",
		}),

		logger: logger,
	}
}

func (m *Metrics) StartServer(addr string) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	m.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	m.logger.Info("Starting metrics server", zap.String("addr", addr))
	return m.server.ListenAndServe()
}

func (m *Metrics) Stop(ctx context.Context) error {
	if m.server != nil {
		m.logger.Info("Stopping metrics server")
		return m.server.Shutdown(ctx)
	}
	return nil
}

func (m *Metrics) TaskSubmitted() {
	m.TasksSubmitted.Inc()
}

func (m *Metrics) TaskCompleted(duration time.Duration) {
	m.TasksCompleted.Inc()
	m.TaskDuration.Observe(duration.Seconds())
}

func (m *Metrics) TaskFailed() {
	m.TasksFailed.Inc()
}

func (m *Metrics) TaskRetried() {
	m.TasksRetried.Inc()
}

func (m *Metrics) SetQueueDepth(priority string, count float64) {
	m.TasksInQueue.WithLabelValues(priority).Set(count)
}

func (m *Metrics) SetTasksProcessing(count float64) {
	m.TasksProcessing.Set(count)
}

func (m *Metrics) SetActiveWorkers(count float64) {
	m.WorkersActive.Set(count)
}

func (m *Metrics) WorkerTaskCompleted(workerID string) {
	m.WorkerTasksTotal.WithLabelValues(workerID, "completed").Inc()
}

func (m *Metrics) WorkerTaskFailed(workerID string) {
	m.WorkerTasksTotal.WithLabelValues(workerID, "failed").Inc()
}

func (m *Metrics) SetWorkerUptime(workerID string, uptime time.Duration) {
	m.WorkerUptime.WithLabelValues(workerID).Set(uptime.Seconds())
}

func (m *Metrics) SetQueueDepthByName(queueName string, count float64) {
	m.QueueDepth.WithLabelValues(queueName).Set(count)
}

func (m *Metrics) QueueOperation(queueName, operation string) {
	m.QueueThroughput.WithLabelValues(queueName, operation).Inc()
}

func (m *Metrics) HTTPRequest(method, endpoint, status string, duration time.Duration) {
	m.HTTPRequests.WithLabelValues(method, endpoint, status).Inc()
	m.HTTPDuration.WithLabelValues(method, endpoint).Observe(duration.Seconds())
}

func (m *Metrics) SetDatabaseConnections(state string, count float64) {
	m.DatabaseConns.WithLabelValues(state).Set(count)
}

func (m *Metrics) SetRedisConnections(count float64) {
	m.RedisConns.Set(count)
}

type HealthChecker struct {
	checks map[string]HealthCheck
	logger *zap.Logger
}

type HealthCheck interface {
	HealthCheck(ctx context.Context) error
}

type HealthStatus struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

func NewHealthChecker(logger *zap.Logger) *HealthChecker {
	return &HealthChecker{
		checks: make(map[string]HealthCheck),
		logger: logger,
	}
}

func (h *HealthChecker) AddCheck(name string, check HealthCheck) {
	h.checks[name] = check
}

func (h *HealthChecker) CheckHealth(ctx context.Context) HealthStatus {
	status := HealthStatus{
		Status: "healthy",
		Checks: make(map[string]string),
	}

	for name, check := range h.checks {
		if err := check.HealthCheck(ctx); err != nil {
			status.Checks[name] = "unhealthy: " + err.Error()
			status.Status = "unhealthy"
			h.logger.Warn("Health check failed",
				zap.String("check", name),
				zap.Error(err))
		} else {
			status.Checks[name] = "healthy"
		}
	}

	return status
} 