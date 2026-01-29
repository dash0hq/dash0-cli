#!/usr/bin/env bash
# Generate dashboards/error_unauthorized.json fixture
#
# This script makes a request with an invalid token to capture
# the 401 error response format.
#
# Required environment variables:
#   DASH0_API_URL - The Dash0 API base URL
#
# Note: This script intentionally uses an invalid token

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FIXTURES_DIR="$(dirname "$SCRIPT_DIR")"
OUTPUT_FILE="$FIXTURES_DIR/dashboards/error_unauthorized.json"

if [[ -z "${DASH0_API_URL:-}" ]]; then
    echo "Error: DASH0_API_URL environment variable is required" >&2
    exit 1
fi

mkdir -p "$(dirname "$OUTPUT_FILE")"

DATASET="${DASH0_DATASET:-default}"

# Use an invalid token to trigger a 401
INVALID_TOKEN="auth_invalid_token_for_testing"

response=$(curl -s -w "\n%{http_code}" \
    -H "Authorization: Bearer ${INVALID_TOKEN}" \
    -H "Accept: application/json" \
    "${DASH0_API_URL}/api/dashboards?dataset=$DATASET") || true

http_code=$(echo "$response" | tail -n1)
body=$(echo "$response" | sed '$d')

if [[ "$http_code" == "401" ]]; then
    echo "$body" | jq '.' > "$OUTPUT_FILE"
    echo "Saved: $OUTPUT_FILE (HTTP $http_code - expected)"
else
    echo "Warning: Expected HTTP 401 but got $http_code" >&2
    echo "$body" | jq '.' > "$OUTPUT_FILE"
    echo "Saved anyway: $OUTPUT_FILE (HTTP $http_code)"
fi
