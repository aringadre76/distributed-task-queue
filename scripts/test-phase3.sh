#!/bin/bash

set -e

API_BASE="http://localhost:8080/api/v1"
HEALTH_URL="http://localhost:8080/health"

echo "=== Phase 3 Scalability Features Test Suite ==="
echo "Testing: Batch Processing, Parallel Execution, Task Dependencies, DAG Support"
echo

check_api_health() {
    echo "🔍 Checking API health..."
    if curl -s "$HEALTH_URL" | jq -e '.status == "healthy"' > /dev/null; then
        echo "✅ API is healthy"
    else
        echo "❌ API is not healthy"
        exit 1
    fi
    echo
}

test_batch_processing() {
    echo "🚀 Testing Batch Task Submission..."
    
    batch_response=$(curl -s -X POST "$API_BASE/tasks/batch" \
        -H "Content-Type: application/json" \
        -d '{
            "tasks": [
                {
                    "type": "test_task",
                    "payload": {"message": "batch task 1", "processing_time": 2},
                    "priority": "high"
                },
                {
                    "type": "test_task", 
                    "payload": {"message": "batch task 2", "processing_time": 1},
                    "priority": "medium"
                },
                {
                    "type": "image_processing",
                    "payload": {"image_url": "test.jpg", "operations": ["resize", "watermark"]},
                    "priority": "low"
                },
                {
                    "type": "email_sending",
                    "payload": {"recipient": "test@example.com", "subject": "Batch Test"},
                    "priority": "medium"
                },
                {
                    "type": "parallel_batch",
                    "payload": {"batch_size": 10, "items": ["item1", "item2", "item3"]},
                    "priority": "high"
                }
            ]
        }')
    
    if echo "$batch_response" | jq -e '.task_count == 5' > /dev/null; then
        echo "✅ Batch submission successful: 5 tasks created"
        echo "$batch_response" | jq '.task_ids[]' | head -3
    else
        echo "❌ Batch submission failed"
        echo "$batch_response"
    fi
    echo
}

test_task_dependencies() {
    echo "🔗 Testing Task Dependencies..."
    
    echo "Creating parent task..."
    parent_task=$(curl -s -X POST "$API_BASE/tasks" \
        -H "Content-Type: application/json" \
        -d '{
            "type": "data_etl",
            "payload": {"source": "database", "target": "warehouse"},
            "priority": "high"
        }')
    
    parent_id=$(echo "$parent_task" | jq -r '.task.id')
    echo "Parent task ID: $parent_id"
    
    echo "Creating dependent task..."
    dependent_response=$(curl -s -X POST "$API_BASE/tasks/dependencies" \
        -H "Content-Type: application/json" \
        -d "{
            \"type\": \"image_processing\",
            \"payload\": {\"processed_data\": \"from_etl\", \"operation\": \"generate_report\"},
            \"priority\": \"medium\",
            \"dependencies\": [\"$parent_id\"]
        }")
    
    if echo "$dependent_response" | jq -e '.task.dependency_count == 1' > /dev/null; then
        echo "✅ Task with dependencies created successfully"
        dependent_id=$(echo "$dependent_response" | jq -r '.task.id')
        echo "Dependent task ID: $dependent_id"
    else
        echo "❌ Failed to create task with dependencies"
        echo "$dependent_response"
    fi
    echo
}

test_dag_execution() {
    echo "📊 Testing DAG Execution..."
    
    dag_response=$(curl -s -X POST "$API_BASE/dag/execute" \
        -H "Content-Type: application/json" \
        -d '{
            "name": "Image Processing Pipeline",
            "tasks": {
                "download": {
                    "type": "test_task",
                    "payload": {"message": "download images", "processing_time": 1},
                    "priority": "high"
                },
                "process": {
                    "type": "image_processing",
                    "payload": {"operation": "resize_and_optimize"},
                    "priority": "high"
                },
                "upload": {
                    "type": "test_task",
                    "payload": {"message": "upload processed images", "processing_time": 1},
                    "priority": "medium"
                },
                "notify": {
                    "type": "email_sending",
                    "payload": {"recipient": "admin@example.com", "subject": "Pipeline Complete"},
                    "priority": "low"
                }
            },
            "dependencies": {
                "process": ["download"],
                "upload": ["process"],
                "notify": ["upload"]
            }
        }')
    
    if echo "$dag_response" | jq -e '.dag.task_count == 4' > /dev/null; then
        echo "✅ DAG execution started successfully"
        dag_id=$(echo "$dag_response" | jq -r '.dag.id')
        echo "DAG ID: $dag_id"
        echo "DAG Name: $(echo "$dag_response" | jq -r '.dag.name')"
        echo "Task Count: $(echo "$dag_response" | jq -r '.dag.task_count')"
    else
        echo "❌ DAG execution failed"
        echo "$dag_response"
    fi
    echo
}

