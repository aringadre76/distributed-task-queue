package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"distributed-task-queue/internal/config"
	"distributed-task-queue/internal/database"
	"distributed-task-queue/internal/logger"
	"distributed-task-queue/internal/monitoring"
	"distributed-task-queue/internal/queue"
	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

type dbHealthChecker struct {
	db *database.DB
}

func (d *dbHealthChecker) HealthCheck(ctx context.Context) error {
	return d.db.HealthCheck()
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	appLogger, err := logger.NewLogger(cfg.Logger)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer appLogger.Sync()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbConfig := database.Config{
		Host:            cfg.Database.Host,
		Port:            cfg.Database.Port,
		User:            cfg.Database.User,
		Password:        cfg.Database.Password,
		DBName:          cfg.Database.DBName,
		SSLMode:         cfg.Database.SSLMode,
		MaxOpenConns:    cfg.Database.MaxOpenConns,
		MaxIdleConns:    cfg.Database.MaxIdleConns,
		ConnMaxLifetime: cfg.Database.ConnMaxLifetime,
	}

	db, err := database.NewConnection(dbConfig, appLogger)
	if err != nil {
		appLogger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer db.Close()

	taskRepo := database.NewTaskRepository(db, appLogger)

	redisClient := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port),
		Password:     cfg.Redis.Password,
		DB:           cfg.Redis.DB,
		PoolSize:     cfg.Redis.PoolSize,
		DialTimeout:  cfg.Redis.DialTimeout,
		ReadTimeout:  cfg.Redis.ReadTimeout,
		WriteTimeout: cfg.Redis.WriteTimeout,
	})

	if _, err := redisClient.Ping(ctx).Result(); err != nil {
		appLogger.Fatal("Failed to connect to Redis", zap.Error(err))
	}

	redisQueue := queue.NewRedisQueue(redisClient, appLogger)

	metrics := monitoring.NewMetrics(appLogger)
	healthChecker := monitoring.NewHealthChecker(appLogger)
	healthChecker.AddCheck("redis", redisQueue)
	healthChecker.AddCheck("database", &dbHealthChecker{db: db})

	go func() {
		if err := metrics.StartServer(cfg.GetMetricsAddr()); err != nil {
			appLogger.Error("Failed to start metrics server", zap.Error(err))
		}
	}()

	queueManager := NewQueueManager(taskRepo, redisQueue, metrics, appLogger)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		queueManager.StartDelayedTaskProcessor(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		queueManager.StartStatsCollector(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		queueManager.StartHealthMonitor(ctx)
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	appLogger.Info("Queue Manager started")

	<-sigChan
	appLogger.Info("Shutting down Queue Manager...")

	cancel()
	
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		appLogger.Info("Queue Manager shutdown complete")
	case <-time.After(30 * time.Second):
		appLogger.Warn("Queue Manager shutdown timed out")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := metrics.Stop(shutdownCtx); err != nil {
		appLogger.Error("Failed to shutdown metrics server", zap.Error(err))
	}

	if err := redisClient.Close(); err != nil {
		appLogger.Error("Failed to close Redis client", zap.Error(err))
	}
}

type QueueManager struct {
	taskRepo database.TaskRepository
	queue    *queue.RedisQueue
	metrics  *monitoring.Metrics
	logger   *zap.Logger
}

func NewQueueManager(
	taskRepo database.TaskRepository,
	queue *queue.RedisQueue,
	metrics *monitoring.Metrics,
	logger *zap.Logger,
) *QueueManager {
	return &QueueManager{
		taskRepo: taskRepo,
		queue:    queue,
		metrics:  metrics,
		logger:   logger,
	}
}

func (qm *QueueManager) StartDelayedTaskProcessor(ctx context.Context) {
	qm.logger.Info("Starting delayed task processor")
	
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			qm.logger.Info("Delayed task processor stopping")
			return
		case <-ticker.C:
			qm.processDelayedTasks(ctx)
		}
	}
}

func (qm *QueueManager) processDelayedTasks(ctx context.Context) {
	if err := qm.queue.ProcessDelayedTasks(ctx); err != nil {
		qm.logger.Error("Failed to process delayed tasks", zap.Error(err))
		return
	}

	qm.logger.Debug("Processed delayed tasks")
}

func (qm *QueueManager) StartStatsCollector(ctx context.Context) {
	qm.logger.Info("Starting queue statistics collector")
	
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			qm.logger.Info("Stats collector stopping")
			return
		case <-ticker.C:
			qm.collectQueueStats(ctx)
		}
	}
}

func (qm *QueueManager) collectQueueStats(ctx context.Context) {
	stats, err := qm.queue.GetQueueStats(ctx)
	if err != nil {
		qm.logger.Error("Failed to get queue stats", zap.Error(err))
		return
	}

	qm.metrics.SetQueueDepthByName("high_priority", float64(stats["high_priority"]))
	qm.metrics.SetQueueDepthByName("medium_priority", float64(stats["medium_priority"]))
	qm.metrics.SetQueueDepthByName("low_priority", float64(stats["low_priority"]))
	qm.metrics.SetQueueDepthByName("processing", float64(stats["processing"]))
	qm.metrics.SetQueueDepthByName("dead_letter", float64(stats["dead_letter"]))
	qm.metrics.SetQueueDepthByName("delayed", float64(stats["delayed"]))

	qm.logger.Debug("Queue stats collected", zap.Any("stats", stats))
}

func (qm *QueueManager) StartHealthMonitor(ctx context.Context) {
	qm.logger.Info("Starting health monitor")
	
	ticker := time.NewTicker(120 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			qm.logger.Info("Health monitor stopping")
			return
		case <-ticker.C:
			qm.checkSystemHealth(ctx)
		}
	}
}

func (qm *QueueManager) checkSystemHealth(ctx context.Context) {
	// Check database health
	if err := qm.taskRepo.HealthCheck(ctx); err != nil {
		qm.logger.Error("Database health check failed", zap.Error(err))
	}

	// Check Redis health
	if err := qm.queue.HealthCheck(ctx); err != nil {
		qm.logger.Error("Redis health check failed", zap.Error(err))
	}

	qm.logger.Debug("System health check completed")
} 