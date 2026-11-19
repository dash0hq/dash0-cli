#!/usr/bin/env bash
set -euo pipefail

# Exercises `dash0 apply` for signal-to-metrics rules. Confirms that:
#   - apply creates the rule on the first run;
#   - a second apply of the same file reports "no changes" and creates no
#     duplicate;
#   - get shows a deleted rule as deleted, and apply restores it.
#
# Step 2 asserts the output, not just the record count, so it fails if the diff
# logic regresses. A count-only check would pass even when every reapply
# rendered a spurious diff, which is what happens if marshalForDiff stops
# stripping dash0.com/version.
#
# The origin is unique per run, because origins are unique per organization and
# apply upserts by PUT, so a fixed one would overwrite a real rule.

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

ORIGIN="apply-s2m-$(uuidgen | tr '[:upper:]' '[:lower:]')"
YAML_FILE="${TMPDIR}/rule.yaml"

echo "=== Signal-to-metrics rule apply idempotency ==="
echo "Origin: $ORIGIN"

ORIGIN="$ORIGIN" yq '
  .metadata.name = env(ORIGIN) |
  .metadata.labels."dash0.com/origin" = env(ORIGIN) |
  .spec.display.name = env(ORIGIN)
' "${FIXTURES}/signal-to-metrics-rule.yaml" > "$YAML_FILE"

cleanup() {
  "$DASH0" s2m delete "$ORIGIN" --force > /dev/null 2>&1 || true
  rm -rf "$TMPDIR"
}
trap cleanup EXIT

echo "--- Step 1: First apply (expect: created) ---"
APPLY1=$("$DASH0" apply -f "$YAML_FILE")
echo "$APPLY1"
if ! echo "$APPLY1" | grep -q "created"; then
  echo "FAIL: expected 'created' in first apply output"
  exit 1
fi

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
COUNT=$("$DASH0" s2m list --all --origin-prefix "$ORIGIN" -o json | jq --arg o "$ORIGIN" '[.[] | select(.metadata.labels["dash0.com/origin"] == $o)] | length')
if [ "$COUNT" != "1" ]; then
  echo "FAIL: expected exactly 1 record with origin '$ORIGIN', got $COUNT"
  exit 1
fi

echo "--- Step 4: Delete ---"
if ! "$DASH0" s2m delete "$ORIGIN" --force > /dev/null; then
  echo "FAIL: delete failed"
  exit 1
fi

# Step 5: The API soft-deletes signal-to-metrics rules, so GET still returns
# the record after delete. `s2m get` must show it as deleted.
echo "--- Step 5: Get after delete (expect: Deleted line) ---"
GET5=$("$DASH0" s2m get "$ORIGIN")
echo "$GET5"
if ! echo "$GET5" | grep -q "^Deleted: "; then
  echo "FAIL: expected a 'Deleted:' line in s2m get output after delete"
  exit 1
fi

# Step 6: Apply again after deletion. Apply calls PUT (upsert), which restores
# the soft-deleted rule and reports it as "restored".
echo "--- Step 6: Apply after delete (expect: restored) ---"
APPLY6=$("$DASH0" apply -f "$YAML_FILE")
echo "$APPLY6"
if ! echo "$APPLY6" | grep -qE 'restored[[:space:]]*$'; then
  echo "FAIL: expected 'restored' in apply output after delete"
  exit 1
fi

echo "--- Step 7: Verify restored rule is active ---"
if ! GET7=$("$DASH0" s2m get "$ORIGIN"); then
  echo "FAIL: s2m get '$ORIGIN' failed after re-apply"
  exit 1
fi
if echo "$GET7" | grep -q "^Deleted: "; then
  echo "FAIL: s2m get still shows '$ORIGIN' as deleted after re-apply"
  exit 1
fi

echo "--- Step 8: Delete ---"
if ! "$DASH0" s2m delete "$ORIGIN" --force > /dev/null; then
  echo "FAIL: cleanup delete failed"
  exit 1
fi

echo "=== Signal-to-metrics rule apply idempotency test PASSED ==="
