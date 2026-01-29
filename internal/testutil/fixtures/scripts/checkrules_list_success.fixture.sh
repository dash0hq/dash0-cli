#!/usr/bin/env bash
# Generate checkrules/list_success.json fixture
#
# This script fetches the list of check rules from the Dash0 API
# and saves it as a fixture file.
#
# Required environment variables:
#   DASH0_API_URL    - The Dash0 API base URL
#   DASH0_AUTH_TOKEN - The authentication token

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

validate_env

FIXTURES_DIR="$(get_fixtures_dir)"
OUTPUT_FILE="$FIXTURES_DIR/checkrules/list_success.json"

mkdir -p "$(dirname "$OUTPUT_FILE")"

DATASET="${DASH0_DATASET:-default}"

api_get_to_file "/api/alerting/check-rules" "$OUTPUT_FILE" "dataset=$DATASET"
