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
	"distributed-task-queue/internal/worker"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
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
	workerRepo := database.NewWorkerRepository(db, appLogger)

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

	workerID := uuid.New().String()
	w := worker.NewWorker(workerID, taskRepo, workerRepo, redisQueue, metrics, appLogger, cfg.Worker)

	if err := w.Register(ctx); err != nil {
		appLogger.Fatal("Failed to register worker", zap.Error(err))
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		w.Start(ctx)
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	appLogger.Info("Worker started", zap.String("worker_id", workerID))

	<-sigChan
	appLogger.Info("Shutting down worker...")

	cancel()
	
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		appLogger.Info("Worker shutdown complete")
	case <-time.After(30 * time.Second):
		appLogger.Warn("Worker shutdown timed out")
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