package worker

import (
	"context"
	"math"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"
)

type ErrorType string

const (
	ErrorTypeTransient     ErrorType = "transient"
	ErrorTypePermanent     ErrorType = "permanent"
	ErrorTypeRateLimit     ErrorType = "rate_limit"
	ErrorTypeTimeout       ErrorType = "timeout"
	ErrorTypeResource      ErrorType = "resource"
	ErrorTypeValidation    ErrorType = "validation"
	ErrorTypeNetwork       ErrorType = "network"
	ErrorTypeAuthentication ErrorType = "authentication"
)

type RetryStrategy struct {
	MaxRetries      int
	BaseDelay      time.Duration
	MaxDelay       time.Duration
	Multiplier     float64
	Jitter         bool
	BackoffType    string
}

type ErrorClassification struct {
	Type          ErrorType
	Severity      string
	ShouldRetry   bool
	RetryStrategy RetryStrategy
	Reason        string
}

type ErrorRule struct {
	Pattern       *regexp.Regexp
	ErrorType     ErrorType
	ShouldRetry   bool
	RetryStrategy RetryStrategy
}

type ErrorClassifier struct {
	rules  []ErrorRule
	logger *zap.Logger
}

func NewErrorClassifier(logger *zap.Logger) *ErrorClassifier {
	rules := []ErrorRule{
		{
			Pattern:     regexp.MustCompile(`(?i)timeout|deadline|context deadline exceeded`),
			ErrorType:   ErrorTypeTimeout,
			ShouldRetry: true,
			RetryStrategy: RetryStrategy{
				MaxRetries:  5,
				BaseDelay:   time.Second,
				MaxDelay:    30 * time.Second,
				Multiplier:  2.0,
				Jitter:      true,
				BackoffType: "exponential",
			},
		},
		{
			Pattern:     regexp.MustCompile(`(?i)network|connection|dns|unreachable`),
			ErrorType:   ErrorTypeNetwork,
			ShouldRetry: true,
			RetryStrategy: RetryStrategy{
				MaxRetries:  3,
				BaseDelay:   2 * time.Second,
				MaxDelay:    time.Minute,
				Multiplier:  2.0,
				Jitter:      true,
				BackoffType: "exponential",
			},
		},
		{
			Pattern:     regexp.MustCompile(`(?i)rate limit|too many requests|429`),
			ErrorType:   ErrorTypeRateLimit,
			ShouldRetry: true,
			RetryStrategy: RetryStrategy{
				MaxRetries:  10,
				BaseDelay:   30 * time.Second,
				MaxDelay:    5 * time.Minute,
				Multiplier:  1.5,
				Jitter:      true,
				BackoffType: "linear",
			},
		},
		{
			Pattern:     regexp.MustCompile(`(?i)validation|invalid|bad request|400`),
			ErrorType:   ErrorTypeValidation,
			ShouldRetry: false,
			RetryStrategy: RetryStrategy{
				MaxRetries:  0,
				BaseDelay:   0,
				MaxDelay:    0,
				Multiplier:  0,
				Jitter:      false,
				BackoffType: "none",
			},
		},
		{
			Pattern:     regexp.MustCompile(`(?i)unauthorized|forbidden|401|403`),
			ErrorType:   ErrorTypeAuthentication,
			ShouldRetry: false,
			RetryStrategy: RetryStrategy{
				MaxRetries:  0,
				BaseDelay:   0,
				MaxDelay:    0,
				Multiplier:  0,
				Jitter:      false,
				BackoffType: "none",
			},
		},
		{
			Pattern:     regexp.MustCompile(`(?i)resource|memory|disk|quota|limit`),
			ErrorType:   ErrorTypeResource,
			ShouldRetry: true,
			RetryStrategy: RetryStrategy{
				MaxRetries:  3,
				BaseDelay:   5 * time.Second,
				MaxDelay:    2 * time.Minute,
				Multiplier:  3.0,
				Jitter:      true,
				BackoffType: "exponential",
			},
		},
	}

	return &ErrorClassifier{
		rules:  rules,
		logger: logger,
	}
}

