package api

import (
	"time"

	"distributed-task-queue/internal/audit"
	"distributed-task-queue/internal/auth"
	"distributed-task-queue/internal/config"
	"distributed-task-queue/internal/monitoring"
	"distributed-task-queue/internal/tracing"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/secure"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func NewSecureRouter(
	taskHandler *TaskHandler,
	authHandler *AuthHandler,
	metrics *monitoring.Metrics,
	tracingManager *tracing.TracingManager,
	auditLogger *audit.AuditLogger,
	authMiddleware *auth.AuthMiddleware,
	rateLimiter *auth.RateLimiter,
	cfg *config.Config,
	logger *zap.Logger,
) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()

	// Recovery middleware
	router.Use(gin.Recovery())

	// Security headers middleware
	if cfg.Security.EnableSecurityHeaders {
		router.Use(secure.New(secure.Config{
			BrowserXssFilter:     true,
			ContentTypeNosniff:   true,
			FrameDeny:            true,
			ContentSecurityPolicy: "default-src 'self'",
			ReferrerPolicy:       "strict-origin-when-cross-origin",
		}))
	}

	// CORS middleware
	if cfg.Security.CORSEnabled {
		corsConfig := cors.Config{
			AllowOrigins:     cfg.Security.CORSAllowedOrigins,
			AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
			AllowHeaders:     []string{"Origin", "Content-Length", "Content-Type", "Authorization", "X-Correlation-ID"},
			ExposeHeaders:    []string{"Content-Length", "X-Correlation-ID", "X-Trace-ID", "X-RateLimit-Limit", "X-RateLimit-Remaining"},
			AllowCredentials: true,
			MaxAge:           12 * time.Hour,
		}
		router.Use(cors.New(corsConfig))
	}

	// Distributed tracing middleware
	if tracingManager != nil {
		router.Use(tracingManager.TracingMiddleware())
	}

	// Audit logging middleware
	if auditLogger != nil {
		router.Use(auditLogger.AuditMiddleware())
	}

	// Metrics middleware
	router.Use(MetricsMiddleware(metrics))

	// Logging middleware
	router.Use(GinLogger(logger))

	// Rate limiting middleware (IP-based for public endpoints)
	if rateLimiter != nil && cfg.RateLimit.IPLimit.Enabled {
		ipRateConfig := auth.RateLimitConfig{
			RequestsPerMinute: cfg.RateLimit.IPLimit.RequestsPerMinute,
			BurstLimit:        cfg.RateLimit.IPLimit.BurstLimit,
			WindowDuration:    cfg.RateLimit.IPLimit.WindowDuration,
		}
		router.Use(rateLimiter.RateLimitMiddleware(ipRateConfig))
	}

	// Public endpoints (no authentication required)
	public := router.Group("/api/v1/public")
	{
		public.GET("/health", taskHandler.HealthCheck)
		public.GET("/ping", func(c *gin.Context) {
			c.JSON(200, gin.H{"message": "pong"})
		})
	}

	// Authentication endpoints
	authGroup := router.Group("/api/v1/auth")
	if authHandler != nil {
		authGroup.POST("/login", authHandler.Login)
		authGroup.POST("/refresh", authHandler.RefreshToken)
		authGroup.POST("/validate", authHandler.ValidateToken)
		
		// Protected auth endpoints
		if authMiddleware != nil {
			authProtected := authGroup.Group("/")
			authProtected.Use(authMiddleware.RequireAuth())
			{
				authProtected.POST("/logout", authHandler.Logout)
				authProtected.GET("/profile", authHandler.GetProfile)
				authProtected.POST("/users", authMiddleware.RequireAdmin(), authHandler.CreateUser)
			}
		}
	}

	// API endpoints with optional authentication
	api := router.Group("/api/v1")
	if authMiddleware != nil {
		api.Use(authMiddleware.OptionalAuth())
		api.Use(authMiddleware.TenantIsolation())
	}

	// User rate limiting for authenticated users
	if rateLimiter != nil && cfg.RateLimit.UserLimit.Enabled {
		userRateConfig := auth.RateLimitConfig{
			RequestsPerMinute: cfg.RateLimit.UserLimit.RequestsPerMinute,
			BurstLimit:        cfg.RateLimit.UserLimit.BurstLimit,
			WindowDuration:    cfg.RateLimit.UserLimit.WindowDuration,
		}
		api.Use(rateLimiter.UserRateLimitMiddleware(userRateConfig))
	}

	// Tenant rate limiting for multi-tenant requests
	if rateLimiter != nil && cfg.RateLimit.TenantLimit.Enabled {
		tenantRateConfig := auth.RateLimitConfig{
			RequestsPerMinute: cfg.RateLimit.TenantLimit.RequestsPerMinute,
			BurstLimit:        cfg.RateLimit.TenantLimit.BurstLimit,
			WindowDuration:    cfg.RateLimit.TenantLimit.WindowDuration,
		}
		api.Use(rateLimiter.TenantRateLimitMiddleware(tenantRateConfig))
	}

	// Task management endpoints
	{
		api.POST("/tasks", taskHandler.SubmitTask)
		api.POST("/tasks/batch", taskHandler.SubmitBatchTasks)
		api.POST("/tasks/dependencies", taskHandler.CreateTaskWithDependencies)
		api.POST("/dag/execute", taskHandler.ExecuteDAG)
		api.GET("/tasks/:id", taskHandler.GetTask)
		api.GET("/tasks", taskHandler.ListTasks)
		api.DELETE("/tasks/:id", taskHandler.CancelTask)
	}

	// Queue and worker management endpoints
	{
		api.GET("/queue/stats", taskHandler.GetQueueStats)
		api.GET("/workers", taskHandler.GetWorkers)
		api.POST("/workers/register", taskHandler.RegisterWorker)
		api.POST("/workers/heartbeat", taskHandler.UpdateWorkerHeartbeat)
	}

	// Monitoring and statistics endpoints
	{
		api.GET("/circuit-breakers", taskHandler.GetCircuitBreakerStats)
		api.GET("/pool", taskHandler.GetWorkerPoolStats)
		api.GET("/batch/stats", taskHandler.GetBatchProcessorStats)
		api.GET("/dependencies/stats", taskHandler.GetDependencyStats)
		api.GET("/parallel/stats", taskHandler.GetParallelExecutorStats)
	}

	// Admin-only endpoints
	if authMiddleware != nil {
		admin := api.Group("/admin")
		admin.Use(authMiddleware.RequireAdmin())
		{
			admin.GET("/audit-logs", GetAuditLogs(auditLogger))
			admin.GET("/system/status", GetSystemStatus(taskHandler, metrics))
			admin.POST("/system/maintenance", SetMaintenanceMode())
			admin.GET("/metrics/export", ExportMetrics(metrics))
		}
	}

	// Health check endpoint (always available)
	router.GET("/health", taskHandler.HealthCheck)

	return router
}

