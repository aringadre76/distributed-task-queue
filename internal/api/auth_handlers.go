package api

import (
	"net/http"
	"time"

	"distributed-task-queue/internal/audit"
	"distributed-task-queue/internal/auth"
	"distributed-task-queue/internal/tracing"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type AuthHandler struct {
	jwtManager  *auth.JWTManager
	auditLogger *audit.AuditLogger
	logger      *zap.Logger
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	TenantID string `json:"tenant_id,omitempty"`
}

type LoginResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int       `json:"expires_in"`
	UserID       string    `json:"user_id"`
	TenantID     string    `json:"tenant_id"`
	Roles        []string  `json:"roles"`
	IssuedAt     time.Time `json:"issued_at"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type TokenValidationResponse struct {
	Valid     bool      `json:"valid"`
	UserID    string    `json:"user_id,omitempty"`
	TenantID  string    `json:"tenant_id,omitempty"`
	Roles     []string  `json:"roles,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

type CreateUserRequest struct {
	Username string   `json:"username" binding:"required"`
	Password string   `json:"password" binding:"required"`
	TenantID string   `json:"tenant_id,omitempty"`
	Roles    []string `json:"roles,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

func NewAuthHandler(jwtManager *auth.JWTManager, auditLogger *audit.AuditLogger, logger *zap.Logger) *AuthHandler {
	return &AuthHandler{
		jwtManager:  jwtManager,
		auditLogger: auditLogger,
		logger:      logger,
	}
}

func (ah *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ah.auditLogger.LogFromGin(c, audit.EventUnauthorized, audit.SeverityWarning, "login", "invalid_request", map[string]interface{}{
			"error": err.Error(),
		})
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	correlationID := tracing.GetCorrelationIDFromGin(c)
	
	userID, tenantID, roles, err := ah.authenticateUser(req.Username, req.Password, req.TenantID)
	if err != nil {
		ah.auditLogger.LogFromGin(c, audit.EventUnauthorized, audit.SeverityWarning, "login", "failed", map[string]interface{}{
			"username": req.Username,
			"tenant_id": req.TenantID,
			"error": err.Error(),
		})
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	accessToken, err := ah.jwtManager.GenerateToken(userID, tenantID, roles)
	if err != nil {
		ah.logger.Error("Failed to generate access token", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	var refreshToken string
	if ah.jwtManager != nil {
		refreshToken, _ = ah.jwtManager.GenerateToken(userID, tenantID, roles)
	}

	response := LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(24 * time.Hour.Seconds()),
		UserID:       userID,
		TenantID:     tenantID,
		Roles:        roles,
		IssuedAt:     time.Now(),
	}

	ah.auditLogger.LogFromGin(c, audit.EventUserLogin, audit.SeverityInfo, "login", "success", map[string]interface{}{
		"user_id": userID,
		"tenant_id": tenantID,
		"roles": roles,
		"correlation_id": correlationID,
	})

	ah.logger.Info("User logged in successfully",
		zap.String("user_id", userID),
		zap.String("tenant_id", tenantID),
		zap.Strings("roles", roles),
		zap.String("correlation_id", correlationID))

	c.JSON(http.StatusOK, response)
}

func (ah *AuthHandler) RefreshToken(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	claims, err := ah.jwtManager.ValidateToken(req.RefreshToken)
	if err != nil {
		ah.auditLogger.LogFromGin(c, audit.EventUnauthorized, audit.SeverityWarning, "refresh_token", "invalid_token", map[string]interface{}{
			"error": err.Error(),
		})
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid refresh token"})
		return
	}

	newAccessToken, err := ah.jwtManager.GenerateToken(claims.UserID, claims.TenantID, claims.Roles)
	if err != nil {
		ah.logger.Error("Failed to generate new access token", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	response := LoginResponse{
		AccessToken: newAccessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int(24 * time.Hour.Seconds()),
		UserID:      claims.UserID,
		TenantID:    claims.TenantID,
		Roles:       claims.Roles,
		IssuedAt:    time.Now(),
	}

	ah.auditLogger.LogWithUser(c.Request.Context(), claims.UserID, claims.TenantID, audit.EventTokenGenerated, audit.SeverityInfo, "refresh_token", "success", map[string]interface{}{
		"correlation_id": tracing.GetCorrelationIDFromGin(c),
	})

	c.JSON(http.StatusOK, response)
}

func (ah *AuthHandler) ValidateToken(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Authorization header required"})
		return
	}

	tokenString := authHeader
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		tokenString = authHeader[7:]
	}

	claims, err := ah.jwtManager.ValidateToken(tokenString)
	if err != nil {
		response := TokenValidationResponse{Valid: false}
		c.JSON(http.StatusOK, response)
		return
	}

	response := TokenValidationResponse{
		Valid:     true,
		UserID:    claims.UserID,
		TenantID:  claims.TenantID,
		Roles:     claims.Roles,
		ExpiresAt: claims.ExpiresAt.Time,
	}

	c.JSON(http.StatusOK, response)
}

func (ah *AuthHandler) Logout(c *gin.Context) {
	claims := auth.GetClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	ah.auditLogger.LogWithUser(c.Request.Context(), claims.UserID, claims.TenantID, audit.EventUserLogout, audit.SeverityInfo, "logout", "success", map[string]interface{}{
		"correlation_id": tracing.GetCorrelationIDFromGin(c),
	})

	ah.logger.Info("User logged out",
		zap.String("user_id", claims.UserID),
		zap.String("tenant_id", claims.TenantID))

	c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}

func (ah *AuthHandler) CreateUser(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	claims := auth.GetClaims(c)
	if claims == nil || !claims.IsAdmin() {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin privileges required"})
		return
	}

	userID, err := ah.createUser(req.Username, req.Password, req.TenantID, req.Roles)
	if err != nil {
		ah.logger.Error("Failed to create user", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	ah.auditLogger.LogWithUser(c.Request.Context(), claims.UserID, claims.TenantID, audit.EventUserLogin, audit.SeverityInfo, "create_user", "success", map[string]interface{}{
		"created_user_id": userID,
		"created_username": req.Username,
		"created_tenant_id": req.TenantID,
		"created_roles": req.Roles,
	})

	c.JSON(http.StatusCreated, gin.H{
		"user_id": userID,
		"username": req.Username,
		"tenant_id": req.TenantID,
		"roles": req.Roles,
		"message": "User created successfully",
	})
}

func (ah *AuthHandler) GetProfile(c *gin.Context) {
	claims := auth.GetClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	profile := map[string]interface{}{
		"user_id":    claims.UserID,
		"tenant_id":  claims.TenantID,
		"roles":      claims.Roles,
		"expires_at": claims.ExpiresAt.Time,
		"issued_at":  claims.IssuedAt.Time,
	}

	c.JSON(http.StatusOK, profile)
}

func (ah *AuthHandler) authenticateUser(username, password, tenantID string) (string, string, []string, error) {
	switch username {
	case "admin":
		if password == "admin123" {
			return "admin-user-id", tenantID, []string{"admin", "user"}, nil
		}
	case "user":
		if password == "user123" {
			return "regular-user-id", tenantID, []string{"user"}, nil
		}
	case "worker":
		if password == "worker123" {
			return "worker-user-id", tenantID, []string{"worker"}, nil
		}
	}

	return "", "", nil, auth.ErrTokenInvalid
}

func (ah *AuthHandler) createUser(username, password, tenantID string, roles []string) (string, error) {
	if roles == nil {
		roles = []string{"user"}
	}

	userID := "user-" + username + "-" + time.Now().Format("20060102150405")
	
	ah.logger.Info("User created",
		zap.String("user_id", userID),
		zap.String("username", username),
		zap.String("tenant_id", tenantID),
		zap.Strings("roles", roles))

	return userID, nil
} 