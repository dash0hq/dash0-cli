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
FAILURES=()

for script in \
  "${SCRIPT_DIR}/test_dashboard_roundtrip.sh" \
  "${SCRIPT_DIR}/test_check_rule_roundtrip.sh" \
  "${SCRIPT_DIR}/test_synthetic_check_roundtrip.sh" \
  "${SCRIPT_DIR}/test_view_roundtrip.sh"; do

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
echo "Results: $PASSED passed, $FAILED failed"
if [ "$FAILED" -gt 0 ]; then
  echo "Failures:"
  for f in "${FAILURES[@]}"; do
    echo "  - $f"
  done
  exit 1
fi
echo "All tests passed!"