func (ec *ErrorClassifier) ClassifyError(err error) ErrorClassification {
	if err == nil {
		return ErrorClassification{
			Type:        ErrorTypeTransient,
			Severity:    "info",
			ShouldRetry: false,
			Reason:      "no error",
		}
	}

	errorMsg := strings.ToLower(err.Error())

	for _, rule := range ec.rules {
		if rule.Pattern.MatchString(errorMsg) {
			severity := "medium"
			if !rule.ShouldRetry {
				severity = "high"
			}

			classification := ErrorClassification{
				Type:          rule.ErrorType,
				Severity:      severity,
				ShouldRetry:   rule.ShouldRetry,
				RetryStrategy: rule.RetryStrategy,
				Reason:        "matched error pattern",
			}

			ec.logger.Debug("Error classified",
				zap.String("error", err.Error()),
				zap.String("type", string(classification.Type)),
				zap.Bool("should_retry", classification.ShouldRetry),
				zap.String("severity", classification.Severity))

			return classification
		}
	}

	defaultClassification := ErrorClassification{
		Type:        ErrorTypeTransient,
		Severity:    "low",
		ShouldRetry: true,
		RetryStrategy: RetryStrategy{
			MaxRetries:  3,
			BaseDelay:   time.Second,
			MaxDelay:    30 * time.Second,
			Multiplier:  2.0,
			Jitter:      true,
			BackoffType: "exponential",
		},
		Reason: "unclassified error - assuming transient",
	}

	ec.logger.Debug("Error not classified, using default",
		zap.String("error", err.Error()),
		zap.String("default_type", string(defaultClassification.Type)))

	return defaultClassification
}

func (ec *ErrorClassifier) CalculateBackoffDelay(classification ErrorClassification, retryCount int) time.Duration {
	strategy := classification.RetryStrategy
	
	if retryCount <= 0 || !classification.ShouldRetry {
		return strategy.BaseDelay
	}

	var delay time.Duration
	switch strategy.BackoffType {
	case "exponential":
		multiplier := math.Pow(strategy.Multiplier, float64(retryCount))
		delay = time.Duration(float64(strategy.BaseDelay) * multiplier)
	case "linear":
		delay = time.Duration(float64(strategy.BaseDelay) * float64(retryCount+1))
	case "fixed":
		delay = strategy.BaseDelay
	default:
		multiplier := math.Pow(strategy.Multiplier, float64(retryCount))
		delay = time.Duration(float64(strategy.BaseDelay) * multiplier)
	}

	if delay > strategy.MaxDelay {
		delay = strategy.MaxDelay
	}

	if strategy.Jitter {
		jitterAmount := float64(delay) * 0.1
		jitterOffset := (float64(time.Now().UnixNano()%1000)/1000.0 - 0.5) * 2.0
		jitter := time.Duration(jitterAmount * jitterOffset)
		delay += jitter
	}

	if delay < 0 {
		delay = strategy.BaseDelay
	}

	return delay
}

func (ec *ErrorClassifier) ShouldRetry(err error, retryCount int) bool {
	classification := ec.ClassifyError(err)
	
	if !classification.ShouldRetry {
		return false
	}

	if retryCount >= classification.RetryStrategy.MaxRetries {
		return false
	}

	return true
}

type BackoffCalculator struct {
	classifier  *ErrorClassifier
	logger      *zap.Logger
	baseDelay   time.Duration
	multiplier  float64
	maxDelay    time.Duration
	jitter      bool
	backoffType string
}

func NewBackoffCalculator(classifier *ErrorClassifier, logger *zap.Logger) *BackoffCalculator {
	return &BackoffCalculator{
		classifier:  classifier,
		logger:      logger,
		baseDelay:   time.Second,
		multiplier:  2.0,
		maxDelay:    time.Minute,
		jitter:      true,
		backoffType: "exponential",
	}
}

func (bc *BackoffCalculator) CalculateDelay(ctx context.Context, err error, retryCount int) time.Duration {
	classification := bc.classifier.ClassifyError(err)
	delay := bc.classifier.CalculateBackoffDelay(classification, retryCount)

	bc.logger.Debug("Calculated backoff delay",
		zap.String("error_type", string(classification.Type)),
		zap.Int("retry_count", retryCount),
		zap.Duration("delay", delay),
		zap.String("backoff_type", classification.RetryStrategy.BackoffType))

	return delay
} 