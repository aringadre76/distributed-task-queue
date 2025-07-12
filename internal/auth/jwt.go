package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

var (
	ErrTokenExpired     = errors.New("token has expired")
	ErrTokenInvalid     = errors.New("token is invalid")
	ErrTokenMalformed   = errors.New("token is malformed")
	ErrTokenNotFound    = errors.New("token not found")
	ErrInsufficientRole = errors.New("insufficient role permissions")
)

type Claims struct {
	UserID   string   `json:"user_id"`
	TenantID string   `json:"tenant_id"`
	Roles    []string `json:"roles"`
	jwt.RegisteredClaims
}

type JWTManager struct {
	secretKey     string
	tokenDuration time.Duration
	logger        *zap.Logger
}

func NewJWTManager(secretKey string, tokenDuration time.Duration, logger *zap.Logger) *JWTManager {
	return &JWTManager{
		secretKey:     secretKey,
		tokenDuration: tokenDuration,
		logger:        logger,
	}
}

func (j *JWTManager) GenerateToken(userID, tenantID string, roles []string) (string, error) {
	claims := &Claims{
		UserID:   userID,
		TenantID: tenantID,
		Roles:    roles,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(j.tokenDuration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "distributed-task-queue",
			Subject:   userID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(j.secretKey))
	if err != nil {
		j.logger.Error("Failed to generate JWT token", zap.Error(err))
		return "", err
	}

	j.logger.Debug("JWT token generated",
		zap.String("user_id", userID),
		zap.String("tenant_id", tenantID),
		zap.Strings("roles", roles))

	return tokenString, nil
}

func (j *JWTManager) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(j.secretKey), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		if errors.Is(err, jwt.ErrTokenMalformed) {
			return nil, ErrTokenMalformed
		}
		return nil, ErrTokenInvalid
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrTokenInvalid
	}

	return claims, nil
}

func (j *JWTManager) RefreshToken(tokenString string) (string, error) {
	claims, err := j.ValidateToken(tokenString)
	if err != nil {
		return "", err
	}

	if time.Until(claims.ExpiresAt.Time) > 30*time.Minute {
		return tokenString, nil
	}

	return j.GenerateToken(claims.UserID, claims.TenantID, claims.Roles)
}

func (c *Claims) HasRole(role string) bool {
	for _, r := range c.Roles {
		if r == role {
			return true
		}
	}
	return false
}

func (c *Claims) HasAnyRole(roles []string) bool {
	for _, role := range roles {
		if c.HasRole(role) {
			return true
		}
	}
	return false
}

func (c *Claims) IsAdmin() bool {
	return c.HasRole("admin")
}

func (c *Claims) IsUser() bool {
	return c.HasRole("user")
}

func (c *Claims) IsWorker() bool {
	return c.HasRole("worker")
} 