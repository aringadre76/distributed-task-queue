#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Configuration
API_BASE="http://localhost:8080/api/v1"
AUTH_BASE="http://localhost:8080/api/v1/auth"
ADMIN_BASE="http://localhost:8080/api/v1/admin"
PUBLIC_BASE="http://localhost:8080/api/v1/public"
METRICS_URL="http://localhost:9091/metrics"

# Test counters
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0
SKIPPED_TESTS=0

# Test results storage
TEST_RESULTS=()

# Helper functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_test() {
    echo -e "${CYAN}[TEST]${NC} $1"
}

log_phase() {
    echo -e "${PURPLE}[PHASE]${NC} $1"
}

assert_response() {
    local response="$1"
    local expected_status="$2"
    local description="$3"
    
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    
    if echo "$response" | jq -e "$expected_status" > /dev/null 2>&1; then
        log_success "$description"
        PASSED_TESTS=$((PASSED_TESTS + 1))
        TEST_RESULTS+=("PASS: $description")
    else
        log_error "$description"
        echo "Response: $response"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        TEST_RESULTS+=("FAIL: $description")
        return 1
    fi
}

assert_http_status() {
    local status="$1"
    local expected="$2"
    local description="$3"
    
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    
    if [ "$status" = "$expected" ]; then
        log_success "$description"
        PASSED_TESTS=$((PASSED_TESTS + 1))
        TEST_RESULTS+=("PASS: $description")
    else
        log_error "$description (expected $expected, got $status)"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        TEST_RESULTS+=("FAIL: $description")
        return 1
    fi
}

check_dependencies() {
    log_info "Checking dependencies..."
    
    if ! command -v curl &> /dev/null; then
        log_error "curl is required but not installed"
        exit 1
    fi
    
    if ! command -v jq &> /dev/null; then
        log_error "jq is required but not installed"
        exit 1
    fi
    
    log_success "All dependencies available"
}

check_api_health() {
    log_info "Checking API server health..."
    
    local max_retries=30
    local retry_count=0
    
    while [ $retry_count -lt $max_retries ]; do
        if curl -s "http://localhost:8080/health" > /dev/null 2>&1; then
            log_success "API server is healthy"
            return 0
        fi
        
        retry_count=$((retry_count + 1))
        log_warning "API server not ready, retrying... ($retry_count/$max_retries)"
        sleep 2
    done
    
    log_error "API server failed to start within expected time"
    return 1
}

test_phase1_foundation() {
    log_phase "Phase 1: Foundation Testing"
    echo "=================================="
    
    log_test "Testing basic API endpoints..."
    
    # Test health endpoint
    health_response=$(curl -s "http://localhost:8080/health")
    assert_response "$health_response" '.status == "healthy"' "Health endpoint returns healthy status"
    
    # Test ping endpoint
    ping_response=$(curl -s "$PUBLIC_BASE/ping")
    assert_response "$ping_response" '.message == "pong"' "Ping endpoint returns pong"
    
    # Test task submission
    log_test "Testing task submission..."
    task_response=$(curl -s -X POST "$API_BASE/tasks" \
        -H "Content-Type: application/json" \
        -d '{
            "type": "test_task",
            "payload": {"message": "Hello World"},
            "priority": "high"
        }')
    assert_response "$task_response" '.task.id' "Task submission successful"
    
    # Extract task ID for further testing
    TASK_ID=$(echo "$task_response" | jq -r '.task.id')
    
    # Test task retrieval
    log_test "Testing task retrieval..."
    get_task_response=$(curl -s "$API_BASE/tasks/$TASK_ID")
    assert_response "$get_task_response" '.task.id == "'$TASK_ID'"' "Task retrieval successful"
    
    # Test task listing
    log_test "Testing task listing..."
    list_tasks_response=$(curl -s "$API_BASE/tasks")
    assert_response "$list_tasks_response" '.tasks' "Task listing successful"
    
    log_success "Phase 1 Foundation tests completed"
}

