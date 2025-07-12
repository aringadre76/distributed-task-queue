#!/bin/bash

# Distributed Task Queue - Development Environment Setup

set -e

echo "🚀 Setting up Distributed Task Queue development environment..."

# Check prerequisites
echo "📋 Checking prerequisites..."

# Check Go
if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed. Please install Go 1.21+"
    exit 1
fi

GO_VERSION=$(go version | cut -d' ' -f3 | sed 's/go//')
if [ "$(printf '%s\n' "1.21" "$GO_VERSION" | sort -V | head -n1)" != "1.21" ]; then
    echo "❌ Go version $GO_VERSION is too old. Please install Go 1.21+"
    exit 1
fi
echo "✅ Go $GO_VERSION"

# Check Docker
if ! command -v docker &> /dev/null; then
    echo "❌ Docker is not installed. Please install Docker"
    exit 1
fi
echo "✅ Docker $(docker --version | cut -d' ' -f3 | sed 's/,//')"

# Check Docker Compose
if ! command -v docker-compose &> /dev/null; then
    echo "❌ Docker Compose is not installed. Please install Docker Compose"
    exit 1
fi
echo "✅ Docker Compose $(docker-compose --version | cut -d' ' -f4 | sed 's/,//')"

# Initialize Go module
echo "📦 Initializing Go module..."
go mod download
go mod tidy

# Create necessary directories
echo "📁 Creating directories..."
mkdir -p bin logs tmp

# Make scripts executable
echo "🔧 Setting up scripts..."
chmod +x scripts/*.sh

# Initialize database schema (when we create it)
# echo "🗄️  Initializing database schema..."
# TODO: Add database migration scripts

echo "✅ Development environment setup complete!"
echo ""
echo "Next steps:"
echo "  1. Start services: make dev-up"
echo "  2. Build project: make build"
echo "  3. Run tests: make test"
echo ""
echo "Services will be available at:"
echo "  - PostgreSQL: localhost:5432"
echo "  - Redis: localhost:6379"
echo "  - RabbitMQ: localhost:5672 (Management: localhost:15672)"
echo "  - Prometheus: localhost:9090"
echo "  - Grafana: localhost:3000 (admin/admin)" 