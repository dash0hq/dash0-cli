#!/usr/bin/env bash
# Generate syntheticchecks/get_success.json fixture
#
# This script fetches a single synthetic check from the Dash0 API
# and saves it as a fixture file.
#
# Required environment variables:
#   DASH0_API_URL    - The Dash0 API base URL
#   DASH0_AUTH_TOKEN - The authentication token
#
# Optional environment variables:
#   DASH0_SYNTHETICCHECK_ID - Specific synthetic check ID to fetch (default: first from list)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

validate_env

FIXTURES_DIR="$(get_fixtures_dir)"
OUTPUT_FILE="$FIXTURES_DIR/syntheticchecks/get_success.json"

mkdir -p "$(dirname "$OUTPUT_FILE")"

DATASET="${DASH0_DATASET:-default}"
SYNTHETICCHECK_ID="${DASH0_SYNTHETICCHECK_ID:-}"

# If no synthetic check ID provided, get the first one from the list
if [[ -z "$SYNTHETICCHECK_ID" ]]; then
    echo "No DASH0_SYNTHETICCHECK_ID provided, fetching first synthetic check from list..."
    response=$(api_get "/api/synthetic-checks" "dataset=$DATASET")
    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | sed '$d')

    if [[ "$http_code" -ne 200 ]]; then
        echo "Error: Failed to list synthetic checks (HTTP $http_code)" >&2
        exit 1
    fi

    SYNTHETICCHECK_ID=$(get_first_id "$body")

    if [[ -z "$SYNTHETICCHECK_ID" ]]; then
        echo "Error: No synthetic checks found to fetch" >&2
        exit 1
    fi

    echo "Using synthetic check ID: $SYNTHETICCHECK_ID"
fi

api_get_to_file "/api/synthetic-checks/$SYNTHETICCHECK_ID" "$OUTPUT_FILE" "dataset=$DATASET"
