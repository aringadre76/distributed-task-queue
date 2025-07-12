package worker

import (
	"sync"
	"time"

	"go.uber.org/zap"
)

type CircuitBreakerState int

const (
	CircuitBreakerClosed CircuitBreakerState = iota
	CircuitBreakerOpen
	CircuitBreakerHalfOpen
)

type CircuitBreakerConfig struct {
	FailureThreshold   int
	SuccessThreshold   int
	Timeout           time.Duration
	MonitoringPeriod  time.Duration
}

type CircuitBreaker struct {
	config           CircuitBreakerConfig
	state            CircuitBreakerState
	failureCount     int
	successCount     int
	lastFailureTime  time.Time
	mutex            sync.RWMutex
	logger           *zap.Logger
}

type TaskTypeCircuitBreakers struct {
	breakers map[string]*CircuitBreaker
	config   CircuitBreakerConfig
	mutex    sync.RWMutex
	logger   *zap.Logger
}

func NewTaskTypeCircuitBreakers(config CircuitBreakerConfig, logger *zap.Logger) *TaskTypeCircuitBreakers {
	return &TaskTypeCircuitBreakers{
		breakers: make(map[string]*CircuitBreaker),
		config:   config,
		logger:   logger,
	}
}

func (tcb *TaskTypeCircuitBreakers) GetCircuitBreaker(taskType string) *CircuitBreaker {
	tcb.mutex.RLock()
	breaker, exists := tcb.breakers[taskType]
	tcb.mutex.RUnlock()

	if exists {
		return breaker
	}

	tcb.mutex.Lock()
	defer tcb.mutex.Unlock()

	if breaker, exists := tcb.breakers[taskType]; exists {
		return breaker
	}

	breaker = &CircuitBreaker{
		config: tcb.config,
		state:  CircuitBreakerClosed,
		logger: tcb.logger.With(zap.String("task_type", taskType)),
	}
	tcb.breakers[taskType] = breaker

	return breaker
}

func (cb *CircuitBreaker) CanExecute() bool {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	switch cb.state {
	case CircuitBreakerClosed:
		return true
	case CircuitBreakerOpen:
		if time.Since(cb.lastFailureTime) >= cb.config.Timeout {
			cb.mutex.RUnlock()
			cb.mutex.Lock()
			if cb.state == CircuitBreakerOpen {
				cb.state = CircuitBreakerHalfOpen
				cb.successCount = 0
				cb.logger.Info("Circuit breaker transitioning to half-open")
			}
			cb.mutex.Unlock()
			cb.mutex.RLock()
			return cb.state == CircuitBreakerHalfOpen
		}
		return false
	case CircuitBreakerHalfOpen:
		return true
	default:
		return false
	}
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failureCount = 0

	switch cb.state {
	case CircuitBreakerHalfOpen:
		cb.successCount++
		if cb.successCount >= cb.config.SuccessThreshold {
			cb.state = CircuitBreakerClosed
			cb.successCount = 0
			cb.logger.Info("Circuit breaker closed after successful recovery")
		}
	case CircuitBreakerClosed:
	}
}

func (cb *CircuitBreaker) RecordFailure(err error) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failureCount++
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case CircuitBreakerClosed:
		if cb.failureCount >= cb.config.FailureThreshold {
			cb.state = CircuitBreakerOpen
			cb.logger.Warn("Circuit breaker opened due to failures",
				zap.Int("failure_count", cb.failureCount),
				zap.Error(err))
		}
	case CircuitBreakerHalfOpen:
		cb.state = CircuitBreakerOpen
		cb.logger.Warn("Circuit breaker reopened after failure in half-open state",
			zap.Error(err))
	}
}

func (cb *CircuitBreaker) GetState() CircuitBreakerState {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.state
}

func (cb *CircuitBreaker) GetStats() map[string]interface{} {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	return map[string]interface{}{
		"state":            cb.state,
		"failure_count":    cb.failureCount,
		"success_count":    cb.successCount,
		"last_failure_time": cb.lastFailureTime,
	}
} 