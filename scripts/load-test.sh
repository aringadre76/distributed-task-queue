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
METRICS_URL="http://localhost:9091/metrics"

# Load test parameters
DEFAULT_CONCURRENT_USERS=50
DEFAULT_REQUESTS_PER_USER=100
DEFAULT_TEST_DURATION=60
DEFAULT_RAMP_UP_TIME=10

# Test results
TOTAL_REQUESTS=0
SUCCESSFUL_REQUESTS=0
FAILED_REQUESTS=0
TOTAL_RESPONSE_TIME=0
MIN_RESPONSE_TIME=999999
MAX_RESPONSE_TIME=0
RESPONSE_TIMES=()

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
    echo -e "${CYAN}[LOAD TEST]${NC} $1"
}

log_phase() {
    echo -e "${PURPLE}[PHASE]${NC} $1"
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
    
    if ! command -v bc &> /dev/null; then
        log_error "bc is required for calculations"
        exit 1
    fi
    
    log_success "All dependencies available"
}

check_api_health() {
    log_info "Checking API server health..."
    
    if curl -s "http://localhost:8080/health" > /dev/null 2>&1; then
        log_success "API server is healthy"
        return 0
    else
        log_error "API server is not responding"
        return 1
    fi
}

get_auth_token() {
    log_info "Getting authentication token..."
    
    login_response=$(curl -s -X POST "$AUTH_BASE/login" \
        -H "Content-Type: application/json" \
        -d '{
            "username": "admin",
            "password": "admin123",
            "tenant_id": "load-test-tenant"
        }')
    
    if echo "$login_response" | jq -e '.access_token' > /dev/null; then
        AUTH_TOKEN=$(echo "$login_response" | jq -r '.access_token')
        log_success "Authentication token obtained"
        return 0
    else
        log_error "Failed to get authentication token"
        return 1
    fi
}

measure_response_time() {
    local url="$1"
    local method="${2:-GET}"
    local data="${3:-}"
    local headers="${4:-}"
    
    local start_time=$(date +%s%N)
    
    if [ -n "$data" ]; then
        if [ -n "$headers" ]; then
            response=$(curl -s -w "%{http_code}" -o /tmp/response_body \
                -X "$method" \
                -H "$headers" \
                -H "Content-Type: application/json" \
                -d "$data" \
                "$url")
        else
            response=$(curl -s -w "%{http_code}" -o /tmp/response_body \
                -X "$method" \
                -H "Content-Type: application/json" \
                -d "$data" \
                "$url")
        fi
    else
        if [ -n "$headers" ]; then
            response=$(curl -s -w "%{http_code}" -o /tmp/response_body \
                -X "$method" \
                -H "$headers" \
                "$url")
        else
            response=$(curl -s -w "%{http_code}" -o /tmp/response_body \
                -X "$method" \
                "$url")
        fi
    fi
    
    local end_time=$(date +%s%N)
    local response_time=$(( (end_time - start_time) / 1000000 ))  # Convert to milliseconds
    
    echo "$response_time:$response"
}

load_test_health_endpoint() {
    log_test "Testing health endpoint under load..."
    
    local concurrent_users=$1
    local requests_per_user=$2
    
    log_info "Starting health endpoint load test: $concurrent_users users, $requests_per_user requests each"
    
    local total_requests=$((concurrent_users * requests_per_user))
    local completed_requests=0
    
    for ((i=1; i<=concurrent_users; i++)); do
        (
            for ((j=1; j<=requests_per_user; j++)); do
                result=$(measure_response_time "http://localhost:8080/health")
                response_time=$(echo "$result" | cut -d: -f1)
                http_code=$(echo "$result" | cut -d: -f2)
                
                if [ "$http_code" = "200" ]; then
                    SUCCESSFUL_REQUESTS=$((SUCCESSFUL_REQUESTS + 1))
                else
                    FAILED_REQUESTS=$((FAILED_REQUESTS + 1))
                fi
                
                TOTAL_REQUESTS=$((TOTAL_REQUESTS + 1))
                TOTAL_RESPONSE_TIME=$((TOTAL_RESPONSE_TIME + response_time))
                RESPONSE_TIMES+=("$response_time")
                
                if [ $response_time -lt $MIN_RESPONSE_TIME ]; then
                    MIN_RESPONSE_TIME=$response_time
                fi
                
                if [ $response_time -gt $MAX_RESPONSE_TIME ]; then
                    MAX_RESPONSE_TIME=$response_time
                fi
                
                completed_requests=$((completed_requests + 1))
                if [ $((completed_requests % 100)) -eq 0 ]; then
                    log_info "Completed $completed_requests/$total_requests requests"
                fi
            done
        ) &
    done
    
    wait
    log_success "Health endpoint load test completed"
}

