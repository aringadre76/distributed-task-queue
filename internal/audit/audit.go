package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"distributed-task-queue/internal/database"
	"distributed-task-queue/internal/tracing"
	"distributed-task-queue/pkg/types"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type AuditEvent string

const (
	EventTaskSubmitted    AuditEvent = "task.submitted"
	EventTaskCompleted    AuditEvent = "task.completed"
	EventTaskFailed       AuditEvent = "task.failed"
	EventTaskCancelled    AuditEvent = "task.cancelled"
	EventWorkerRegistered AuditEvent = "worker.registered"
	EventWorkerOffline    AuditEvent = "worker.offline"
	EventUserLogin        AuditEvent = "user.login"
	EventUserLogout       AuditEvent = "user.logout"
	EventTokenGenerated   AuditEvent = "token.generated"
	EventTokenRevoked     AuditEvent = "token.revoked"
	EventRateLimitHit     AuditEvent = "rate_limit.hit"
	EventUnauthorized     AuditEvent = "access.unauthorized"
	EventPermissionDenied AuditEvent = "access.permission_denied"
	EventSystemError      AuditEvent = "system.error"
	EventConfigChanged    AuditEvent = "config.changed"
	EventDataExported     AuditEvent = "data.exported"
	EventDataDeleted      AuditEvent = "data.deleted"
)

type AuditSeverity string

const (
	SeverityInfo     AuditSeverity = "info"
	SeverityWarning  AuditSeverity = "warning"
	SeverityError    AuditSeverity = "error"
	SeverityCritical AuditSeverity = "critical"
)

