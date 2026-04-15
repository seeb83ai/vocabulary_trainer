#!/bin/bash
set -euo pipefail

# Only run in Claude Code remote (web) sessions
if [ "${CLAUDE_CODE_REMOTE:-}" != "true" ]; then
  exit 0
fi

echo "Setting up vocabulary_trainer environment..."

# Download Go module dependencies
echo "Downloading Go modules..."
cd "${CLAUDE_PROJECT_DIR}/service"
go mod download

# Install Node.js dependencies for frontend tests
echo "Installing npm dependencies..."
cd "${CLAUDE_PROJECT_DIR}"
npm install

echo "Environment setup complete."