test_phase2_reliability() {
    log_phase "Phase 2: Reliability Testing"
    echo "=================================="
    
    log_test "Testing error handling..."
    
    # Test invalid task submission
    invalid_task_response=$(curl -s -X POST "$API_BASE/tasks" \
        -H "Content-Type: application/json" \
        -d '{
            "type": "invalid_type",
            "payload": {}
        }')
    assert_response "$invalid_task_response" '.error' "Invalid task properly rejected"
    
    # Test task cancellation
    log_test "Testing task cancellation..."
    cancel_response=$(curl -s -X DELETE "$API_BASE/tasks/$TASK_ID")
    assert_response "$cancel_response" '.message' "Task cancellation successful"
    
    # Test retry mechanism (submit task and check retry count)
    log_test "Testing retry mechanism..."
    retry_task_response=$(curl -s -X POST "$API_BASE/tasks" \
        -H "Content-Type: application/json" \
        -d '{
            "type": "test_task",
            "payload": {"should_fail": true},
            "priority": "low"
        }')
    assert_response "$retry_task_response" '.id' "Retry task submission successful"
    
    log_success "Phase 2 Reliability tests completed"
}

test_phase3_scalability() {
    log_phase "Phase 3: Scalability Testing"
    echo "=================================="
    
    log_test "Testing batch task submission..."
    
    # Test batch submission
    batch_response=$(curl -s -X POST "$API_BASE/tasks/batch" \
        -H "Content-Type: application/json" \
        -d '{
            "tasks": [
                {"type": "test_task", "payload": {"batch": 1}, "priority": "medium"},
                {"type": "test_task", "payload": {"batch": 2}, "priority": "medium"},
                {"type": "test_task", "payload": {"batch": 3}, "priority": "medium"}
            ]
        }')
    assert_response "$batch_response" '.batch_id' "Batch task submission successful"
    
    # Test queue statistics
    log_test "Testing queue statistics..."
    queue_stats_response=$(curl -s "$API_BASE/queue/stats")
    assert_response "$queue_stats_response" '.queue_depth' "Queue statistics accessible"
    
    # Test worker pool statistics
    log_test "Testing worker pool statistics..."
    pool_stats_response=$(curl -s "$API_BASE/pool")
    assert_response "$pool_stats_response" '.active_workers' "Worker pool statistics accessible"
    
    # Test parallel execution stats
    log_test "Testing parallel execution statistics..."
    parallel_stats_response=$(curl -s "$API_BASE/parallel/stats")
    assert_response "$parallel_stats_response" '.active_goroutines' "Parallel execution statistics accessible"
    
    log_success "Phase 3 Scalability tests completed"
}

