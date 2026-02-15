#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

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
