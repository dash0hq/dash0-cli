#!/usr/bin/env bash
# Generate dashboards/get_success.json fixture
#
# This script fetches a single dashboard from the Dash0 API
# and saves it as a fixture file.
#
# Required environment variables:
#   DASH0_API_URL    - The Dash0 API base URL
#   DASH0_AUTH_TOKEN - The authentication token
#
# Optional environment variables:
#   DASH0_DASHBOARD_ID - Specific dashboard ID to fetch (default: first from list)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

validate_env

FIXTURES_DIR="$(get_fixtures_dir)"
OUTPUT_FILE="$FIXTURES_DIR/dashboards/get_success.json"

mkdir -p "$(dirname "$OUTPUT_FILE")"

DATASET="${DASH0_DATASET:-default}"
DASHBOARD_ID="${DASH0_DASHBOARD_ID:-}"

# If no dashboard ID provided, get the first one from the list
if [[ -z "$DASHBOARD_ID" ]]; then
    echo "No DASH0_DASHBOARD_ID provided, fetching first dashboard from list..."
    response=$(api_get "/api/dashboards" "dataset=$DATASET")
    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | sed '$d')

    if [[ "$http_code" -ne 200 ]]; then
        echo "Error: Failed to list dashboards (HTTP $http_code)" >&2
        exit 1
    fi

    DASHBOARD_ID=$(get_first_id "$body")

    if [[ -z "$DASHBOARD_ID" ]]; then
        echo "Error: No dashboards found to fetch" >&2
        exit 1
    fi

    echo "Using dashboard ID: $DASHBOARD_ID"
fi

api_get_to_file "/api/dashboards/$DASHBOARD_ID" "$OUTPUT_FILE" "dataset=$DATASET"
