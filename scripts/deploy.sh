#!/bin/bash

# Distributed Task Queue - Kubernetes Deployment Script

set -e

echo "🚀 Deploying Distributed Task Queue to Kubernetes..."

# Check kubectl
if ! command -v kubectl &> /dev/null; then
    echo "❌ kubectl is not installed. Please install kubectl"
    exit 1
fi

# Check cluster connection
if ! kubectl cluster-info &> /dev/null; then
    echo "❌ Cannot connect to Kubernetes cluster"
    exit 1
fi

echo "✅ Connected to Kubernetes cluster"

# Build Docker images (placeholder for now)
echo "🐳 Building Docker images..."
# TODO: Add Docker build commands when Dockerfiles are created

# Apply Kubernetes manifests
echo "📋 Applying Kubernetes manifests..."
kubectl apply -f deployments/kubernetes/

# Wait for deployments
echo "⏳ Waiting for deployments to be ready..."
kubectl wait --for=condition=available --timeout=300s deployment --all -n default

echo "✅ Deployment complete!"
echo ""
echo "Check deployment status:"
echo "  kubectl get pods"
echo "  kubectl get services" 