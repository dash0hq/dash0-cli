#!/usr/bin/env bash
# Generate views/get_success.json fixture
#
# This script fetches a single view from the Dash0 API
# and saves it as a fixture file.
#
# Required environment variables:
#   DASH0_API_URL    - The Dash0 API base URL
#   DASH0_AUTH_TOKEN - The authentication token
#
# Optional environment variables:
#   DASH0_VIEW_ID - Specific view ID to fetch (default: first from list)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

validate_env

FIXTURES_DIR="$(get_fixtures_dir)"
OUTPUT_FILE="$FIXTURES_DIR/views/get_success.json"

mkdir -p "$(dirname "$OUTPUT_FILE")"

DATASET="${DASH0_DATASET:-default}"
VIEW_ID="${DASH0_VIEW_ID:-}"

# If no view ID provided, get the first one from the list
if [[ -z "$VIEW_ID" ]]; then
    echo "No DASH0_VIEW_ID provided, fetching first view from list..."
    response=$(api_get "/api/views" "dataset=$DATASET")
    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | sed '$d')

    if [[ "$http_code" -ne 200 ]]; then
        echo "Error: Failed to list views (HTTP $http_code)" >&2
        exit 1
    fi

    VIEW_ID=$(get_first_id "$body")

    if [[ -z "$VIEW_ID" ]]; then
        echo "Error: No views found to fetch" >&2
        exit 1
    fi

    echo "Using view ID: $VIEW_ID"
fi

api_get_to_file "/api/views/$VIEW_ID" "$OUTPUT_FILE" "dataset=$DATASET"
