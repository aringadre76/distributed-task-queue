#!/bin/bash

# Distributed Task Queue - Test Script

set -e

echo "🧪 Running Distributed Task Queue tests..."

# Unit tests
echo "📋 Running unit tests..."
go test ./... -v

# Race condition tests
echo "🏃 Running race condition tests..."
go test -race ./...

# Coverage tests
echo "📊 Running coverage tests..."
go test -cover ./... -coverprofile=coverage.out

# Generate coverage HTML report
if [ -f coverage.out ]; then
    go tool cover -html=coverage.out -o coverage.html
    echo "📈 Coverage report generated: coverage.html"
fi

# Integration tests (when implemented)
echo "🔗 Running integration tests..."
# TODO: Add integration test commands

# Load tests (when implemented)
echo "⚡ Running load tests..."
# TODO: Add load test commands

echo "✅ All tests completed!" 