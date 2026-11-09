#!/usr/bin/env bash
set -euo pipefail

# Exercises `dash0 apply` for Dash0GenerativeAiEvaluation documents. Confirms that:
#   - apply creates the eval on first run (PUT by origin);
#   - apply is idempotent on the second run (no duplicate, no changes);
#   - cleanup via the dedicated `evals delete` works.

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
TMPDIR="$(mktemp -d)"
SUFFIX="$(uuidgen | tr '[:upper:]' '[:lower:]' | cut -c1-8)"
NAME="cli-apply-${SUFFIX}"
ORIGIN="dash0-cli_apply-${SUFFIX}"
YAML_FILE="${TMPDIR}/eval.yaml"
sed -e "s|__ORIGIN__|${ORIGIN}|" -e "s|__NAME__|${NAME}|g" -e "s|__RATE__|0.1|" "${FIXTURES}/eval.yaml" > "$YAML_FILE"

cleanup() {
  "$DASH0" -X evals delete "$ORIGIN" --force > /dev/null 2>&1 || true
  rm -rf "$TMPDIR"
}
trap cleanup EXIT

echo "=== Eval apply idempotency test ==="
echo "Origin: $ORIGIN"

# Step 1: First apply — should create.
echo "--- Step 1: First apply (expect: created) ---"
APPLY1=$("$DASH0" -X apply -f "$YAML_FILE")
echo "$APPLY1"
if ! echo "$APPLY1" | grep -q "Eval \"$NAME\".*created"; then
  echo "FAIL: expected 'Eval \"$NAME\" ... created' in first apply output"
  exit 1
fi

# Step 2: Second apply — must be a no-op.
echo "--- Step 2: Second apply (expect: no changes) ---"
APPLY2=$("$DASH0" -X apply -f "$YAML_FILE")
echo "$APPLY2"
if ! echo "$APPLY2" | grep -q 'no changes'; then
  echo "FAIL: second apply did not report 'no changes' — re-applying an unchanged eval is not a no-op"
  exit 1
fi

# Step 3: Exactly one eval exists with this origin.
echo "--- Step 3: Verify exactly one eval with this origin ---"
COUNT=$("$DASH0" -X evals list --all -o json | jq --arg o "$ORIGIN" '[.[] | select(.metadata.labels["dash0.com/origin"] == $o)] | length')
if [ "$COUNT" != "1" ]; then
  echo "FAIL: expected exactly 1 eval with origin '$ORIGIN', got $COUNT"
  exit 1
fi

# Step 4: Cleanup (also runs from the EXIT trap as a safety net).
echo "--- Step 4: Delete ---"
"$DASH0" -X evals delete "$ORIGIN" --force > /dev/null

echo "=== Eval apply idempotency test PASSED ==="
