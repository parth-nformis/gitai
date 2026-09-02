#!/bin/bash

# Exit on any error
set -e

if [ "$GITAI_UPDATE" = "true" ]; then
    echo "Updating GitAI..."
else
    echo "Installing GitAI..."
fi

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "Error: Go is not installed. Please install Go (https://go.dev/)."
    exit 1
fi

# Create a temporary building directory
TEMP_BUILD_DIR=$(mktemp -d)

# Clone the repository to the temp directory silently
git clone -q --depth 1 https://github.com/parthdande/gitai.git "$TEMP_BUILD_DIR" 2>/dev/null

# Build the binary inside the temp directory. Build the whole ./cmd
# package rather than main.go alone: cmd/ is multi-file, and a
# file-mode build compiles only that one file, leaving every function
# in a sibling file undefined so the build fails.
#
# Inject the release version from the repo's `version` file so the
# installed binary reports the version that was just cloned.
(
    cd "$TEMP_BUILD_DIR"
    VERSION="$(cat version 2>/dev/null || echo 0.1.2)"
    go build -ldflags "-X main.Version=$VERSION" -o gitai ./cmd
)

# Install destination: per-user, so no sudo is needed and `gitai
# -update` can swap the binary in place without a password prompt.
DEST_DIR="$HOME/.local/bin"
mkdir -p "$DEST_DIR"

# Create config directory and default config file
CONFIG_DIR="$HOME/.gitai"
mkdir -p "$CONFIG_DIR"

if [ ! -f "$CONFIG_DIR/gitai.json" ]; then
    cat > "$CONFIG_DIR/gitai.json" << 'EOF'
{
  "api_base": "",
  "api_key": "",
  "model": ""
}
EOF
    echo " Created config file at $CONFIG_DIR/gitai.json"
fi

if mv "$TEMP_BUILD_DIR/gitai" "$DEST_DIR/gitai"; then
    # Clean up the temp directory
    rm -rf "$TEMP_BUILD_DIR"
    echo "--------------------------------------------------------"
    if [ "$GITAI_UPDATE" = "true" ]; then
        echo " GitAI updated successfully!"
    else
        echo " GitAI installed successfully!"
    fi
    echo "--------------------------------------------------------"
    if [ "$GITAI_UPDATE" != "true" ]; then
        echo "Next steps:"
        echo "  1. Add your API key to $CONFIG_DIR/gitai.json"
        echo "     (or set GEMINI_API_KEY environment variable)"
        echo "  2. Run 'gitai -commitmsg' or 'gitai -commit' in any Git repo"
        echo "--------------------------------------------------------"
    fi
else
    # Clean up the temp directory
    rm -rf "$TEMP_BUILD_DIR"
    echo "Failed to install binary to $DEST_DIR. Check that you can write to it."
    exit 1
fi

