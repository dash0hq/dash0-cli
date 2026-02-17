#!/usr/bin/env bash
# Downloads the Dash0 CLI binary to ~/.dash0/bin/ if it is not already present.
#
# Environment variables:
#   CLI_VERSION - The version to download (required, e.g., "1.2.0").

set -euo pipefail

INSTALL_DIR="$HOME/.dash0/bin"

if [ -x "$INSTALL_DIR/dash0" ]; then
  echo "Dash0 CLI binary already present (from cache)."
  exit 0
fi

# --- Detect platform -------------------------------------------------------------------
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)         ARCH="amd64" ;;
  aarch64|arm64)  ARCH="arm64" ;;
  *)
    echo "::error::Unsupported architecture: $ARCH. Only amd64 and arm64 are supported."
    exit 1
    ;;
esac

if [ "$OS" != "linux" ]; then
  echo "::error::Unsupported OS: $OS. Only Linux is supported."
  exit 1
fi

# --- Download --------------------------------------------------------------------------
VERSION="${CLI_VERSION:?CLI_VERSION is required}"
BINARY_URL="https://github.com/dash0hq/dash0-cli/releases/download/v${VERSION}/dash0_${VERSION}_${OS}_${ARCH}.tar.gz"
echo "Installing Dash0 CLI ${VERSION} from ${BINARY_URL}..."
curl --proto '=https' --tlsv1.2 -LsSf "$BINARY_URL" | tar -xzf - -C "$INSTALL_DIR" dash0
