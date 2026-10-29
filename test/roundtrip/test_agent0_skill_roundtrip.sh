#!/usr/bin/env bash
set -euo pipefail

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
TMPDIR="$(mktemp -d)"
SUFFIX="$(uuidgen | tr '[:upper:]' '[:lower:]' | cut -c1-8)"
NAME="cli-rt-${SUFFIX}"
ORIGIN="dash0-cli_rt-${SUFFIX}"
FIXTURE="${TMPDIR}/skill.yaml"
CREATED=0

cleanup() {
  if [ "$CREATED" = "1" ]; then
    echo "--- Cleanup: deleting Agent0 skill $ORIGIN ---"
    "$DASH0" -X agent0 skills delete "$ORIGIN" --force > /dev/null 2>&1 || true
  fi
  rm -rf "$TMPDIR"
}
trap cleanup EXIT

sed -e "s|__ORIGIN__|${ORIGIN}|" -e "s|__NAME__|${NAME}|" -e "s|__NOTE__|Original note.|" "${FIXTURES}/agent0-skill.yaml" > "$FIXTURE"

echo "=== Agent0 skill round-trip test ==="
echo "Origin: $ORIGIN"

# Step 1: Create from fixture
echo "--- Step 1: Create skill from fixture ---"
if ! CREATE_OUTPUT=$("$DASH0" -X agent0 skills create -f "$FIXTURE"); then
  echo "FAIL: agent0 skills create failed"
  exit 1
fi
CREATED=1
echo "$CREATE_OUTPUT"
if ! echo "$CREATE_OUTPUT" | grep -q "\"$NAME\" created"; then
  echo "FAIL: create output does not report skill '$NAME' as created"
  exit 1
fi

# Step 2: List by origin prefix and find the created skill
echo "--- Step 2: List skills and find created skill ---"
FOUND=$("$DASH0" -X agent0 skills list --origin-prefix "$ORIGIN" -o json | jq -r '[.[] | .metadata.name][0] // empty')
if [ "$FOUND" != "$NAME" ]; then
  echo "FAIL: Could not find created skill '$ORIGIN' in list (found name: '$FOUND')"
  exit 1
fi

# Step 3: Get by origin; spec.permissions, defaultEnabled included, round-trips
echo "--- Step 3: Get skill by origin ---"
GET_JSON=$("$DASH0" -X agent0 skills get "$ORIGIN" -o json)
DEFAULT_ENABLED=$(echo "$GET_JSON" | jq -r '.spec.permissions[] | select(.role == "basic_member") | .defaultEnabled')
if [ "$DEFAULT_ENABLED" != "false" ]; then
  echo "FAIL: expected basic_member defaultEnabled false, got '$DEFAULT_ENABLED'"
  exit 1
fi

# Step 4: Export, edit a file, and update
echo "--- Step 4: Export, edit, and update ---"
EXPORT_FILE="${TMPDIR}/exported.yaml"
"$DASH0" -X agent0 skills get "$ORIGIN" -o yaml > "$EXPORT_FILE"
yq -i '(.spec.files[] | select(.relativePath == "references/notes.md") | .content) = "# Notes\n\nEdited note.\n"' "$EXPORT_FILE"
if ! UPDATE_OUTPUT=$("$DASH0" -X agent0 skills update -f "$EXPORT_FILE"); then
  echo "FAIL: agent0 skills update failed"
  exit 1
fi
echo "$UPDATE_OUTPUT"
NOTE=$("$DASH0" -X agent0 skills get "$ORIGIN" -o json | jq -r '.spec.files[] | select(.relativePath == "references/notes.md") | .content')
if ! echo "$NOTE" | grep -q "Edited note."; then
  echo "FAIL: expected the edited note after update, got '$NOTE'"
  exit 1
fi

# Step 5: Re-applying the export is a no-op
echo "--- Step 5: Apply the export (expect: no changes) ---"
APPLY_OUTPUT=$("$DASH0" -X apply -f "$EXPORT_FILE")
echo "$APPLY_OUTPUT"
if ! echo "$APPLY_OUTPUT" | grep -q 'no changes'; then
  echo "FAIL: applying the unchanged export did not report 'no changes'"
  exit 1
fi

# Step 6: Delete
echo "--- Step 6: Delete skill ---"
if ! "$DASH0" -X agent0 skills delete "$ORIGIN" --force; then
  echo "FAIL: agent0 skills delete failed"
  exit 1
fi

# Step 7: Verify deletion
echo "--- Step 7: Verify deletion ---"
COUNT=$("$DASH0" -X agent0 skills list --origin-prefix "$ORIGIN" -o json | jq 'length')
if [ "$COUNT" != "0" ]; then
  echo "FAIL: Agent0 skill '$ORIGIN' still exists after deletion"
  exit 1
fi

# Step 8: Deleting again with --force succeeds (idempotent)
echo "--- Step 8: Delete again with --force (expect: success) ---"
if ! "$DASH0" -X agent0 skills delete "$ORIGIN" --force; then
  echo "FAIL: second agent0 skills delete --force failed"
  exit 1
fi
CREATED=0

echo "=== Agent0 skill round-trip test PASSED ==="
