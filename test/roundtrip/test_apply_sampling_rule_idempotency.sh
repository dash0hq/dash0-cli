#!/usr/bin/env bash
set -euo pipefail

# Exercises `dash0 apply` for sampling rules. Confirms that:
#   - apply creates the rule on the first run;
#   - a second apply of the same file reports "no changes" and creates no
#     duplicate;
#   - apply after a delete creates a new rule with a new ID, because the API
#     deletes sampling rules permanently.
#
# Step 2 asserts the output, not just the record count, so it fails if the diff
# logic regresses. A count-only check would pass even when every reapply
# rendered a spurious diff.
#
# The origin is unique per run, because origins are unique per organization and
# apply upserts by PUT, so a fixed one would overwrite a real rule. The rule is
# disabled, so it never turns sampling on for the dataset.

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

ORIGIN="apply-sampling-$(uuidgen | tr '[:upper:]' '[:lower:]')"
YAML_FILE="${TMPDIR}/rule.yaml"

echo "=== Sampling rule apply idempotency ==="
echo "Origin: $ORIGIN"

ORIGIN="$ORIGIN" yq '
  .metadata.name = env(ORIGIN) |
  .metadata.labels."dash0.com/origin" = env(ORIGIN) |
  .spec.display.name = env(ORIGIN)
' "${FIXTURES}/sampling-rule.yaml" > "$YAML_FILE"

cleanup() {
  "$DASH0" sampling-rules delete "$ORIGIN" --force > /dev/null 2>&1 || true
  rm -rf "$TMPDIR"
}
trap cleanup EXIT

count_rules() {
  "$DASH0" sampling-rules list --all -o json | jq --arg o "$ORIGIN" '[.[] | select(.metadata.labels["dash0.com/origin"] == $o)] | length'
}

echo "--- Step 1: First apply (expect: created) ---"
APPLY1=$("$DASH0" apply -f "$YAML_FILE")
echo "$APPLY1"
if ! echo "$APPLY1" | grep -q "created"; then
  echo "FAIL: expected 'created' in first apply output"
  exit 1
fi
ID1=$("$DASH0" sampling-rules get "$ORIGIN" -o json | jq -r '.metadata.labels["dash0.com/id"]')

echo "--- Step 2: Second apply (expect: no changes, no duplicate) ---"
if ! APPLY2=$("$DASH0" apply -f "$YAML_FILE" 2>&1); then
  echo "FAIL: second apply errored out (expected idempotent upsert):"
  echo "$APPLY2"
  exit 1
fi
echo "$APPLY2"
if echo "$APPLY2" | grep -q "created"; then
  echo "FAIL: unexpected 'created' on second apply — a duplicate was created"
  exit 1
fi
if ! echo "$APPLY2" | grep -q "no changes"; then
  echo "FAIL: expected 'no changes' on the second apply of identical content."
  echo "This usually means the diff no longer strips the server-managed labels"
  echo "(dash0.com/version is incremented on every PUT)."
  exit 1
fi

echo "--- Step 3: Exactly one record with this origin ---"
COUNT=$(count_rules)
if [ "$COUNT" != "1" ]; then
  echo "FAIL: expected exactly 1 record with origin '$ORIGIN', got $COUNT"
  exit 1
fi

echo "--- Step 4: Delete ---"
if ! "$DASH0" sampling-rules delete "$ORIGIN" --force > /dev/null; then
  echo "FAIL: delete failed"
  exit 1
fi

echo "--- Step 5: Apply after delete (expect: created, with a new ID) ---"
APPLY5=$("$DASH0" apply -f "$YAML_FILE")
echo "$APPLY5"
if ! echo "$APPLY5" | grep -q "created"; then
  echo "FAIL: expected 'created' in apply output after a permanent delete"
  exit 1
fi
ID5=$("$DASH0" sampling-rules get "$ORIGIN" -o json | jq -r '.metadata.labels["dash0.com/id"]')
if [ "$ID5" = "$ID1" ]; then
  echo "FAIL: expected a new ID after a permanent delete, got the old one '$ID1'"
  exit 1
fi
COUNT5=$(count_rules)
if [ "$COUNT5" != "1" ]; then
  echo "FAIL: expected exactly 1 record with origin '$ORIGIN' after re-apply, got $COUNT5"
  exit 1
fi

echo "--- Step 6: Delete ---"
if ! "$DASH0" sampling-rules delete "$ORIGIN" --force > /dev/null; then
  echo "FAIL: cleanup delete failed"
  exit 1
fi

echo "=== Sampling rule apply idempotency test PASSED ==="