load_test_task_submission() {
    log_test "Testing task submission under load..."
    
    local concurrent_users=$1
    local requests_per_user=$2
    
    log_info "Starting task submission load test: $concurrent_users users, $requests_per_user requests each"
    
    local total_requests=$((concurrent_users * requests_per_user))
    local completed_requests=0
    
    for ((i=1; i<=concurrent_users; i++)); do
        (
            for ((j=1; j<=requests_per_user; j++)); do
                task_data="{\"type\": \"load_test_task\", \"payload\": {\"user\": $i, \"request\": $j, \"timestamp\": $(date +%s)}, \"priority\": \"medium\"}"
                
                result=$(measure_response_time "$API_BASE/tasks" "POST" "$task_data" "Authorization: Bearer $AUTH_TOKEN")
                response_time=$(echo "$result" | cut -d: -f1)
                http_code=$(echo "$result" | cut -d: -f2)
                
                if [ "$http_code" = "200" ] || [ "$http_code" = "201" ]; then
                    SUCCESSFUL_REQUESTS=$((SUCCESSFUL_REQUESTS + 1))
                else
                    FAILED_REQUESTS=$((FAILED_REQUESTS + 1))
                fi
                
                TOTAL_REQUESTS=$((TOTAL_REQUESTS + 1))
                TOTAL_RESPONSE_TIME=$((TOTAL_RESPONSE_TIME + response_time))
                RESPONSE_TIMES+=("$response_time")
                
                if [ $response_time -lt $MIN_RESPONSE_TIME ]; then
                    MIN_RESPONSE_TIME=$response_time
                fi
                
                if [ $response_time -gt $MAX_RESPONSE_TIME ]; then
                    MAX_RESPONSE_TIME=$response_time
                fi
                
                completed_requests=$((completed_requests + 1))
                if [ $((completed_requests % 100)) -eq 0 ]; then
                    log_info "Completed $completed_requests/$total_requests requests"
                fi
            done
        ) &
    done
    
    wait
    log_success "Task submission load test completed"
}

load_test_batch_submission() {
    log_test "Testing batch task submission under load..."
    
    local concurrent_users=$1
    local requests_per_user=$2
    
    log_info "Starting batch submission load test: $concurrent_users users, $requests_per_user requests each"
    
    local total_requests=$((concurrent_users * requests_per_user))
    local completed_requests=0
    
    for ((i=1; i<=concurrent_users; i++)); do
        (
            for ((j=1; j<=requests_per_user; j++)); do
                batch_data="{\"tasks\": [{\"type\": \"batch_test_task\", \"payload\": {\"user\": $i, \"batch\": $j}, \"priority\": \"low\"}, {\"type\": \"batch_test_task\", \"payload\": {\"user\": $i, \"batch\": $j}, \"priority\": \"medium\"}, {\"type\": \"batch_test_task\", \"payload\": {\"user\": $i, \"batch\": $j}, \"priority\": \"high\"}]}"
                
                result=$(measure_response_time "$API_BASE/tasks/batch" "POST" "$batch_data" "Authorization: Bearer $AUTH_TOKEN")
                response_time=$(echo "$result" | cut -d: -f1)
                http_code=$(echo "$result" | cut -d: -f1)
                
                if [ "$http_code" = "200" ] || [ "$http_code" = "201" ]; then
                    SUCCESSFUL_REQUESTS=$((SUCCESSFUL_REQUESTS + 1))
                else
                    FAILED_REQUESTS=$((FAILED_REQUESTS + 1))
                fi
                
                TOTAL_REQUESTS=$((TOTAL_REQUESTS + 1))
                TOTAL_RESPONSE_TIME=$((TOTAL_RESPONSE_TIME + response_time))
                RESPONSE_TIMES+=("$response_time")
                
                if [ $response_time -lt $MIN_RESPONSE_TIME ]; then
                    MIN_RESPONSE_TIME=$response_time
                fi
                
                if [ $response_time -gt $MAX_RESPONSE_TIME ]; then
                    MAX_RESPONSE_TIME=$response_time
                fi
                
                completed_requests=$((completed_requests + 1))
                if [ $((completed_requests % 50)) -eq 0 ]; then
                    log_info "Completed $completed_requests/$total_requests batch requests"
                fi
            done
        ) &
    done
    
    wait
    log_success "Batch submission load test completed"
}