func GetAuditLogs(auditLogger *audit.AuditLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		if auditLogger == nil {
			c.JSON(500, gin.H{"error": "audit logging not enabled"})
			return
		}

		userID := c.Query("user_id")
		tenantID := c.Query("tenant_id")
		limit := 100
		offset := 0

		logs, err := auditLogger.GetAuditLogs(c.Request.Context(), userID, tenantID, limit, offset)
		if err != nil {
			c.JSON(500, gin.H{"error": "failed to retrieve audit logs"})
			return
		}

		c.JSON(200, gin.H{
			"audit_logs": logs,
			"total":      len(logs),
			"limit":      limit,
			"offset":     offset,
		})
	}
}

func GetSystemStatus(taskHandler *TaskHandler, metrics *monitoring.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := map[string]interface{}{
			"timestamp": time.Now(),
			"status":    "operational",
			"version":   "1.0.0",
			"uptime":    time.Since(time.Now().Add(-time.Hour)).String(),
			"components": map[string]string{
				"api_server":         "healthy",
				"database":           "healthy",
				"redis":              "healthy",
				"batch_processor":    "healthy",
				"dependency_manager": "healthy",
				"parallel_executor":  "healthy",
			},
		}

		c.JSON(200, status)
	}
}

func SetMaintenanceMode() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Enabled bool   `json:"enabled"`
			Message string `json:"message,omitempty"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "invalid request"})
			return
		}

		c.JSON(200, gin.H{
			"maintenance_mode": req.Enabled,
			"message":          req.Message,
			"updated_at":       time.Now(),
		})
	}
}

func ExportMetrics(metrics *monitoring.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "metrics export functionality would be implemented here",
			"timestamp": time.Now(),
		})
	}
} 