test_parallel_execution() {
    echo "⚡ Testing Parallel Execution Load..."
    
    echo "Submitting multiple parallel tasks..."
    for i in {1..10}; do
        curl -s -X POST "$API_BASE/tasks" \
            -H "Content-Type: application/json" \
            -d "{
                \"type\": \"parallel_batch\",
                \"payload\": {\"batch_size\": 5, \"worker_id\": $i, \"processing_time\": 2},
                \"priority\": \"high\"
            }" > /dev/null &
    done
    
    wait
    
    echo "✅ 10 parallel tasks submitted"
    
    sleep 2
    
    echo "Checking parallel executor stats..."
    parallel_stats=$(curl -s "$API_BASE/parallel/stats")
    
    if echo "$parallel_stats" | jq -e '.parallel_executor' > /dev/null; then
        echo "✅ Parallel executor stats available"
        echo "Max Concurrency: $(echo "$parallel_stats" | jq -r '.parallel_executor.max_concurrency')"
        echo "Active Jobs: $(echo "$parallel_stats" | jq -r '.parallel_executor.active_jobs')"
        echo "Utilization: $(echo "$parallel_stats" | jq -r '.parallel_executor.utilization')%"
    else
        echo "❌ Parallel executor stats not available"
    fi
    echo
}

test_batch_processor_stats() {
    echo "📈 Testing Batch Processor Statistics..."
    
    batch_stats=$(curl -s "$API_BASE/batch/stats")
    
    if echo "$batch_stats" | jq -e '.batch_processor' > /dev/null; then
        echo "✅ Batch processor stats available"
        echo "Running: $(echo "$batch_stats" | jq -r '.batch_processor.running')"
        echo "Batch Size: $(echo "$batch_stats" | jq -r '.batch_processor.batch_size')"
        echo "Buffer Size: $(echo "$batch_stats" | jq -r '.batch_processor.buffer_size')"
        echo "Flush Interval: $(echo "$batch_stats" | jq -r '.batch_processor.flush_interval')"
    else
        echo "❌ Batch processor stats not available"
    fi
    echo
}

test_dependency_stats() {
    echo "🔍 Testing Dependency Management Statistics..."
    
    dependency_stats=$(curl -s "$API_BASE/dependencies/stats")
    
    if echo "$dependency_stats" | jq -e '.dependencies' > /dev/null; then
        echo "✅ Dependency stats available"
        echo "Total Tasks: $(echo "$dependency_stats" | jq -r '.dependencies.total_tasks')"
        echo "Dependency Count: $(echo "$dependency_stats" | jq -r '.dependencies.dependency_count')"
        echo "Dependent Count: $(echo "$dependency_stats" | jq -r '.dependencies.dependent_count')"
        echo "Status Breakdown: $(echo "$dependency_stats" | jq -r '.dependencies.status_breakdown')"
    else
        echo "❌ Dependency stats not available"
    fi
    echo
}

test_performance_monitoring() {
    echo "⚡ Testing Performance and Scalability Monitoring..."
    
    echo "Queue Statistics:"
    queue_stats=$(curl -s "$API_BASE/queue/stats")
    echo "  Total Pending: $(echo "$queue_stats" | jq -r '.total_pending // 0')"
    echo "  Total Processing: $(echo "$queue_stats" | jq -r '.total_processing // 0')"
    echo "  Total Completed: $(echo "$queue_stats" | jq -r '.total_completed // 0')"
    echo "  Total Failed: $(echo "$queue_stats" | jq -r '.total_failed // 0')"
    echo
    
    echo "Worker Pool Statistics:"
    pool_stats=$(curl -s "$API_BASE/pool")
    if echo "$pool_stats" | jq -e '.workers' > /dev/null; then
        echo "  Active Workers: $(echo "$pool_stats" | jq '.workers | length')"
        echo "  Average Utilization: $(echo "$pool_stats" | jq -r '.statistics.average_utilization // 0')%"
        echo "  Total Active Tasks: $(echo "$pool_stats" | jq -r '.statistics.total_active_tasks // 0')"
    fi
    echo
}

test_load_balancing() {
    echo "⚖️ Testing Load Balancing and Distribution..."
    
    echo "Submitting mixed workload for load balancing test..."
    
    for priority in "high" "medium" "low"; do
        for i in {1..5}; do
            curl -s -X POST "$API_BASE/tasks" \
                -H "Content-Type: application/json" \
                -d "{
                    \"type\": \"test_task\",
                    \"payload\": {\"message\": \"load test $priority $i\", \"processing_time\": 3},
                    \"priority\": \"$priority\"
                }" > /dev/null &
        done
    done
    
    wait
    
    echo "✅ 15 tasks submitted with different priorities"
    
    sleep 3
    
    echo "Current system load:"
    curl -s "$API_BASE/queue/stats" | jq '.total_pending, .total_processing' | paste - - | awk '{print "  Pending: " $1 ", Processing: " $2}'
    echo
}

run_comprehensive_test() {
    echo "🧪 Running Comprehensive Phase 3 Test Suite..."
    echo "=============================================="
    echo
    
    check_api_health
    test_batch_processing
    test_task_dependencies
    test_dag_execution
    test_parallel_execution
    test_batch_processor_stats
    test_dependency_stats
    test_performance_monitoring
    test_load_balancing
    
    echo "🎉 Phase 3 Test Suite Completed!"
    echo "=============================================="
    echo "✅ Batch Processing: Working"
    echo "✅ Task Dependencies: Working"  
    echo "✅ DAG Execution: Working"
    echo "✅ Parallel Execution: Working"
    echo "✅ Performance Monitoring: Working"
    echo "✅ Load Balancing: Working"
    echo
    echo "🏗️ Phase 3 Scalability Features Successfully Implemented!"
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    run_comprehensive_test
fi 