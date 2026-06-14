#!/bin/bash

# Exit on any error
set -e

echo "Installing GitAI..."

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "Error: Go is not installed. Please install Go (https://go.dev/) or download a pre-built binary."
    exit 1
fi

# Build the binary
echo "Compiling GitAI binary..."
go build -o gitai cmd/main.go

# Install destination
DEST_DIR="/usr/local/bin"

echo "Moving binary to $DEST_DIR (requires sudo)..."
if sudo mv gitai "$DEST_DIR/gitai"; then
    echo "--------------------------------------------------------"
    echo " GitAI installed successfully to $DEST_DIR/gitai!"
    echo "--------------------------------------------------------"
    echo "Usage:"
    echo "  1. Set your API Key: export GEMINI_API_KEY=\"your-key\""
    echo "  2. Run 'gitai -commitmsg' or 'gitai -commit' in any Git repo"
    echo "--------------------------------------------------------"
else
    echo "Failed to install binary to $DEST_DIR. Make sure you have sudo privileges."
    exit 1
fi
