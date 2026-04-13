#!/usr/bin/env bash
set -euo pipefail

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
UNIQUE_ID="metrics-roundtrip-$(date +%s)-$$"
SERVICE_NAME="metrics-roundtrip-${UNIQUE_ID}"

echo "=== Metrics instant round-trip test ==="
echo "Unique ID: $UNIQUE_ID"
echo "Service:   $SERVICE_NAME"

# Step 1: Send a log record so that the dash0.logs metric is created for our service.
echo "--- Step 1: Send log record ---"
SEND_OUTPUT=$("$DASH0" logs send "Metrics round-trip test: ${UNIQUE_ID}" \
  --severity-text INFO --severity-number 9 \
  --resource-attribute "service.name=${SERVICE_NAME}" \
  --log-attribute "test.id=${UNIQUE_ID}")
echo "$SEND_OUTPUT"
if ! echo "$SEND_OUTPUT" | grep -q "Log record sent"; then
  echo "FAIL: logs send did not succeed"
  exit 1
fi

# Step 2: Wait for ingestion and metric aggregation.
echo "--- Step 2: Waiting 60s for ingestion and metric aggregation ---"
sleep 60

# Step 3: Query with --promql, default table output (verbose format).
echo "--- Step 3: Query with --promql (table) ---"
TABLE_OUTPUT=$("$DASH0" metrics instant \
  --promql "{otel_metric_name=\"dash0.logs\",service_name=\"${SERVICE_NAME}\"}")
echo "$TABLE_OUTPUT"
if ! echo "$TABLE_OUTPUT" | grep -q "Query:"; then
  echo "FAIL: table output missing 'Query:' header"
  exit 1
fi
if ! echo "$TABLE_OUTPUT" | grep -q "Value:"; then
  echo "FAIL: table output missing 'Value:' line"
  exit 1
fi

# Step 4: Query with --promql, JSON output.
echo "--- Step 4: Query with --promql (json) ---"
JSON_OUTPUT=$("$DASH0" metrics instant \
  --promql "{otel_metric_name=\"dash0.logs\",service_name=\"${SERVICE_NAME}\"}" \
  -o json)
if ! echo "$JSON_OUTPUT" | jq -e '.status == "success"' > /dev/null 2>&1; then
  echo "FAIL: JSON output does not have status 'success'"
  echo "$JSON_OUTPUT"
  exit 1
fi
echo "Valid JSON output received"

# Step 5: Query with --promql, CSV output.
echo "--- Step 5: Query with --promql (csv) ---"
CSV_OUTPUT=$("$DASH0" metrics instant \
  --promql "{otel_metric_name=\"dash0.logs\",service_name=\"${SERVICE_NAME}\"}" \
  -o csv)
echo "$CSV_OUTPUT"
HEADER=$(echo "$CSV_OUTPUT" | head -n 1)
if ! echo "$HEADER" | grep -q "timestamp"; then
  echo "FAIL: CSV header missing 'timestamp'"
  exit 1
fi
if ! echo "$HEADER" | grep -q "__name__"; then
  echo "FAIL: CSV header missing '__name__'"
  exit 1
fi
if ! echo "$HEADER" | grep -q "value"; then
  echo "FAIL: CSV header missing 'value'"
  exit 1
fi
# Verify we got at least one data row.
DATA_LINES=$(echo "$CSV_OUTPUT" | tail -n +2 | wc -l | tr -d ' ')
if [ "$DATA_LINES" -lt 1 ]; then
  echo "FAIL: CSV output has no data rows"
  exit 1
fi

# Step 6: Query with --promql, CSV with --column and --skip-header.
echo "--- Step 6: Query with --column (csv) ---"
COL_OUTPUT=$("$DASH0" metrics instant \
  --promql "{otel_metric_name=\"dash0.logs\",service_name=\"${SERVICE_NAME}\"}" \
  -o csv --column __name__ --column service_name)
echo "$COL_OUTPUT"
COL_HEADER=$(echo "$COL_OUTPUT" | head -n 1)
if [ "$COL_HEADER" != "timestamp,__name__,service_name,value" ]; then
  echo "FAIL: expected CSV header 'timestamp,__name__,service_name,value', got '${COL_HEADER}'"
  exit 1
fi

# Step 7: Query with --filter instead of --promql.
# Use __name__ to select the metric and service.name to narrow to our test service.
echo "--- Step 7: Query with --filter (table) ---"
FILTER_OUTPUT=$("$DASH0" metrics instant \
  --filter "__name__ is dash0_logs_total" \
  --filter "service.name is ${SERVICE_NAME}")
echo "$FILTER_OUTPUT"
if ! echo "$FILTER_OUTPUT" | grep -q "Value:"; then
  echo "FAIL: filter query did not return results"
  exit 1
fi

# Step 8: Verify deprecated --query flag still works.
echo "--- Step 8: Deprecated --query flag ---"
DEPRECATED_OUTPUT=$("$DASH0" metrics instant \
  --query "{otel_metric_name=\"dash0.logs\",service_name=\"${SERVICE_NAME}\"}" 2>&1)
if ! echo "$DEPRECATED_OUTPUT" | grep -q "Value:"; then
  echo "FAIL: deprecated --query flag did not return results"
  exit 1
fi
echo "Deprecated --query flag works"

# Step 9: Verify --promql and --filter are mutually exclusive.
echo "--- Step 9: Mutual exclusivity ---"
if "$DASH0" metrics instant --promql "up" --filter "job is api" 2>/dev/null; then
  echo "FAIL: --promql and --filter should be mutually exclusive"
  exit 1
fi
echo "Mutual exclusivity enforced"

echo "=== Metrics instant round-trip test PASSED ==="
