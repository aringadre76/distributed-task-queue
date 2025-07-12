package api

import (
	"time"

	"distributed-task-queue/internal/monitoring"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func NewRouter(handler *TaskHandler, metrics *monitoring.Metrics, logger *zap.Logger) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()

	router.Use(gin.Recovery())
	router.Use(GinLogger(logger))
	router.Use(MetricsMiddleware(metrics))

	corsConfig := cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Length", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}
	router.Use(cors.New(corsConfig))

	api := router.Group("/api/v1")
	{
		api.POST("/tasks", handler.SubmitTask)
		api.POST("/tasks/batch", handler.SubmitBatchTasks)
		api.POST("/tasks/dependencies", handler.CreateTaskWithDependencies)
		api.POST("/dag/execute", handler.ExecuteDAG)
		api.GET("/tasks/:id", handler.GetTask)
		api.GET("/tasks", handler.ListTasks)
		api.DELETE("/tasks/:id", handler.CancelTask)
		api.GET("/queue/stats", handler.GetQueueStats)
		api.GET("/workers", handler.GetWorkers)
		api.POST("/workers/register", handler.RegisterWorker)
		api.POST("/workers/heartbeat", handler.UpdateWorkerHeartbeat)
		api.GET("/circuit-breakers", handler.GetCircuitBreakerStats)
		api.GET("/pool", handler.GetWorkerPoolStats)
		api.GET("/batch/stats", handler.GetBatchProcessorStats)
		api.GET("/dependencies/stats", handler.GetDependencyStats)
		api.GET("/parallel/stats", handler.GetParallelExecutorStats)
	}

	router.GET("/health", handler.HealthCheck)
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "pong"})
	})

	return router
}

func GinLogger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		if raw != "" {
			path = path + "?" + raw
		}

		logger.Info("HTTP Request",
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
			zap.String("ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
		)
	}
} 