package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	AuthorizationHeader = "Authorization"
	BearerPrefix        = "Bearer "
	ClaimsKey           = "claims"
	UserIDKey           = "user_id"
	TenantIDKey         = "tenant_id"
	RolesKey            = "roles"
)

type AuthMiddleware struct {
	jwtManager *JWTManager
	logger     *zap.Logger
}

func NewAuthMiddleware(jwtManager *JWTManager, logger *zap.Logger) *AuthMiddleware {
	return &AuthMiddleware{
		jwtManager: jwtManager,
		logger:     logger,
	}
}

func (am *AuthMiddleware) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := am.extractToken(c)
		if token == "" {
			am.logger.Warn("Authentication required but no token provided",
				zap.String("path", c.Request.URL.Path),
				zap.String("method", c.Request.Method))
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			c.Abort()
			return
		}

		claims, err := am.jwtManager.ValidateToken(token)
		if err != nil {
			am.logger.Warn("Invalid token provided",
				zap.Error(err),
				zap.String("path", c.Request.URL.Path))
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		am.setClaims(c, claims)
		c.Next()
	}
}

func (am *AuthMiddleware) RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := am.getClaims(c)
		if claims == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			c.Abort()
			return
		}

		if !claims.HasAnyRole(roles) {
			am.logger.Warn("Insufficient permissions",
				zap.String("user_id", claims.UserID),
				zap.Strings("user_roles", claims.Roles),
				zap.Strings("required_roles", roles))
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			c.Abort()
			return
		}

		c.Next()
	}
}

func (am *AuthMiddleware) RequireAdmin() gin.HandlerFunc {
	return am.RequireRole("admin")
}

func (am *AuthMiddleware) RequireUser() gin.HandlerFunc {
	return am.RequireRole("user", "admin")
}

func (am *AuthMiddleware) RequireWorker() gin.HandlerFunc {
	return am.RequireRole("worker", "admin")
}

func (am *AuthMiddleware) OptionalAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := am.extractToken(c)
		if token == "" {
			c.Next()
			return
		}

		claims, err := am.jwtManager.ValidateToken(token)
		if err != nil {
			am.logger.Debug("Optional auth failed", zap.Error(err))
			c.Next()
			return
		}

		am.setClaims(c, claims)
		c.Next()
	}
}

func (am *AuthMiddleware) TenantIsolation() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := am.getClaims(c)
		if claims == nil {
			c.Next()
			return
		}

		if claims.TenantID != "" {
			c.Set(TenantIDKey, claims.TenantID)
		}

		c.Next()
	}
}

func (am *AuthMiddleware) extractToken(c *gin.Context) string {
	authHeader := c.GetHeader(AuthorizationHeader)
	if authHeader == "" {
		return ""
	}

	if !strings.HasPrefix(authHeader, BearerPrefix) {
		return ""
	}

	return strings.TrimPrefix(authHeader, BearerPrefix)
}

func (am *AuthMiddleware) setClaims(c *gin.Context, claims *Claims) {
	c.Set(ClaimsKey, claims)
	c.Set(UserIDKey, claims.UserID)
	c.Set(TenantIDKey, claims.TenantID)
	c.Set(RolesKey, claims.Roles)
}

func (am *AuthMiddleware) getClaims(c *gin.Context) *Claims {
	claims, exists := c.Get(ClaimsKey)
	if !exists {
		return nil
	}

	claimsTyped, ok := claims.(*Claims)
	if !ok {
		return nil
	}

	return claimsTyped
}

func GetClaims(c *gin.Context) *Claims {
	claims, exists := c.Get(ClaimsKey)
	if !exists {
		return nil
	}

	claimsTyped, ok := claims.(*Claims)
	if !ok {
		return nil
	}

	return claimsTyped
}

func GetUserID(c *gin.Context) string {
	userID, exists := c.Get(UserIDKey)
	if !exists {
		return ""
	}

	userIDStr, ok := userID.(string)
	if !ok {
		return ""
	}

	return userIDStr
}

func GetTenantID(c *gin.Context) string {
	tenantID, exists := c.Get(TenantIDKey)
	if !exists {
		return ""
	}

	tenantIDStr, ok := tenantID.(string)
	if !ok {
		return ""
	}

	return tenantIDStr
}

func GetRoles(c *gin.Context) []string {
	roles, exists := c.Get(RolesKey)
	if !exists {
		return []string{}
	}

	rolesSlice, ok := roles.([]string)
	if !ok {
		return []string{}
	}

	return rolesSlice
} 