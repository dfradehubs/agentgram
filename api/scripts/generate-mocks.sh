#!/bin/bash

# Generate mocks for testing

set -e

echo "🔧 Generating mocks..."

# Check if mockgen is installed
if ! command -v mockgen &> /dev/null; then
    echo "Installing mockgen..."
    go install go.uber.org/mock/mockgen@latest
fi

# Generate mocks for interfaces
# Add more as needed

# Example:
# mockgen -source=internal/agents/registry.go -destination=internal/agents/mock_registry.go -package=agents

echo "✅ Mocks generated"
