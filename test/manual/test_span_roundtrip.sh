#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
UNIQUE_ID="manual-test-$(date +%s)-$$"

echo "=== Span round-trip test ==="
echo "Unique ID: $UNIQUE_ID"

# Step 1: Send a span with a unique attribute
echo "--- Step 1: Send span ---"
SEND_OUTPUT=$("$DASH0" -X spans send \
  --name "roundtrip-test-span" \
  --kind SERVER --status-code OK --duration 100ms \
  --resource-attribute service.name=roundtrip-test \
  --span-attribute test.id="${UNIQUE_ID}")
echo "$SEND_OUTPUT"
if ! echo "$SEND_OUTPUT" | grep -q "Span sent successfully"; then
  echo "FAIL: spans send did not succeed"
  exit 1
fi

# Extract trace-id and span-id from the output
TRACE_ID=$(echo "$SEND_OUTPUT" | grep -oE 'trace-id: [0-9a-f]+' | cut -d' ' -f2)
SPAN_ID=$(echo "$SEND_OUTPUT" | grep -oE 'span-id: [0-9a-f]+' | cut -d' ' -f2)
echo "Trace ID: $TRACE_ID"
echo "Span ID:  $SPAN_ID"
if [ -z "$TRACE_ID" ] || [ -z "$SPAN_ID" ]; then
  echo "FAIL: could not extract trace-id or span-id from send output"
  exit 1
fi

# Step 2: Wait for ingestion
echo "--- Step 2: Waiting 10s for ingestion ---"
sleep 10

# Step 3: Query in table format and verify the span appears
echo "--- Step 3: Query spans (table) ---"
TABLE_OUTPUT=$("$DASH0" -X spans query --from now-5m --filter "test.id is ${UNIQUE_ID}")
echo "$TABLE_OUTPUT"
if ! echo "$TABLE_OUTPUT" | grep -q "roundtrip-test-span"; then
  echo "FAIL: span not found in table output"
  exit 1
fi
if ! echo "$TABLE_OUTPUT" | grep -q "$TRACE_ID"; then
  echo "FAIL: trace ID not found in table output"
  exit 1
fi

# Step 4: Query in CSV format
echo "--- Step 4: Query spans (csv) ---"
CSV_OUTPUT=$("$DASH0" -X spans query --from now-5m --filter "test.id is ${UNIQUE_ID}" -o csv)
echo "$CSV_OUTPUT"
if ! echo "$CSV_OUTPUT" | grep -q "otel.span.start_time"; then
  echo "FAIL: CSV header not found"
  exit 1
fi
if ! echo "$CSV_OUTPUT" | grep -q "roundtrip-test-span"; then
  echo "FAIL: span not found in CSV output"
  exit 1
fi

# Step 5: Query in JSON format
echo "--- Step 5: Query spans (json) ---"
JSON_OUTPUT=$("$DASH0" -X spans query --from now-5m --filter "test.id is ${UNIQUE_ID}" -o json)
if ! echo "$JSON_OUTPUT" | jq -e '.resourceSpans' > /dev/null 2>&1; then
  echo "FAIL: JSON output is not valid OTLP/JSON"
  exit 1
fi
echo "Valid OTLP/JSON output received"

# Step 6: Retrieve via traces get
echo "--- Step 6: Retrieve trace ---"
TRACE_OUTPUT=$("$DASH0" -X traces get "$TRACE_ID" --from now-5m)
echo "$TRACE_OUTPUT"
if ! echo "$TRACE_OUTPUT" | grep -q "roundtrip-test-span"; then
  echo "FAIL: span not found in traces get output"
  exit 1
fi
if ! echo "$TRACE_OUTPUT" | grep -q "$SPAN_ID"; then
  echo "FAIL: span ID not found in traces get output"
  exit 1
fi

echo "=== Span round-trip test PASSED ==="
