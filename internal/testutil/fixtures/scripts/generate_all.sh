#!/usr/bin/env bash
# Generate all fixture files from the Dash0 API
#
# This script dynamically discovers and runs all individual fixture generation scripts.
#
# Required environment variables:
#   DASH0_API_URL    - The Dash0 API base URL (e.g., https://api.eu-west-1.aws.dash0.com)
#   DASH0_AUTH_TOKEN - The authentication token (must start with 'auth_')
#
# Optional environment variables:
#   DASH0_DATASET    - The dataset to use (default: 'default')
#
# Usage:
#   export DASH0_API_URL='https://api.eu-west-1.aws.dash0.com'
#   export DASH0_AUTH_TOKEN='auth_your_token_here'
#   ./generate_all.sh
#
# To generate only specific resource types, run the individual scripts:
#   ./dashboards_list_success.fixture.sh
#   ./checkrules_get_success.fixture.sh
#   etc.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "=========================================="
echo "Dash0 CLI Fixture Generator"
echo "=========================================="
echo ""

# Validate environment
if [[ -z "${DASH0_API_URL:-}" ]]; then
    echo "Error: DASH0_API_URL environment variable is required" >&2
    echo "" >&2
    echo "Usage:" >&2
    echo "  export DASH0_API_URL='https://api.eu-west-1.aws.dash0.com'" >&2
    echo "  export DASH0_AUTH_TOKEN='auth_your_token_here'" >&2
    echo "  $0" >&2
    exit 1
fi

if [[ -z "${DASH0_AUTH_TOKEN:-}" ]]; then
    echo "Error: DASH0_AUTH_TOKEN environment variable is required" >&2
    exit 1
fi

echo "API URL: $DASH0_API_URL"
echo "Dataset: ${DASH0_DATASET:-default}"
echo ""

# Track successes and failures
declare -a successes=()
declare -a failures=()

run_script() {
    local script="$1"
    local name
    name=$(basename "$script" .fixture.sh)

    echo "----------------------------------------"
    echo "Running: $name"
    echo "----------------------------------------"

    if "$script"; then
        successes+=("$name")
    else
        failures+=("$name")
        echo "Warning: $name failed" >&2
    fi
    echo ""
}

# Discover all resource types by finding <resource>_*.fixture.sh scripts
# Script names follow pattern: <resource>_<operation>.fixture.sh
# where <resource> is: dashboards, checkrules, syntheticchecks, views
discover_resource_types() {
    find "$SCRIPT_DIR" -maxdepth 1 -name '*.fixture.sh' -type f \
        | xargs -n1 basename \
        | sed 's/_.*//' \
        | sort -u
}

# Get scripts for a specific resource type, sorted for consistent ordering
get_scripts_for_resource() {
    local resource="$1"
    find "$SCRIPT_DIR" -maxdepth 1 -name "${resource}_*.fixture.sh" -type f | sort
}

# Format resource type for display (e.g., "checkrules" -> "Check Rules")
format_resource_name() {
    local resource="$1"
    case "$resource" in
        checkrules) echo "Check Rules" ;;
        syntheticchecks) echo "Synthetic Checks" ;;
        dashboards) echo "Dashboards" ;;
        views) echo "Views" ;;
        *) echo "$resource" ;;
    esac
}

# Main: discover and run all scripts grouped by resource type
for resource in $(discover_resource_types); do
    display_name=$(format_resource_name "$resource")
    echo "=== $display_name ==="

    for script in $(get_scripts_for_resource "$resource"); do
        run_script "$script"
    done
done

# Summary
echo "=========================================="
echo "Summary"
echo "=========================================="
echo "Successful: ${#successes[@]}"
echo "Failed: ${#failures[@]}"

if [[ ${#failures[@]} -gt 0 ]]; then
    echo ""
    echo "Failed scripts:"
    for f in "${failures[@]}"; do
        echo "  - $f"
    done
    exit 1
fi

echo ""
echo "All fixtures generated successfully!"