type AuditLog struct {
	ID            uuid.UUID              `json:"id"`
	Timestamp     time.Time              `json:"timestamp"`
	Event         AuditEvent             `json:"event"`
	Severity      AuditSeverity          `json:"severity"`
	UserID        string                 `json:"user_id,omitempty"`
	TenantID      string                 `json:"tenant_id,omitempty"`
	ResourceID    string                 `json:"resource_id,omitempty"`
	ResourceType  string                 `json:"resource_type,omitempty"`
	Action        string                 `json:"action"`
	Result        string                 `json:"result"`
	IPAddress     string                 `json:"ip_address,omitempty"`
	UserAgent     string                 `json:"user_agent,omitempty"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
	TraceID       string                 `json:"trace_id,omitempty"`
	Details       map[string]interface{} `json:"details,omitempty"`
	Error         string                 `json:"error,omitempty"`
}

type AuditLogger struct {
	db     *database.DB
	logger *zap.Logger
}

func NewAuditLogger(db *database.DB, logger *zap.Logger) *AuditLogger {
	return &AuditLogger{
		db:     db,
		logger: logger,
	}
}

func (al *AuditLogger) Log(ctx context.Context, event AuditEvent, severity AuditSeverity, action, result string, details map[string]interface{}) {
	auditLog := &AuditLog{
		ID:            uuid.New(),
		Timestamp:     time.Now(),
		Event:         event,
		Severity:      severity,
		Action:        action,
		Result:        result,
		CorrelationID: tracing.GetCorrelationID(ctx),
		TraceID:       tracing.GetTraceID(ctx),
		Details:       details,
	}

	if err := al.persistAuditLog(ctx, auditLog); err != nil {
		al.logger.Error("Failed to persist audit log", zap.Error(err))
	}

	al.logToZap(auditLog)
}

func (al *AuditLogger) LogWithUser(ctx context.Context, userID, tenantID string, event AuditEvent, severity AuditSeverity, action, result string, details map[string]interface{}) {
	auditLog := &AuditLog{
		ID:            uuid.New(),
		Timestamp:     time.Now(),
		Event:         event,
		Severity:      severity,
		UserID:        userID,
		TenantID:      tenantID,
		Action:        action,
		Result:        result,
		CorrelationID: tracing.GetCorrelationID(ctx),
		TraceID:       tracing.GetTraceID(ctx),
		Details:       details,
	}

	if err := al.persistAuditLog(ctx, auditLog); err != nil {
		al.logger.Error("Failed to persist audit log", zap.Error(err))
	}

	al.logToZap(auditLog)
}

func (al *AuditLogger) LogWithResource(ctx context.Context, userID, tenantID, resourceID, resourceType string, event AuditEvent, severity AuditSeverity, action, result string, details map[string]interface{}) {
	auditLog := &AuditLog{
		ID:            uuid.New(),
		Timestamp:     time.Now(),
		Event:         event,
		Severity:      severity,
		UserID:        userID,
		TenantID:      tenantID,
		ResourceID:    resourceID,
		ResourceType:  resourceType,
		Action:        action,
		Result:        result,
		CorrelationID: tracing.GetCorrelationID(ctx),
		TraceID:       tracing.GetTraceID(ctx),
		Details:       details,
	}

	if err := al.persistAuditLog(ctx, auditLog); err != nil {
		al.logger.Error("Failed to persist audit log", zap.Error(err))
	}

	al.logToZap(auditLog)
}

func (al *AuditLogger) LogError(ctx context.Context, userID, tenantID string, event AuditEvent, action string, err error, details map[string]interface{}) {
	auditLog := &AuditLog{
		ID:            uuid.New(),
		Timestamp:     time.Now(),
		Event:         event,
		Severity:      SeverityError,
		UserID:        userID,
		TenantID:      tenantID,
		Action:        action,
		Result:        "error",
		Error:         err.Error(),
		CorrelationID: tracing.GetCorrelationID(ctx),
		TraceID:       tracing.GetTraceID(ctx),
		Details:       details,
	}

	if err := al.persistAuditLog(ctx, auditLog); err != nil {
		al.logger.Error("Failed to persist audit log", zap.Error(err))
	}

	al.logToZap(auditLog)
}

func (al *AuditLogger) LogFromGin(c *gin.Context, event AuditEvent, severity AuditSeverity, action, result string, details map[string]interface{}) {
	auditLog := &AuditLog{
		ID:            uuid.New(),
		Timestamp:     time.Now(),
		Event:         event,
		Severity:      severity,
		Action:        action,
		Result:        result,
		IPAddress:     c.ClientIP(),
		UserAgent:     c.Request.UserAgent(),
		CorrelationID: tracing.GetCorrelationIDFromGin(c),
		TraceID:       tracing.GetTraceIDFromGin(c),
		Details:       details,
	}

	if userID, exists := c.Get("user_id"); exists {
		if uid, ok := userID.(string); ok {
			auditLog.UserID = uid
		}
	}

	if tenantID, exists := c.Get("tenant_id"); exists {
		if tid, ok := tenantID.(string); ok {
			auditLog.TenantID = tid
		}
	}

	if err := al.persistAuditLog(c.Request.Context(), auditLog); err != nil {
		al.logger.Error("Failed to persist audit log", zap.Error(err))
	}

	al.logToZap(auditLog)
}

func (al *AuditLogger) persistAuditLog(ctx context.Context, auditLog *AuditLog) error {
	detailsJSON, err := json.Marshal(auditLog.Details)
	if err != nil {
		detailsJSON = []byte("{}")
	}

	query := `
		INSERT INTO audit_logs (
			id, timestamp, event, severity, user_id, tenant_id, 
			resource_id, resource_type, action, result, ip_address, 
			user_agent, correlation_id, trace_id, details, error
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
		)`

	_, err = al.db.ExecContext(ctx, query,
		auditLog.ID,
		auditLog.Timestamp,
		auditLog.Event,
		auditLog.Severity,
		nullString(auditLog.UserID),
		nullString(auditLog.TenantID),
		nullString(auditLog.ResourceID),
		nullString(auditLog.ResourceType),
		auditLog.Action,
		auditLog.Result,
		nullString(auditLog.IPAddress),
		nullString(auditLog.UserAgent),
		nullString(auditLog.CorrelationID),
		nullString(auditLog.TraceID),
		detailsJSON,
		nullString(auditLog.Error),
	)

	return err
}

func (al *AuditLogger) logToZap(auditLog *AuditLog) {
	fields := []zap.Field{
		zap.String("audit_id", auditLog.ID.String()),
		zap.String("event", string(auditLog.Event)),
		zap.String("severity", string(auditLog.Severity)),
		zap.String("action", auditLog.Action),
		zap.String("result", auditLog.Result),
		zap.Time("timestamp", auditLog.Timestamp),
	}

	if auditLog.UserID != "" {
		fields = append(fields, zap.String("user_id", auditLog.UserID))
	}

	if auditLog.TenantID != "" {
		fields = append(fields, zap.String("tenant_id", auditLog.TenantID))
	}

	if auditLog.ResourceID != "" {
		fields = append(fields, zap.String("resource_id", auditLog.ResourceID))
	}

	if auditLog.ResourceType != "" {
		fields = append(fields, zap.String("resource_type", auditLog.ResourceType))
	}

	if auditLog.IPAddress != "" {
		fields = append(fields, zap.String("ip_address", auditLog.IPAddress))
	}

	if auditLog.CorrelationID != "" {
		fields = append(fields, zap.String("correlation_id", auditLog.CorrelationID))
	}

	if auditLog.TraceID != "" {
		fields = append(fields, zap.String("trace_id", auditLog.TraceID))
	}

	if auditLog.Error != "" {
		fields = append(fields, zap.String("error", auditLog.Error))
	}

	if len(auditLog.Details) > 0 {
		fields = append(fields, zap.Any("details", auditLog.Details))
	}

	switch auditLog.Severity {
	case SeverityInfo:
		al.logger.Info("Audit Log", fields...)
	case SeverityWarning:
		al.logger.Warn("Audit Log", fields...)
	case SeverityError:
		al.logger.Error("Audit Log", fields...)
	case SeverityCritical:
		al.logger.Error("Critical Audit Log", fields...)
	}
}

func (al *AuditLogger) AuditMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		c.Next()

		duration := time.Since(start)
		status := c.Writer.Status()

		event := EventSystemError
		severity := SeverityInfo
		result := "success"

		if status >= 400 {
			severity = SeverityWarning
			result = "error"
			if status >= 500 {
				severity = SeverityError
			}
		}

		if method == "POST" && status < 400 {
			event = EventTaskSubmitted
		}

		details := map[string]interface{}{
			"method":   method,
			"path":     path,
			"status":   status,
			"duration": duration.String(),
		}

		al.LogFromGin(c, event, severity, fmt.Sprintf("%s %s", method, path), result, details)
	}
}

func (al *AuditLogger) GetAuditLogs(ctx context.Context, userID, tenantID string, limit, offset int) ([]*AuditLog, error) {
	query := `
		SELECT id, timestamp, event, severity, user_id, tenant_id, 
			   resource_id, resource_type, action, result, ip_address, 
			   user_agent, correlation_id, trace_id, details, error
		FROM audit_logs
		WHERE ($1 = '' OR user_id = $1) AND ($2 = '' OR tenant_id = $2)
		ORDER BY timestamp DESC
		LIMIT $3 OFFSET $4`

	rows, err := al.db.QueryContext(ctx, query, userID, tenantID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*AuditLog
	for rows.Next() {
		log := &AuditLog{}
		var detailsJSON []byte
		var userIDPtr, tenantIDPtr, resourceIDPtr, resourceTypePtr *string
		var ipAddressPtr, userAgentPtr, correlationIDPtr, traceIDPtr, errorPtr *string

		err := rows.Scan(
			&log.ID,
			&log.Timestamp,
			&log.Event,
			&log.Severity,
			&userIDPtr,
			&tenantIDPtr,
			&resourceIDPtr,
			&resourceTypePtr,
			&log.Action,
			&log.Result,
			&ipAddressPtr,
			&userAgentPtr,
			&correlationIDPtr,
			&traceIDPtr,
			&detailsJSON,
			&errorPtr,
		)
		if err != nil {
			return nil, err
		}

		if userIDPtr != nil {
			log.UserID = *userIDPtr
		}
		if tenantIDPtr != nil {
			log.TenantID = *tenantIDPtr
		}
		if resourceIDPtr != nil {
			log.ResourceID = *resourceIDPtr
		}
		if resourceTypePtr != nil {
			log.ResourceType = *resourceTypePtr
		}
		if ipAddressPtr != nil {
			log.IPAddress = *ipAddressPtr
		}
		if userAgentPtr != nil {
			log.UserAgent = *userAgentPtr
		}
		if correlationIDPtr != nil {
			log.CorrelationID = *correlationIDPtr
		}
		if traceIDPtr != nil {
			log.TraceID = *traceIDPtr
		}
		if errorPtr != nil {
			log.Error = *errorPtr
		}

		if len(detailsJSON) > 0 {
			json.Unmarshal(detailsJSON, &log.Details)
		}

		logs = append(logs, log)
	}

	return logs, nil
}

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (al *AuditLogger) LogTaskOperation(ctx context.Context, userID, tenantID string, task *types.Task, operation string, result string, details map[string]interface{}) {
	event := EventTaskSubmitted
	switch operation {
	case "completed":
		event = EventTaskCompleted
	case "failed":
		event = EventTaskFailed
	case "cancelled":
		event = EventTaskCancelled
	}

	if details == nil {
		details = make(map[string]interface{})
	}
	details["task_type"] = task.Type
	details["task_priority"] = task.Priority

	al.LogWithResource(ctx, userID, tenantID, task.ID.String(), "task", event, SeverityInfo, operation, result, details)
}

func (al *AuditLogger) LogSecurityEvent(ctx context.Context, userID, tenantID, ipAddress string, event AuditEvent, action string, details map[string]interface{}) {
	auditLog := &AuditLog{
		ID:            uuid.New(),
		Timestamp:     time.Now(),
		Event:         event,
		Severity:      SeverityWarning,
		UserID:        userID,
		TenantID:      tenantID,
		Action:        action,
		Result:        "blocked",
		IPAddress:     ipAddress,
		CorrelationID: tracing.GetCorrelationID(ctx),
		TraceID:       tracing.GetTraceID(ctx),
		Details:       details,
	}

	if err := al.persistAuditLog(ctx, auditLog); err != nil {
		al.logger.Error("Failed to persist security audit log", zap.Error(err))
	}

	al.logToZap(auditLog)
} 