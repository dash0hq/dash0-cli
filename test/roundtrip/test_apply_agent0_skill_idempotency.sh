#!/usr/bin/env bash
set -euo pipefail

# Exercises `dash0 apply` for Dash0Skill documents. Confirms that:
#   - apply creates the skill on first run (PUT by origin);
#   - apply is idempotent on the second run (no duplicate, no changes);
#   - cleanup via the dedicated `agent0 skills delete` works.

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
TMPDIR="$(mktemp -d)"
SUFFIX="$(uuidgen | tr '[:upper:]' '[:lower:]' | cut -c1-8)"
NAME="cli-apply-${SUFFIX}"
ORIGIN="dash0-cli_apply-${SUFFIX}"
YAML_FILE="${TMPDIR}/skill.yaml"
sed -e "s|__ORIGIN__|${ORIGIN}|" -e "s|__NAME__|${NAME}|" -e "s|__NOTE__|Apply note.|" "${FIXTURES}/agent0-skill.yaml" > "$YAML_FILE"

cleanup() {
  "$DASH0" -X agent0 skills delete "$ORIGIN" --force > /dev/null 2>&1 || true
  rm -rf "$TMPDIR"
}
trap cleanup EXIT

echo "=== Agent0 skill apply idempotency test ==="
echo "Origin: $ORIGIN"

# Step 1: First apply — should create.
echo "--- Step 1: First apply (expect: created) ---"
APPLY1=$("$DASH0" -X apply -f "$YAML_FILE")
echo "$APPLY1"
if ! echo "$APPLY1" | grep -q "Agent0 skill"; then
  echo "FAIL: expected 'Agent0 skill' in first apply output"
  exit 1
fi
if ! echo "$APPLY1" | grep -q "created"; then
  echo "FAIL: expected 'created' in first apply output"
  exit 1
fi

# Step 2: Second apply — must be a no-op.
echo "--- Step 2: Second apply (expect: no changes) ---"
APPLY2=$("$DASH0" -X apply -f "$YAML_FILE")
echo "$APPLY2"
if ! echo "$APPLY2" | grep -q 'no changes'; then
  echo "FAIL: second apply did not report 'no changes' — re-applying an unchanged skill is not a no-op"
  exit 1
fi

# Step 3: Exactly one skill exists with this origin.
echo "--- Step 3: Verify exactly one skill with this origin ---"
COUNT=$("$DASH0" -X agent0 skills list --origin-prefix "$ORIGIN" -o json | jq 'length')
if [ "$COUNT" != "1" ]; then
  echo "FAIL: expected exactly 1 skill with origin '$ORIGIN', got $COUNT"
  exit 1
fi

# Step 4: Cleanup (also runs from the EXIT trap as a safety net).
echo "--- Step 4: Delete ---"
"$DASH0" -X agent0 skills delete "$ORIGIN" --force > /dev/null

echo "=== Agent0 skill apply idempotency test PASSED ==="