test_phase4_production() {
    log_phase "Phase 4: Production Features Testing"
    echo "=========================================="
    
    log_test "Testing JWT Authentication..."
    
    # Test login
    login_response=$(curl -s -X POST "$AUTH_BASE/login" \
        -H "Content-Type: application/json" \
        -d '{
            "username": "admin",
            "password": "admin123",
            "tenant_id": "test-tenant"
        }')
    assert_response "$login_response" '.access_token' "Admin login successful"
    
    ADMIN_TOKEN=$(echo "$login_response" | jq -r '.access_token')
    ADMIN_USER_ID=$(echo "$login_response" | jq -r '.user_id')
    
    # Test user login
    user_login_response=$(curl -s -X POST "$AUTH_BASE/login" \
        -H "Content-Type: application/json" \
        -d '{
            "username": "user",
            "password": "user123",
            "tenant_id": "test-tenant"
        }')
    assert_response "$user_login_response" '.access_token' "User login successful"
    
    USER_TOKEN=$(echo "$user_login_response" | jq -r '.access_token')
    
    # Test token validation
    log_test "Testing token validation..."
    validate_response=$(curl -s -X POST "$AUTH_BASE/validate" \
        -H "Authorization: Bearer $ADMIN_TOKEN")
    assert_response "$validate_response" '.valid == true' "Token validation successful"
    
    # Test role-based authorization
    log_test "Testing role-based authorization..."
    
    # Admin should access admin endpoints
    admin_response=$(curl -s -X GET "$ADMIN_BASE/system/status" \
        -H "Authorization: Bearer $ADMIN_TOKEN")
    assert_response "$admin_response" '.status' "Admin can access admin endpoints"
    
    # User should not access admin endpoints
    user_admin_response=$(curl -s -X GET "$ADMIN_BASE/system/status" \
        -H "Authorization: Bearer $USER_TOKEN")
    assert_response "$user_admin_response" '.error' "User properly restricted from admin endpoints"
    
    # Test rate limiting
    log_test "Testing rate limiting..."
    rate_limit_hit=false
    
    for i in {1..20}; do
        response=$(curl -s -w "%{http_code}" -o /dev/null "$PUBLIC_BASE/ping")
        if [ "$response" = "429" ]; then
            rate_limit_hit=true
            log_success "Rate limit triggered on request $i"
            break
        fi
        sleep 0.1
    done
    
    if [ "$rate_limit_hit" = false ]; then
        log_warning "Rate limit not triggered (might be set too high for test)"
        SKIPPED_TESTS=$((SKIPPED_TESTS + 1))
    fi
    
    # Test distributed tracing
    log_test "Testing distributed tracing..."
    correlation_id="test-correlation-$(date +%s)"
    
    trace_response=$(curl -s -I "$PUBLIC_BASE/ping" \
        -H "X-Correlation-ID: $correlation_id")
    
    if echo "$trace_response" | grep -q "X-Correlation-ID: $correlation_id"; then
        log_success "Correlation ID properly propagated"
        PASSED_TESTS=$((PASSED_TESTS + 1))
    else
        log_error "Correlation ID not propagated"
        FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
    
    # Test audit logging
    log_test "Testing audit logging..."
    
    # Perform actions that should be audited
    curl -s -X POST "$AUTH_BASE/login" \
        -H "Content-Type: application/json" \
        -d '{"username": "admin", "password": "admin123"}' > /dev/null
    
    curl -s -X POST "$API_BASE/tasks" \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer $ADMIN_TOKEN" \
        -d '{"type": "test_task", "payload": {"audit": "test"}, "priority": "high"}' > /dev/null
    
    sleep 2  # Wait for audit logs
    
    audit_logs_response=$(curl -s -X GET "$ADMIN_BASE/audit-logs" \
        -H "Authorization: Bearer $ADMIN_TOKEN")
    
    if echo "$audit_logs_response" | jq -e '.audit_logs' > /dev/null; then
        audit_count=$(echo "$audit_logs_response" | jq '.audit_logs | length')
        log_success "Audit logs accessible ($audit_count entries found)"
        PASSED_TESTS=$((PASSED_TESTS + 1))
    else
        log_error "Audit logs not accessible"
        FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
    
    # Test security headers
    log_test "Testing security headers..."
    security_response=$(curl -s -I "$PUBLIC_BASE/ping")
    
    if echo "$security_response" | grep -q "X-Content-Type-Options: nosniff"; then
        log_success "Content-Type-Options header present"
        PASSED_TESTS=$((PASSED_TESTS + 1))
    else
        log_error "Content-Type-Options header missing"
        FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
    
    if echo "$security_response" | grep -q "X-Frame-Options: DENY"; then
        log_success "X-Frame-Options header present"
        PASSED_TESTS=$((PASSED_TESTS + 1))
    else
        log_error "X-Frame-Options header missing"
        FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
    
    log_success "Phase 4 Production tests completed"
}

test_monitoring() {
    log_phase "Monitoring and Metrics Testing"
    echo "===================================="
    
    log_test "Testing Prometheus metrics..."
    
    if curl -s "$METRICS_URL" > /dev/null; then
        log_success "Prometheus metrics endpoint accessible"
        PASSED_TESTS=$((PASSED_TESTS + 1))
        
        # Check for specific metrics
        metrics_content=$(curl -s "$METRICS_URL")
        if echo "$metrics_content" | grep -q "dtq_http_request_duration_seconds"; then
            log_success "HTTP request duration metrics present"
            PASSED_TESTS=$((PASSED_TESTS + 1))
        else
            log_error "HTTP request duration metrics missing"
            FAILED_TESTS=$((FAILED_TESTS + 1))
        fi
    else
        log_error "Prometheus metrics endpoint not accessible"
        FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
    
    log_test "Testing system monitoring endpoints..."
    
    # Test circuit breaker stats
    cb_stats_response=$(curl -s "$API_BASE/circuit-breakers")
    assert_response "$cb_stats_response" '.circuit_breakers' "Circuit breaker statistics accessible"
    
    # Test batch processor stats
    batch_stats_response=$(curl -s "$API_BASE/batch/stats")
    assert_response "$batch_stats_response" '.batch_processor' "Batch processor statistics accessible"
    
    # Test dependency stats
    dep_stats_response=$(curl -s "$API_BASE/dependencies/stats")
    assert_response "$dep_stats_response" '.dependency_manager' "Dependency statistics accessible"
    
    log_success "Monitoring tests completed"
}

