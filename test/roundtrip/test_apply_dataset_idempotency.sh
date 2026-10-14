#!/usr/bin/env bash
set -euo pipefail

# Exercises `dash0 apply` for Dash0Dataset documents. Confirms that:
#   - apply requires --experimental;
#   - apply creates the dataset on first run;
#   - apply is idempotent on the second run (PUT by slug, no duplicate);
#   - cleanup via the dedicated `datasets delete` works.

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
TMPDIR="$(mktemp -d)"
# Dash0 keeps a deleted dataset for 30 days, so every run uses a fresh slug.
SLUG="cli-apply-$(uuidgen | tr '[:upper:]' '[:lower:]' | cut -c1-8)"
NAME="Apply Roundtrip Dataset ${SLUG}"
YAML_FILE="${TMPDIR}/dataset.yaml"
sed -e "s|__SLUG__|${SLUG}|" -e "s|__NAME__|${NAME}|" "${FIXTURES}/dataset.yaml" > "$YAML_FILE"

cleanup() {
  "$DASH0" -X datasets delete "$SLUG" --force > /dev/null 2>&1 || true
  rm -rf "$TMPDIR"
}
trap cleanup EXIT

echo "=== Dataset apply idempotency test ==="
echo "Slug: $SLUG"

# Step 1: apply without --experimental must be rejected.
echo "--- Step 1: Apply without -X (expect: rejected) ---"
if "$DASH0" apply -f "$YAML_FILE" > /dev/null 2>&1; then
  echo "FAIL: apply of a Dash0Dataset succeeded without --experimental"
  exit 1
fi

# Step 2: First apply — should create.
echo "--- Step 2: First apply (expect: created) ---"
APPLY1=$("$DASH0" -X apply -f "$YAML_FILE")
echo "$APPLY1"
if ! echo "$APPLY1" | grep -q "Dataset"; then
  echo "FAIL: expected 'Dataset' in first apply output"
  exit 1
fi
if ! echo "$APPLY1" | grep -q "created"; then
  echo "FAIL: expected 'created' in first apply output"
  exit 1
fi

# Step 3: Second apply — must not create a duplicate.
echo "--- Step 3: Second apply (expect: no duplicate created) ---"
APPLY2=$("$DASH0" -X apply -f "$YAML_FILE")
echo "$APPLY2"
if echo "$APPLY2" | grep -q "created"; then
  echo "FAIL: unexpected 'created' on second apply"
  exit 1
fi

# Step 4: Exactly one dataset exists with this slug.
echo "--- Step 4: Verify exactly one dataset with this slug ---"
COUNT=$("$DASH0" -X datasets list -o json | jq --arg s "$SLUG" '[.[] | select(.metadata.labels["dash0.com/id"] == $s)] | length')
if [ "$COUNT" != "1" ]; then
  echo "FAIL: expected exactly 1 dataset with slug '$SLUG', got $COUNT"
  exit 1
fi

# Step 5: Cleanup (also runs from the EXIT trap as a safety net).
echo "--- Step 5: Delete ---"
"$DASH0" -X datasets delete "$SLUG" --force > /dev/null

echo "=== Dataset apply idempotency test PASSED ==="
