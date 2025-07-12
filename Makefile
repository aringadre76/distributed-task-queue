# Distributed Task Queue - Makefile

.PHONY: help setup build test test-comprehensive test-load test-phase4 clean dev-up dev-down deploy status

# Default target
help:
	@echo "Available commands:"
	@echo "  setup              - Setup development environment"
	@echo "  build              - Build all services"
	@echo "  test               - Run Go unit tests"
	@echo "  test-comprehensive - Run comprehensive test suite (all phases)"
	@echo "  test-load          - Run load testing (performance validation)"
	@echo "  test-phase4        - Run Phase 4 production features test"
	@echo "  clean              - Clean build artifacts"
	@echo "  dev-up             - Start local development environment"
	@echo "  dev-down           - Stop local development environment"
	@echo "  deploy             - Deploy to Kubernetes"
	@echo "  status             - Check system status"

# Setup development environment
setup:
	@echo "Setting up development environment..."
	go mod download
	go mod tidy
	chmod +x scripts/*.sh

# Build all services
build:
	@echo "Building all services..."
	go build -o bin/api-server ./cmd/api-server
	go build -o bin/worker ./cmd/worker
	go build -o bin/queue-manager ./cmd/queue-manager
	go build -o bin/cli ./cmd/cli

# Run Go unit tests
test:
	@echo "Running Go unit tests..."
	go test ./...
	go test -race ./...
	go test -cover ./...

# Run comprehensive test suite
test-comprehensive:
	@echo "Running comprehensive test suite..."
	@echo "This will test all phases of the distributed task queue system"
	@echo "Make sure the API server is running: ./bin/api-server &"
	@echo
	./scripts/test-comprehensive.sh

# Run load testing
test-load:
	@echo "Running load testing..."
	@echo "This will validate the 1000+ tasks/second throughput goal"
	@echo "Make sure the API server is running: ./bin/api-server &"
	@echo
	@echo "Usage: make test-load [concurrent_users] [requests_per_user] [duration]"
	@echo "Default: 50 concurrent users, 100 requests each, 60 seconds"
	@echo
	./scripts/load-test.sh ${ARGS}

# Run Phase 4 production features test
test-phase4:
	@echo "Running Phase 4 production features test..."
	@echo "Testing: JWT Auth, Rate Limiting, Tracing, Audit Logging"
	@echo "Make sure the API server is running: ./bin/api-server &"
	@echo
	./scripts/test-phase4.sh

# Check system status
status:
	@echo "Checking system status..."
	@echo "API Server:"
	@if pgrep -f api-server > /dev/null; then echo "  ✅ Running"; else echo "  ❌ Not running"; fi
	@echo "Docker Services:"
	@docker-compose ps --format "table {{.Name}}\t{{.Status}}\t{{.Ports}}"
	@echo
	@echo "Port Usage:"
	@ss -tulpn | grep -E ":(8080|9090|9091)" | head -5

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	rm -rf dist/
	go clean

# Start local development environment
dev-up:
	@echo "Starting local development environment..."
	docker-compose up -d

# Stop local development environment
dev-down:
	@echo "Stopping local development environment..."
	docker-compose down

# Deploy to Kubernetes
deploy:
	@echo "Deploying to Kubernetes..."
	kubectl apply -f deployments/kubernetes/ 