#!/usr/bin/env bash
# Generate views/list_empty.json fixture
#
# This script creates an empty array fixture for testing
# the case when no views exist.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FIXTURES_DIR="$(dirname "$SCRIPT_DIR")"
OUTPUT_FILE="$FIXTURES_DIR/views/list_empty.json"

mkdir -p "$(dirname "$OUTPUT_FILE")"

echo '[]' > "$OUTPUT_FILE"

echo "Saved: $OUTPUT_FILE (static empty array)"
