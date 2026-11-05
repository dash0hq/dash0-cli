#!/usr/bin/env bash
# Resolves the Dash0 CLI version to install and prepares the install directory.
#
# When SKIP_IF_ON_PATH is "true" and `dash0` is already on PATH, the script
# verifies it meets the minimum version requirement and sets skip-install=true.
#
# Environment variables:
#   CLI_VERSION        - Desired version (empty = latest). A leading "v" is stripped.
#   SKIP_IF_ON_PATH    - If "true", skip installation when `dash0` is already on PATH.
#   MIN_VERSION        - Minimum acceptable CLI version (default: "1.1.0"). Callers that depend
#                         on a feature introduced after 1.1.0 should set this explicitly.
#
# Outputs (via GITHUB_OUTPUT):
#   version        - Resolved semver string (e.g., "1.2.0")
#   skip-install   - "true" when the CLI is already on PATH and meets the minimum version

set -euo pipefail

MIN_SUPPORTED="${MIN_VERSION:-1.1.0}"
INSTALL_DIR="$HOME/.dash0/bin"

# --- Check for existing installation ---------------------------------------------------
if [ "${SKIP_IF_ON_PATH:-false}" = "true" ] && command -v dash0 &>/dev/null; then
  EXISTING_VERSION=$(dash0 version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1)
  if [ -z "$EXISTING_VERSION" ]; then
    echo "::error::Could not determine installed Dash0 CLI version."
    exit 1
  fi
  if ! printf '%s\n%s\n' "$MIN_SUPPORTED" "$EXISTING_VERSION" | sort -V -C; then
    echo "::error::Installed Dash0 CLI version $EXISTING_VERSION is below the minimum supported version ($MIN_SUPPORTED)."
    exit 1
  fi
  echo "Dash0 CLI $EXISTING_VERSION already on PATH."
  echo "version=$EXISTING_VERSION" >> "$GITHUB_OUTPUT"
  echo "skip-install=true" >> "$GITHUB_OUTPUT"
  exit 0
fi

echo "skip-install=false" >> "$GITHUB_OUTPUT"

# --- Resolve version -------------------------------------------------------------------
VERSION="${CLI_VERSION:-}"
if [ -z "$VERSION" ]; then
  echo "Fetching latest Dash0 CLI version..."
  # Resolve the latest PUBLISHED release, not the newest tag. The release tag is
  # pushed before GoReleaser uploads the assets, so between the two the newest tag
  # names a version whose download URL 404s. `/releases/latest` only ever points at
  # a non-draft, non-prerelease release, so its asset download is always available.
  # The redirect is followed without a token; the repo is public.
  LATEST_RELEASE_URL=$(curl --proto '=https' --tlsv1.2 -LsS -o /dev/null -w '%{url_effective}' \
    https://github.com/dash0hq/dash0-cli/releases/latest) || true
  VERSION="${LATEST_RELEASE_URL##*/releases/tag/v}"
  if [ -z "$VERSION" ] || [ "$VERSION" = "$LATEST_RELEASE_URL" ]; then
    echo "::error::Could not determine latest Dash0 CLI version"
    exit 1
  fi
  echo "Latest version: $VERSION"
else
  VERSION="${VERSION#v}"
  echo "Using specified version: $VERSION"
fi

# --- Enforce minimum version -----------------------------------------------------------
if ! printf '%s\n%s\n' "$MIN_SUPPORTED" "$VERSION" | sort -V -C; then
  echo "::error::Dash0 CLI version $VERSION is below the minimum supported version ($MIN_SUPPORTED)."
  exit 1
fi

echo "version=$VERSION" >> "$GITHUB_OUTPUT"

# --- Prepare install directory ---------------------------------------------------------
mkdir -p "$INSTALL_DIR"
