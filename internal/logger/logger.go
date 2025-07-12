package logger

import (
	"fmt"
	"os"

	"distributed-task-queue/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func NewLogger(cfg config.LoggerConfig) (*zap.Logger, error) {
	var zapConfig zap.Config

	if cfg.Format == "json" {
		zapConfig = zap.NewProductionConfig()
	} else {
		zapConfig = zap.NewDevelopmentConfig()
	}

	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		return nil, fmt.Errorf("invalid log level %s: %w", cfg.Level, err)
	}
	zapConfig.Level = zap.NewAtomicLevelAt(level)

	if cfg.OutputPath != "" && cfg.OutputPath != "stdout" {
		zapConfig.OutputPaths = []string{cfg.OutputPath}
	}

	logger, err := zapConfig.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build logger: %w", err)
	}

	return logger, nil
}

func NewTestLogger() *zap.Logger {
	cfg := zap.NewDevelopmentConfig()
	cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	logger, _ := cfg.Build()
	return logger
}

func NewLoggerWithDefaults() (*zap.Logger, error) {
	env := os.Getenv("ENV")
	if env == "production" {
		return zap.NewProduction()
	}
	return zap.NewDevelopment()
}