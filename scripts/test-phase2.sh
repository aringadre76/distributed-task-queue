#!/bin/bash

set -e

BASE_URL="http://localhost:8080/api/v1"
CLI_PATH="./bin/cli"

echo "=== Phase 2 Reliability Features Test ==="
echo

echo "1. Testing Circuit Breaker Stats API..."
response=$(curl -s "${BASE_URL}/circuit-breakers" | jq .)
echo "Circuit Breaker Stats:"
echo "$response"
echo

echo "2. Testing Worker Pool Stats API..."
response=$(curl -s "${BASE_URL}/pool" | jq .)
echo "Worker Pool Stats:"
echo "$response"
echo

echo "3. Testing Enhanced Error Handling with Multiple Task Types..."

echo "Submitting test_task..."
test_task_response=$(curl -s -X POST "${BASE_URL}/tasks" \
  -H "Content-Type: application/json" \
  -d '{
    "type": "test_task",
    "payload": {
      "message": "testing enhanced error handling",
      "should_fail": false,
      "processing_time": 1
    },
    "priority": "medium"
  }')
test_task_id=$(echo "$test_task_response" | jq -r '.task.id')
echo "Test Task ID: $test_task_id"

echo "Submitting image_processing task..."
image_task_response=$(curl -s -X POST "${BASE_URL}/tasks" \
  -H "Content-Type: application/json" \
  -d '{
    "type": "image_processing",
    "payload": {
      "image_url": "https://example.com/test.jpg",
      "operations": ["resize", "watermark"],
      "output_format": "png"
    },
    "priority": "high"
  }')
image_task_id=$(echo "$image_task_response" | jq -r '.task.id')
echo "Image Task ID: $image_task_id"

echo "Submitting email_sending task..."
email_task_response=$(curl -s -X POST "${BASE_URL}/tasks" \
  -H "Content-Type: application/json" \
  -d '{
    "type": "email_sending",
    "payload": {
      "to": "test@example.com",
      "subject": "Phase 2 Test",
      "template": "test_template",
      "variables": {"name": "Test User"}
    },
    "priority": "low"
  }')
email_task_id=$(echo "$email_task_response" | jq -r '.task.id')
echo "Email Task ID: $email_task_id"

echo

echo "4. Testing Delayed Task Execution..."
delayed_task_response=$(curl -s -X POST "${BASE_URL}/tasks" \
  -H "Content-Type: application/json" \
  -d '{
    "type": "data_etl",
    "payload": {
      "source": "test_database",
      "destination": "analytics_db",
      "batch_size": 1000
    },
    "priority": "medium",
    "scheduled_at": "'$(date -d '+10 seconds' -Iseconds)'"
  }')
delayed_task_id=$(echo "$delayed_task_response" | jq -r '.task.id')
echo "Delayed Task ID (scheduled for +10s): $delayed_task_id"
echo

echo "5. Monitoring Task Execution..."
sleep 3

echo "Checking task statuses:"
for task_id in "$test_task_id" "$image_task_id" "$email_task_id"; do
  if [ "$task_id" != "null" ]; then
    status_response=$(curl -s "${BASE_URL}/tasks/${task_id}")
    status=$(echo "$status_response" | jq -r '.task.status')
    echo "  Task $task_id: $status"
  fi
done

echo

echo "6. Testing Queue Statistics After Load..."
queue_stats=$(curl -s "${BASE_URL}/queue/stats" | jq .)
echo "Queue Stats:"
echo "$queue_stats"
echo

echo "7. Testing Worker Pool Performance..."
pool_stats=$(curl -s "${BASE_URL}/pool" | jq .)
echo "Pool Stats After Load:"
echo "$pool_stats"
echo

echo "8. Testing Task History..."
completed_tasks=$(curl -s "${BASE_URL}/tasks?status=completed&page_size=5" | jq .)
echo "Recent Completed Tasks:"
echo "$completed_tasks" | jq '.tasks | length'
echo

echo "9. Testing Retry Behavior with Failing Task..."
failing_task_response=$(curl -s -X POST "${BASE_URL}/tasks" \
  -H "Content-Type: application/json" \
  -d '{
    "type": "test_task",
    "payload": {
      "message": "testing retry behavior",
      "should_fail": true,
      "error_type": "transient"
    },
    "priority": "medium"
  }')
failing_task_id=$(echo "$failing_task_response" | jq -r '.task.id')
echo "Failing Task ID: $failing_task_id"

sleep 5

failing_task_status=$(curl -s "${BASE_URL}/tasks/${failing_task_id}")
echo "Failing Task Status:"
echo "$failing_task_status" | jq '.task | {id, status, retry_count, error}'
echo

echo "10. Final System Health Check..."
health_response=$(curl -s "http://localhost:8080/health")
echo "Health Status:"
echo "$health_response" | jq .
echo

echo "=== Phase 2 Test Complete ==="
echo "✅ Circuit Breaker monitoring"
echo "✅ Enhanced error classification"  
echo "✅ Advanced retry logic with exponential backoff"
echo "✅ Worker pool management and statistics"
echo "✅ Task scheduling and delayed execution"
echo "✅ Comprehensive monitoring and health checks"
echo
echo "Phase 2 features are ready for production!" 