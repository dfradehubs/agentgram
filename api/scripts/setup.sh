#!/bin/bash

# Setup script for agentgram backend development

set -e

echo "🚀 Setting up agentgram backend..."

# Check Go version (compatible with macOS)
GO_VERSION=$(go version 2>/dev/null | sed -E 's/.*go([0-9]+\.[0-9]+).*/\1/' || echo "not found")
echo "📦 Go version: $GO_VERSION"

if [[ "$GO_VERSION" == "not found" ]]; then
    echo "❌ Go is not installed"
    exit 1
fi

# Compare versions (simple check)
GO_MAJOR=$(echo "$GO_VERSION" | cut -d. -f1)
GO_MINOR=$(echo "$GO_VERSION" | cut -d. -f2)

if [[ "$GO_MAJOR" -lt 1 ]] || [[ "$GO_MAJOR" -eq 1 && "$GO_MINOR" -lt 22 ]]; then
    echo "❌ Go 1.22 or higher is required (found $GO_VERSION)"
    exit 1
fi

# Install dependencies
echo "📥 Installing dependencies..."
go mod download

# Install development tools
echo "🔧 Installing development tools..."

# Air for hot reload
if ! command -v air &> /dev/null; then
    echo "  Installing air..."
    go install github.com/air-verse/air@latest
fi

# golangci-lint for linting
if ! command -v golangci-lint &> /dev/null; then
    echo "  Installing golangci-lint..."
    go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
fi

# mockgen for generating mocks
if ! command -v mockgen &> /dev/null; then
    echo "  Installing mockgen..."
    go install go.uber.org/mock/mockgen@latest
fi

# Create .env if not exists
if [ ! -f .env ]; then
    echo "📝 Creating .env from .env.example..."
    cp .env.example .env
    echo "⚠️  Please edit .env with your actual values"
fi

# Create tmp directory for air
mkdir -p tmp

echo ""
echo "✅ Setup complete!"
echo ""
echo "Available commands:"
echo "  make dev          - Run in development mode"
echo "  make dev-watch    - Run with hot reload"
echo "  make test         - Run tests"
echo "  make lint         - Run linter"
echo "  make docker-up    - Start Docker environment"
echo ""
echo "Run 'make help' for more options"
