#!/usr/bin/env bash
# Generate syntheticchecks/error_not_found.json fixture
#
# This script fetches a non-existent synthetic check to capture
# the 404 error response format.
#
# Required environment variables:
#   DASH0_API_URL    - The Dash0 API base URL
#   DASH0_AUTH_TOKEN - The authentication token

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

validate_env

FIXTURES_DIR="$(get_fixtures_dir)"
OUTPUT_FILE="$FIXTURES_DIR/syntheticchecks/error_not_found.json"

mkdir -p "$(dirname "$OUTPUT_FILE")"

DATASET="${DASH0_DATASET:-default}"

# Use a non-existent UUID to trigger a 404
NONEXISTENT_ID="00000000-0000-0000-0000-000000000000"

api_get_error_to_file "/api/synthetic-checks/$NONEXISTENT_ID" "$OUTPUT_FILE" "404" "dataset=$DATASET"
