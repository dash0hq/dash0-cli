#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"

# Verify that an active profile is configured before running any tests.
if ! "$DASH0" config show >/dev/null 2>&1; then
  echo "Error: no active profile configured."
  echo "Create one with: dash0 config profiles create <name> --api-url <url> --auth-token <token>"
  exit 1
fi

echo "Active profile:"
"$DASH0" config show
echo
echo "Running all round-trip tests..."
echo

PASSED=0
FAILED=0
SKIPPED=0
FAILURES=()

# API-side round-trip tests only need DASH0_API_URL + DASH0_AUTH_TOKEN.
API_TESTS=(
  "${SCRIPT_DIR}/test_dashboard_roundtrip.sh"
  "${SCRIPT_DIR}/test_check_rule_roundtrip.sh"
  "${SCRIPT_DIR}/test_synthetic_check_roundtrip.sh"
  "${SCRIPT_DIR}/test_view_roundtrip.sh"
  "${SCRIPT_DIR}/test_apply_dashboard_idempotency.sh"
  "${SCRIPT_DIR}/test_apply_check_rule_idempotency.sh"
  "${SCRIPT_DIR}/test_apply_view_idempotency.sh"
  "${SCRIPT_DIR}/test_apply_synthetic_check_idempotency.sh"
  "${SCRIPT_DIR}/test_dashboard_annotations.sh"
  "${SCRIPT_DIR}/test_view_annotations.sh"
  "${SCRIPT_DIR}/test_synthetic_check_annotations.sh"
  "${SCRIPT_DIR}/test_prometheus_rule_roundtrip.sh"
  "${SCRIPT_DIR}/test_perses_dashboard_roundtrip.sh"
  "${SCRIPT_DIR}/test_notification_channel_roundtrip.sh"
  "${SCRIPT_DIR}/test_metrics_instant_roundtrip.sh"
  "${SCRIPT_DIR}/test_team_roundtrip.sh"
)

# OTLP-based round-trip tests additionally need DASH0_OTLP_URL.
OTLP_TESTS=(
  "${SCRIPT_DIR}/test_log_roundtrip.sh"
  "${SCRIPT_DIR}/test_span_roundtrip.sh"
)

TESTS=("${API_TESTS[@]}")
OTLP_URL_FROM_PROFILE=$("$DASH0" config show -o json 2>/dev/null | sed -n 's/.*"otlpUrl":[[:space:]]*{[[:space:]]*"value":[[:space:]]*"\([^"]*\)".*/\1/p')
if [ -n "${DASH0_OTLP_URL:-}" ] || [ -n "$OTLP_URL_FROM_PROFILE" ]; then
  TESTS+=("${OTLP_TESTS[@]}")
else
  echo "Note: DASH0_OTLP_URL is not configured; skipping OTLP round-trip tests."
  echo
  SKIPPED=${#OTLP_TESTS[@]}
fi

for script in "${TESTS[@]}"; do

  name="$(basename "$script" .sh)"
  echo "========================================"
  echo "Running: $name"
  echo "========================================"
  if bash "$script"; then
    PASSED=$((PASSED + 1))
  else
    FAILED=$((FAILED + 1))
    FAILURES+=("$name")
  fi
  echo
done

echo "========================================"
if [ "$SKIPPED" -gt 0 ]; then
  echo "Results: $PASSED passed, $FAILED failed, $SKIPPED skipped"
else
  echo "Results: $PASSED passed, $FAILED failed"
fi
if [ "$FAILED" -gt 0 ]; then
  echo "Failures:"
  for f in "${FAILURES[@]}"; do
    echo "  - $f"
  done
  exit 1
fi
echo "All tests passed!"
