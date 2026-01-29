#!/usr/bin/env bash
# Common functions for fixture generation scripts
#
# Required environment variables:
#   DASH0_API_URL   - The Dash0 API base URL (e.g., https://api.eu-west-1.aws.dash0.com)
#   DASH0_AUTH_TOKEN - The authentication token (must start with 'auth_')
#
# Optional environment variables:
#   DASH0_DATASET   - The dataset to use (default: 'default')

set -euo pipefail

# Validate required environment variables
validate_env() {
    if [[ -z "${DASH0_API_URL:-}" ]]; then
        echo "Error: DASH0_API_URL environment variable is required" >&2
        exit 1
    fi

    if [[ -z "${DASH0_AUTH_TOKEN:-}" ]]; then
        echo "Error: DASH0_AUTH_TOKEN environment variable is required" >&2
        exit 1
    fi

    if [[ ! "${DASH0_AUTH_TOKEN}" =~ ^auth_ ]]; then
        echo "Error: DASH0_AUTH_TOKEN must start with 'auth_'" >&2
        exit 1
    fi
}

# Get the fixtures directory (parent of scripts directory)
get_fixtures_dir() {
    local script_dir
    script_dir="$(cd "$(dirname "${BASH_SOURCE[1]}")" && pwd)"
    echo "$(dirname "$script_dir")"
}

# Make a GET request to the Dash0 API
# Usage: api_get <path> [query_params]
api_get() {
    local path="$1"
    local query="${2:-}"
    local url="${DASH0_API_URL}${path}"

    if [[ -n "$query" ]]; then
        url="${url}?${query}"
    fi

    curl -s -w "\n%{http_code}" \
        -H "Authorization: Bearer ${DASH0_AUTH_TOKEN}" \
        -H "Accept: application/json" \
        "$url"
}

# Make a GET request and save the response to a file
# Usage: api_get_to_file <path> <output_file> [query_params]
api_get_to_file() {
    local path="$1"
    local output_file="$2"
    local query="${3:-}"

    local response
    response=$(api_get "$path" "$query")

    local http_code
    http_code=$(echo "$response" | tail -n1)
    local body
    body=$(echo "$response" | sed '$d')

    if [[ "$http_code" -ge 200 && "$http_code" -lt 300 ]]; then
        echo "$body" | jq '.' > "$output_file"
        echo "Saved: $output_file (HTTP $http_code)"
    else
        echo "Error: HTTP $http_code for $path" >&2
        echo "$body" >&2
        return 1
    fi
}

# Make a GET request expecting an error and save the response
# Usage: api_get_error_to_file <path> <output_file> <expected_status> [query_params]
api_get_error_to_file() {
    local path="$1"
    local output_file="$2"
    local expected_status="$3"
    local query="${4:-}"

    local response
    response=$(api_get "$path" "$query") || true

    local http_code
    http_code=$(echo "$response" | tail -n1)
    local body
    body=$(echo "$response" | sed '$d')

    if [[ "$http_code" == "$expected_status" ]]; then
        echo "$body" | jq '.' > "$output_file"
        echo "Saved: $output_file (HTTP $http_code - expected)"
    else
        echo "Warning: Expected HTTP $expected_status but got $http_code for $path" >&2
        echo "$body" | jq '.' > "$output_file"
        echo "Saved anyway: $output_file (HTTP $http_code)"
    fi
}

# Get the first item ID from a list response
# Usage: get_first_id <json_array>
get_first_id() {
    local json="$1"
    echo "$json" | jq -r '.[0].id // empty'
}