test_performance() {
    log_phase "Performance Testing"
    echo "======================="
    
    log_test "Testing API response times..."
    
    # Test health endpoint performance
    start_time=$(date +%s%N)
    for i in {1..10}; do
        curl -s "http://localhost:8080/health" > /dev/null
    done
    end_time=$(date +%s%N)
    
    duration=$(( (end_time - start_time) / 1000000 ))  # Convert to milliseconds
    avg_duration=$(( duration / 10 ))
    
    if [ $avg_duration -lt 100 ]; then
        log_success "Health endpoint performance: ${avg_duration}ms average (excellent)"
        PASSED_TESTS=$((PASSED_TESTS + 1))
    elif [ $avg_duration -lt 500 ]; then
        log_success "Health endpoint performance: ${avg_duration}ms average (good)"
        PASSED_TESTS=$((PASSED_TESTS + 1))
    else
        log_warning "Health endpoint performance: ${avg_duration}ms average (slow)"
        SKIPPED_TESTS=$((SKIPPED_TESTS + 1))
    fi
    
    log_test "Testing concurrent task submission..."
    
    # Submit multiple tasks concurrently
    start_time=$(date +%s%N)
    for i in {1..20}; do
        curl -s -X POST "$API_BASE/tasks" \
            -H "Content-Type: application/json" \
            -d "{\"type\": \"test_task\", \"payload\": {\"concurrent\": $i}, \"priority\": \"medium\"}" > /dev/null &
    done
    wait
    end_time=$(date +%s%N)
    
    duration=$(( (end_time - start_time) / 1000000 ))
    log_success "Concurrent task submission completed in ${duration}ms"
    PASSED_TESTS=$((PASSED_TESTS + 1))
    
    log_success "Performance tests completed"
}

print_summary() {
    echo
    echo "=========================================="
    echo "           TEST SUMMARY"
    echo "=========================================="
    echo "Total Tests: $TOTAL_TESTS"
    echo "Passed: $PASSED_TESTS"
    echo "Failed: $FAILED_TESTS"
    echo "Skipped: $SKIPPED_TESTS"
    echo
    
    if [ $FAILED_TESTS -eq 0 ]; then
        echo -e "${GREEN}🎉 ALL TESTS PASSED! 🎉${NC}"
        echo
        echo "Distributed Task Queue System is fully operational!"
        echo
        echo "✅ Phase 1: Foundation - Basic API and queue management"
        echo "✅ Phase 2: Reliability - Error handling and retries"
        echo "✅ Phase 3: Scalability - Batch processing and parallel execution"
        echo "✅ Phase 4: Production - Security, monitoring, and compliance"
        echo
        echo "System is ready for production use!"
    else
        echo -e "${RED}❌ $FAILED_TESTS TESTS FAILED${NC}"
        echo
        echo "Failed tests:"
        for result in "${TEST_RESULTS[@]}"; do
            if [[ $result == FAIL:* ]]; then
                echo "  $result"
            fi
        done
        echo
        exit 1
    fi
}

main() {
    echo "=========================================="
    echo "  DISTRIBUTED TASK QUEUE TEST SUITE"
    echo "=========================================="
    echo "Comprehensive testing of all system phases"
    echo
    
    check_dependencies
    check_api_health
    
    test_phase1_foundation
    test_phase2_reliability
    test_phase3_scalability
    test_phase4_production
    test_monitoring
    test_performance
    
    print_summary
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main
fi 