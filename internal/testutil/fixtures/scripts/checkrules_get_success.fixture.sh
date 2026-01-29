#!/usr/bin/env bash
# Generate checkrules/get_success.json fixture
#
# This script fetches a single check rule from the Dash0 API
# and saves it as a fixture file.
#
# Required environment variables:
#   DASH0_API_URL    - The Dash0 API base URL
#   DASH0_AUTH_TOKEN - The authentication token
#
# Optional environment variables:
#   DASH0_CHECKRULE_ID - Specific check rule ID to fetch (default: first from list)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

validate_env

FIXTURES_DIR="$(get_fixtures_dir)"
OUTPUT_FILE="$FIXTURES_DIR/checkrules/get_success.json"

mkdir -p "$(dirname "$OUTPUT_FILE")"

DATASET="${DASH0_DATASET:-default}"
CHECKRULE_ID="${DASH0_CHECKRULE_ID:-}"

# If no check rule ID provided, get the first one from the list
if [[ -z "$CHECKRULE_ID" ]]; then
    echo "No DASH0_CHECKRULE_ID provided, fetching first check rule from list..."
    response=$(api_get "/api/alerting/check-rules" "dataset=$DATASET")
    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | sed '$d')

    if [[ "$http_code" -ne 200 ]]; then
        echo "Error: Failed to list check rules (HTTP $http_code)" >&2
        exit 1
    fi

    CHECKRULE_ID=$(get_first_id "$body")

    if [[ -z "$CHECKRULE_ID" ]]; then
        echo "Error: No check rules found to fetch" >&2
        exit 1
    fi

    echo "Using check rule ID: $CHECKRULE_ID"
fi

api_get_to_file "/api/alerting/check-rules/$CHECKRULE_ID" "$OUTPUT_FILE" "dataset=$DATASET"
