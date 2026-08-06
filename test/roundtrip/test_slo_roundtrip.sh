#!/usr/bin/env bash
set -euo pipefail

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
FIXTURE="${FIXTURES}/slo.yaml"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

ASSET_NAME=$(yq '.metadata.annotations."dash0.com/display-name"' "$FIXTURE")

echo "=== SLO round-trip test ==="
echo "Asset name: $ASSET_NAME"

# Step 1: Create from fixture
echo "--- Step 1: Create SLO from fixture ---"
if ! CREATE_OUTPUT=$("$DASH0" slos create -f "$FIXTURE"); then
  echo "FAIL: slos create failed"
  exit 1
fi
echo "$CREATE_OUTPUT"
if ! echo "$CREATE_OUTPUT" | grep -q "$ASSET_NAME"; then
  echo "FAIL: create output does not mention asset name '$ASSET_NAME'"
  exit 1
fi

# Step 2: List SLOs and find the created asset by name. Polled rather than
# asserted once: a write is not always immediately visible to a list query on the
# dev tenant, which made this step flaky.
echo "--- Step 2: List SLOs and find created asset ---"
MAX_ATTEMPTS=6
DELAY=2
ID=""
for attempt in $(seq 1 "$MAX_ATTEMPTS"); do
  if ! LIST_JSON=$("$DASH0" slos list --all -o json); then
    echo "FAIL: slos list -o json failed"
    exit 1
  fi
  ID=$(echo "$LIST_JSON" | jq -r --arg name "$ASSET_NAME" '[.[] | select(.metadata.annotations["dash0.com/display-name"] == $name)][0].metadata.labels["dash0.com/id"] // empty')
  if [ -n "$ID" ]; then
    echo "Created SLO ID: $ID (attempt $attempt)"
    break
  fi
  if [ "$attempt" -eq "$MAX_ATTEMPTS" ]; then
    echo "FAIL: Could not find created SLO '$ASSET_NAME' in list after $MAX_ATTEMPTS attempts"
    exit 1
  fi
  echo "Attempt $attempt/$MAX_ATTEMPTS: not yet listed, retrying in ${DELAY}s..."
  sleep "$DELAY"
  DELAY=$((DELAY * 2))
done

# Step 3: Get by ID
echo "--- Step 3: Get SLO by ID ---"
if ! "$DASH0" slos get "$ID"; then
  echo "FAIL: slos get failed"
  exit 1
fi

# Step 4: Export to YAML
echo "--- Step 4: Export SLO to YAML ---"
if ! "$DASH0" slos get "$ID" -o yaml > "${TMPDIR}/exported.yaml"; then
  echo "FAIL: slos get -o yaml failed"
  exit 1
fi
echo "Exported to ${TMPDIR}/exported.yaml"

# Step 5: Re-import exported YAML via apply (round-trip). Re-applying a document
# that was just exported unchanged must be a no-op, so apply must state that no
# changes were detected. This is what closes the round-trip: it proves the export
# is faithful enough to feed straight back in, and that the server-managed
# metadata the export carries (dash0.com/version, created-at, updated-at, and so
# on) is normalized out of the comparison rather than counted as a change.
echo "--- Step 5: Re-import exported YAML via apply (expect: no changes) ---"
if ! APPLY_OUTPUT=$("$DASH0" apply -f "${TMPDIR}/exported.yaml"); then
  echo "FAIL: apply failed"
  exit 1
fi
echo "$APPLY_OUTPUT"
if ! echo "$APPLY_OUTPUT" | grep -q 'no changes'; then
  echo "FAIL: re-applying the exported SLO did not report 'no changes'"
  exit 1
fi

# Step 6: Delete
echo "--- Step 6: Delete SLO ---"
if ! "$DASH0" slos delete "$ID" --force; then
  echo "FAIL: slos delete failed"
  exit 1
fi

# Step 7: Verify deletion. Polled for the same read-lag reason as Step 2 — the
# delete may not be reflected in a list query immediately.
echo "--- Step 7: Verify deletion ---"
MAX_ATTEMPTS=6
DELAY=2
for attempt in $(seq 1 "$MAX_ATTEMPTS"); do
  if ! LIST_JSON=$("$DASH0" slos list --all -o json); then
    echo "FAIL: slos list -o json failed"
    exit 1
  fi
  if ! echo "$LIST_JSON" | jq -e --arg id "$ID" '.[] | select(.metadata.labels["dash0.com/id"] == $id)' > /dev/null 2>&1; then
    echo "SLO '$ID' is gone (attempt $attempt)"
    break
  fi
  if [ "$attempt" -eq "$MAX_ATTEMPTS" ]; then
    echo "FAIL: SLO '$ID' still exists after deletion (checked $MAX_ATTEMPTS times)"
    exit 1
  fi
  echo "Attempt $attempt/$MAX_ATTEMPTS: still listed, retrying in ${DELAY}s..."
  sleep "$DELAY"
  DELAY=$((DELAY * 2))
done

echo "=== SLO round-trip test PASSED ==="
