#!/bin/bash

set -e

echo "Submitting test tasks to the Distributed Task Queue..."

API_URL="${API_URL:-http://localhost:8080}"
CLI_TOOL="${CLI_TOOL:-./bin/dtq-cli}"

echo "Using API URL: $API_URL"

submit_task() {
    local type="$1"
    local payload="$2"
    local priority="${3:-medium}"
    local description="$4"
    
    echo "Submitting $description..."
    
    if command -v "$CLI_TOOL" >/dev/null 2>&1; then
        result=$($CLI_TOOL task submit "$type" "$payload" --priority="$priority" --api-url="$API_URL" 2>&1)
        echo "  Result: $result"
    else
        result=$(curl -s -X POST "$API_URL/api/v1/tasks" \
            -H "Content-Type: application/json" \
            -d "{\"type\":\"$type\",\"payload\":$payload,\"priority\":\"$priority\"}")
        task_id=$(echo "$result" | jq -r '.id // "unknown"')
        echo "  Task ID: $task_id"
    fi
    
    echo ""
}

echo "=== Test Task Examples ==="

submit_task "test_task" '{"message":"Hello World","sleep_duration":2}' "high" "Quick test task"

submit_task "test_task" '{"message":"Background processing","sleep_duration":5}' "medium" "Medium priority task"

submit_task "test_task" '{"message":"Low priority task","sleep_duration":1}' "low" "Low priority task"

echo "=== Image Processing Examples ==="

submit_task "image_processing" '{"image_url":"https://example.com/image.jpg","operations":["resize","watermark"],"output_format":"png"}' "high" "Image processing task"

submit_task "image_processing" '{"images":["img1.jpg","img2.jpg","img3.jpg"],"resize":{"width":800,"height":600},"quality":85}' "medium" "Batch image processing"

echo "=== Email Sending Examples ==="

submit_task "email_sending" '{"recipient":"user@example.com","subject":"Welcome!","template":"welcome_email","data":{"name":"John Doe"}}' "medium" "Welcome email"

submit_task "email_sending" '{"recipients":["user1@example.com","user2@example.com"],"subject":"Newsletter","template":"newsletter","campaign_id":"newsletter_2024_01"}' "low" "Newsletter campaign"

echo "=== Data ETL Examples ==="

submit_task "data_etl" '{"source":"users.csv","destination":"postgres://users_table","transformations":["validate_emails","normalize_names"],"batch_size":1000}' "medium" "User data ETL"

submit_task "data_etl" '{"source":"api://analytics/events","destination":"warehouse://events","window":"1h","aggregations":["count","sum","avg"]}' "high" "Real-time analytics ETL"

echo "=== Scheduled Task Examples ==="

if command -v "$CLI_TOOL" >/dev/null 2>&1; then
    future_time=$(date -d "+30 seconds" --iso-8601=seconds)
    echo "Submitting scheduled task for: $future_time"
    
    $CLI_TOOL task submit "test_task" '{"message":"Scheduled task","sleep_duration":1}' \
        --priority="medium" \
        --scheduled-at="$future_time" \
        --api-url="$API_URL"
    echo ""
fi

echo "=== Failed Task Examples (for testing retry logic) ==="

submit_task "unknown_task_type" '{"test":"This should fail"}' "medium" "Task with unknown type (will fail)"

echo "All test tasks submitted successfully!"
echo ""
echo "Use the following commands to monitor the tasks:"
echo "  $CLI_TOOL task list --api-url=$API_URL"
echo "  $CLI_TOOL queue stats --api-url=$API_URL"
echo "  $CLI_TOOL worker list --api-url=$API_URL"
echo "  $CLI_TOOL system health --api-url=$API_URL"
echo ""
echo "To check the status of a specific task:"
echo "  $CLI_TOOL task status <TASK_ID> --api-url=$API_URL" 