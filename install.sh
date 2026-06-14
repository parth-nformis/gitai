#!/bin/bash

# Exit on any error
set -e

echo "Installing GitAI..."

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "Error: Go is not installed. Please install Go (https://go.dev/)."
    exit 1
fi

# Create a temporary building directory
TEMP_BUILD_DIR=$(mktemp -d)

# Clone the repository to the temp directory silently
git clone -q --depth 1 https://github.com/parthdande/gitai.git "$TEMP_BUILD_DIR" 2>/dev/null

# Build the binary inside the temp directory
(
    cd "$TEMP_BUILD_DIR"
    go build -o gitai cmd/main.go
)

# Install destination
DEST_DIR="/usr/local/bin"

if sudo mv "$TEMP_BUILD_DIR/gitai" "$DEST_DIR/gitai"; then
    # Clean up the temp directory
    rm -rf "$TEMP_BUILD_DIR"
    echo "--------------------------------------------------------"
    echo " GitAI installed successfully!"
    echo "--------------------------------------------------------"
    echo "Usage:"
    echo "  1. Set your API Key: export GEMINI_API_KEY=\"your-key\""
    echo "  2. Run 'gitai -commitmsg' or 'gitai -commit' in any Git repo"
    echo "--------------------------------------------------------"
else
    # Clean up the temp directory
    rm -rf "$TEMP_BUILD_DIR"
    echo "Failed to install binary to $DEST_DIR. Make sure you have sudo privileges."
    exit 1
fi

