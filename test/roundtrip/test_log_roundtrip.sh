#!/usr/bin/env bash
set -euo pipefail

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
UNIQUE_ID="manual-test-$(date +%s)-$$"

echo "=== Log round-trip test ==="
echo "Unique ID: $UNIQUE_ID"

# Step 1: Send a log record with a unique attribute
echo "--- Step 1: Send log record ---"
SEND_OUTPUT=$("$DASH0" logs send "Log round-trip test: ${UNIQUE_ID}" \
  --severity-text INFO --severity-number 9 \
  --resource-attribute service.name=roundtrip-test \
  --log-attribute test.id="${UNIQUE_ID}")
echo "$SEND_OUTPUT"
if ! echo "$SEND_OUTPUT" | grep -q "Log record sent"; then
  echo "FAIL: logs send did not succeed"
  exit 1
fi

# Step 2: Wait for ingestion with retry and backoff
echo "--- Step 2: Waiting for ingestion ---"
MAX_ATTEMPTS=6
DELAY=5
for attempt in $(seq 1 "$MAX_ATTEMPTS"); do
  TABLE_OUTPUT=$("$DASH0" logs query --from now-5m --filter "test.id is ${UNIQUE_ID}" 2>/dev/null) || true
  if echo "$TABLE_OUTPUT" | grep -q "Log round-trip test"; then
    echo "Log record found after attempt $attempt"
    break
  fi
  if [ "$attempt" -eq "$MAX_ATTEMPTS" ]; then
    echo "FAIL: log record not found after $MAX_ATTEMPTS attempts"
    echo "$TABLE_OUTPUT"
    exit 1
  fi
  echo "Attempt $attempt/$MAX_ATTEMPTS: not yet ingested, retrying in ${DELAY}s..."
  sleep "$DELAY"
  DELAY=$((DELAY * 2))
done

# Step 3: Verify table output
echo "--- Step 3: Verify logs (table) ---"
echo "$TABLE_OUTPUT"
if ! echo "$TABLE_OUTPUT" | grep -q "INFO"; then
  echo "FAIL: severity INFO not found in table output"
  exit 1
fi

# Step 4: Query in CSV format
echo "--- Step 4: Query logs (csv) ---"
CSV_OUTPUT=$("$DASH0" logs query --from now-5m --filter "test.id is ${UNIQUE_ID}" -o csv)
echo "$CSV_OUTPUT"
if ! echo "$CSV_OUTPUT" | grep -q "otel.log.time"; then
  echo "FAIL: CSV header not found"
  exit 1
fi
if ! echo "$CSV_OUTPUT" | grep -q "Log round-trip test"; then
  echo "FAIL: log record not found in CSV output"
  exit 1
fi

# Step 5: Query in JSON format
echo "--- Step 5: Query logs (json) ---"
JSON_OUTPUT=$("$DASH0" logs query --from now-5m --filter "test.id is ${UNIQUE_ID}" -o json)
if ! echo "$JSON_OUTPUT" | jq -e '.resourceLogs' > /dev/null 2>&1; then
  echo "FAIL: JSON output is not valid OTLP/JSON"
  exit 1
fi
echo "Valid OTLP/JSON output received"

echo "=== Log round-trip test PASSED ==="
