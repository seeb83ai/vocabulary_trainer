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

# Install Node.js dependencies (including Playwright test runner)
echo "Installing npm dependencies..."
cd "${CLAUDE_PROJECT_DIR}"
# PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD: environment pre-installs Chromium at
# /opt/pw-browsers (set via PLAYWRIGHT_BROWSERS_PATH). No download needed.
PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 npm install

echo "Environment setup complete."