calculate_percentile() {
    local percentile=$1
    local sorted_times=($(printf '%s\n' "${RESPONSE_TIMES[@]}" | sort -n))
    local total=${#sorted_times[@]}
    local index=$((total * percentile / 100))
    
    if [ $index -ge $total ]; then
        index=$((total - 1))
    fi
    
    echo "${sorted_times[$index]}"
}

calculate_statistics() {
    log_info "Calculating load test statistics..."
    
    if [ $TOTAL_REQUESTS -eq 0 ]; then
        log_error "No requests were made"
        return 1
    fi
    
    local success_rate=$(echo "scale=2; $SUCCESSFUL_REQUESTS * 100 / $TOTAL_REQUESTS" | bc)
    local avg_response_time=$(echo "scale=2; $TOTAL_RESPONSE_TIME / $TOTAL_REQUESTS" | bc)
    local p50_response_time=$(calculate_percentile 50)
    local p95_response_time=$(calculate_percentile 95)
    local p99_response_time=$(calculate_percentile 99)
    local requests_per_second=$(echo "scale=2; $TOTAL_REQUESTS / $DEFAULT_TEST_DURATION" | bc)
    
    echo
    echo "=========================================="
    echo "           LOAD TEST RESULTS"
    echo "=========================================="
    echo "Total Requests: $TOTAL_REQUESTS"
    echo "Successful Requests: $SUCCESSFUL_REQUESTS"
    echo "Failed Requests: $FAILED_REQUESTS"
    echo "Success Rate: ${success_rate}%"
    echo
    echo "Response Time Statistics (ms):"
    echo "  Average: ${avg_response_time}ms"
    echo "  Minimum: ${MIN_RESPONSE_TIME}ms"
    echo "  Maximum: ${MAX_RESPONSE_TIME}ms"
    echo "  50th Percentile (P50): ${p50_response_time}ms"
    echo "  95th Percentile (P95): ${p95_response_time}ms"
    echo "  99th Percentile (P99): ${p99_response_time}ms"
    echo
    echo "Throughput: ${requests_per_second} requests/second"
    echo
    
    # Check if we meet the 1000+ tasks/second goal
    if (( $(echo "$requests_per_second >= 1000" | bc -l) )); then
        log_success "🎉 THROUGHPUT GOAL ACHIEVED: ${requests_per_second} requests/second"
        log_success "System meets the 1000+ tasks/second requirement!"
    else
        log_warning "⚠️ Throughput goal not met: ${requests_per_second} requests/second"
        log_warning "Target was 1000+ requests/second"
    fi
    
    # Performance assessment
    if (( $(echo "$avg_response_time < 100" | bc -l) )); then
        log_success "Excellent average response time: ${avg_response_time}ms"
    elif (( $(echo "$avg_response_time < 500" | bc -l) )); then
        log_success "Good average response time: ${avg_response_time}ms"
    else
        log_warning "Slow average response time: ${avg_response_time}ms"
    fi
    
    if (( $(echo "$success_rate >= 99" | bc -l) )); then
        log_success "Excellent success rate: ${success_rate}%"
    elif (( $(echo "$success_rate >= 95" | bc -l) )); then
        log_success "Good success rate: ${success_rate}%"
    else
        log_warning "Low success rate: ${success_rate}%"
    fi
}

get_system_metrics() {
    log_info "Collecting system metrics..."
    
    if curl -s "$METRICS_URL" > /dev/null 2>&1; then
        log_success "Prometheus metrics endpoint accessible"
        
        # Get queue depth
        queue_depth=$(curl -s "$METRICS_URL" | grep "dtq_queue_depth" | grep -v "#" | awk '{print $2}' | head -1)
        if [ -n "$queue_depth" ]; then
            log_info "Current queue depth: $queue_depth"
        fi
        
        # Get active workers
        active_workers=$(curl -s "$METRICS_URL" | grep "dtq_active_workers" | grep -v "#" | awk '{print $2}' | head -1)
        if [ -n "$active_workers" ]; then
            log_info "Active workers: $active_workers"
        fi
    else
        log_warning "Prometheus metrics endpoint not accessible"
    fi
}

run_load_test() {
    local concurrent_users=${1:-$DEFAULT_CONCURRENT_USERS}
    local requests_per_user=${2:-$DEFAULT_REQUESTS_PER_USER}
    local test_duration=${3:-$DEFAULT_TEST_DURATION}
    
    log_phase "Starting Load Test"
    echo "================================"
    echo "Concurrent Users: $concurrent_users"
    echo "Requests per User: $requests_per_user"
    echo "Test Duration: ${test_duration}s"
    echo "Total Expected Requests: $((concurrent_users * requests_per_user))"
    echo
    
    # Reset counters
    TOTAL_REQUESTS=0
    SUCCESSFUL_REQUESTS=0
    FAILED_REQUESTS=0
    TOTAL_RESPONSE_TIME=0
    MIN_RESPONSE_TIME=999999
    MAX_RESPONSE_TIME=0
    RESPONSE_TIMES=()
    
    # Get system metrics before test
    get_system_metrics
    
    # Run load tests
    load_test_health_endpoint $concurrent_users $requests_per_user
    load_test_task_submission $concurrent_users $requests_per_user
    load_test_batch_submission $((concurrent_users / 2)) $((requests_per_user / 2))
    
    # Calculate and display results
    calculate_statistics
    
    # Get system metrics after test
    get_system_metrics
}

main() {
    echo "=========================================="
    echo "  DISTRIBUTED TASK QUEUE LOAD TEST"
    echo "=========================================="
    echo "Testing system performance and throughput"
    echo
    
    check_dependencies
    check_api_health
    get_auth_token
    
    # Parse command line arguments
    CONCURRENT_USERS=${1:-$DEFAULT_CONCURRENT_USERS}
    REQUESTS_PER_USER=${2:-$DEFAULT_REQUESTS_PER_USER}
    TEST_DURATION=${3:-$DEFAULT_TEST_DURATION}
    
    run_load_test $CONCURRENT_USERS $REQUESTS_PER_USER $TEST_DURATION
    
    echo
    log_success "Load test completed successfully!"
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi 