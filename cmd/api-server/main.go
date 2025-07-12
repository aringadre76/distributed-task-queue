package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"distributed-task-queue/internal/api"
	"distributed-task-queue/internal/audit"
	"distributed-task-queue/internal/auth"
	"distributed-task-queue/internal/config"
	"distributed-task-queue/internal/database"
	"distributed-task-queue/internal/logger"
	"distributed-task-queue/internal/monitoring"
	"distributed-task-queue/internal/queue"
	"distributed-task-queue/internal/tracing"
	"distributed-task-queue/internal/worker"
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
		log.Fatal("Failed to load configuration", err)
	}

	zapLogger, err := logger.NewLogger(cfg.Logger)
	if err != nil {
		log.Fatal("Failed to create logger", err)
	}
	defer zapLogger.Sync()

	zapLogger.Info("Starting Distributed Task Queue API Server...")

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

	db, err := database.NewConnection(dbConfig, zapLogger)
	if err != nil {
		zapLogger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer db.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port),
		Password:     cfg.Redis.Password,
		DB:           cfg.Redis.DB,
		PoolSize:     cfg.Redis.PoolSize,
		DialTimeout:  cfg.Redis.DialTimeout,
		ReadTimeout:  cfg.Redis.ReadTimeout,
		WriteTimeout: cfg.Redis.WriteTimeout,
	})
	defer redisClient.Close()

	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		zapLogger.Fatal("Failed to connect to Redis", zap.Error(err))
	}

	redisQueue := queue.NewRedisQueue(redisClient, zapLogger)

	// Initialize repositories
	taskRepo := database.NewTaskRepository(db, zapLogger)
	workerRepo := database.NewWorkerRepository(db, zapLogger)

	// Initialize Phase 4 components
	var tracingManager *tracing.TracingManager
	if cfg.Tracing.Enabled {
		tracingManager, err = tracing.NewTracingManager(cfg.Tracing.ServiceName, cfg.Tracing.JaegerEndpoint, zapLogger)
		if err != nil {
			zapLogger.Error("Failed to initialize tracing", zap.Error(err))
		} else {
			zapLogger.Info("Distributed tracing initialized",
				zap.String("service", cfg.Tracing.ServiceName),
				zap.String("jaeger_endpoint", cfg.Tracing.JaegerEndpoint))
		}
	}

	var auditLogger *audit.AuditLogger
	if cfg.Audit.Enabled {
		auditLogger = audit.NewAuditLogger(db, zapLogger)
		zapLogger.Info("Audit logging initialized")
	}

	var jwtManager *auth.JWTManager
	var authMiddleware *auth.AuthMiddleware
	if cfg.Auth.Enabled {
		jwtManager = auth.NewJWTManager(cfg.Auth.JWTSecret, cfg.Auth.TokenDuration, zapLogger)
		authMiddleware = auth.NewAuthMiddleware(jwtManager, zapLogger)
		zapLogger.Info("JWT authentication initialized")
	}

	var rateLimiter *auth.RateLimiter
	if cfg.RateLimit.Enabled {
		rateLimiter = auth.NewRateLimiter(redisClient, zapLogger)
		zapLogger.Info("Rate limiting initialized")
	}

	// Initialize monitoring and health checks
	metrics := monitoring.NewMetrics(zapLogger)
	healthChecker := monitoring.NewHealthChecker(zapLogger)

	healthChecker.AddCheck("database", &dbHealthChecker{db: db})
	healthChecker.AddCheck("redis", redisQueue)

	// Initialize handlers
	taskHandler := api.NewTaskHandler(taskRepo, workerRepo, redisQueue, metrics, healthChecker, zapLogger)
	authHandler := api.NewAuthHandler(jwtManager, auditLogger, zapLogger)

	// Initialize Phase 3 components
	batchConfig := queue.BatchConfig{
		BatchSize: cfg.Worker.BatchProcessing.BatchSize,
		FlushTime: cfg.Worker.BatchProcessing.FlushTime,
	}
	batchProcessor := queue.NewBatchProcessor(redisQueue, zapLogger, batchConfig)
	if err := batchProcessor.Start(context.Background()); err != nil {
		zapLogger.Fatal("Failed to start batch processor", zap.Error(err))
	}

	dependencyManager := queue.NewDependencyManager(redisQueue, zapLogger)

	parallelExecutor := worker.NewParallelExecutor(cfg.Worker.ParallelExecution.MaxConcurrency, zapLogger)
	if err := parallelExecutor.Start(context.Background()); err != nil {
		zapLogger.Fatal("Failed to start parallel executor", zap.Error(err))
	}

	// Set Phase 3 components on task handler
	taskHandler.SetBatchProcessor(batchProcessor)
	taskHandler.SetDependencyManager(dependencyManager)
	taskHandler.SetParallelExecutor(parallelExecutor)

	// Create router with Phase 4 middleware
	router := api.NewSecureRouter(
		taskHandler,
		authHandler,
		metrics,
		tracingManager,
		auditLogger,
		authMiddleware,
		rateLimiter,
		cfg,
		zapLogger,
	)

	server := &http.Server{
		Addr:         cfg.GetServerAddr(),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Start metrics server
	go func() {
		if cfg.Metrics.Enabled {
			zapLogger.Info("Starting metrics server", zap.String("addr", cfg.GetMetricsAddr()))
			if err := metrics.StartServer(cfg.GetMetricsAddr()); err != nil && err != http.ErrServerClosed {
				zapLogger.Error("Metrics server failed", zap.Error(err))
			}
		}
	}()

	// Start delayed task processor
	go startDelayedTaskProcessor(redisQueue, zapLogger)

	// Start audit log cleanup if enabled
	if cfg.Audit.Enabled && cfg.Audit.RetentionDays > 0 {
		go startAuditLogCleanup(db, cfg.Audit.RetentionDays, zapLogger)
	}

	// Start server
	go func() {
		zapLogger.Info("Starting API server", zap.String("addr", cfg.GetServerAddr()))
		if cfg.Security.TLSEnabled {
			if err := server.ListenAndServeTLS(cfg.Security.TLSCertFile, cfg.Security.TLSKeyFile); err != nil && err != http.ErrServerClosed {
				zapLogger.Fatal("Failed to start HTTPS server", zap.Error(err))
			}
		} else {
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				zapLogger.Fatal("Failed to start HTTP server", zap.Error(err))
			}
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	zapLogger.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown Phase 3 components
	if batchProcessor != nil {
		batchProcessor.Stop()
	}
	if parallelExecutor != nil {
		parallelExecutor.Stop()
	}

	// Shutdown server
	if err := server.Shutdown(ctx); err != nil {
		zapLogger.Fatal("Server forced to shutdown", zap.Error(err))
	}

	zapLogger.Info("Server exited")
}

func startDelayedTaskProcessor(queue *queue.RedisQueue, logger *zap.Logger) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := queue.ProcessDelayedTasks(context.Background()); err != nil {
				logger.Error("Failed to process delayed tasks", zap.Error(err))
			}
		}
	}
}

func startAuditLogCleanup(db *database.DB, retentionDays int, logger *zap.Logger) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	logger.Info("Starting audit log cleanup task", zap.Int("retention_days", retentionDays))

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			
			query := "SELECT cleanup_old_audit_logs($1)"
			var deletedCount int
			err := db.QueryRowContext(ctx, query, retentionDays).Scan(&deletedCount)
			
			if err != nil {
				logger.Error("Failed to cleanup old audit logs", zap.Error(err))
			} else if deletedCount > 0 {
				logger.Info("Cleaned up old audit logs", zap.Int("deleted_count", deletedCount))
			}
			
			cancel()
		}
	}
} 