#!/usr/bin/env bash
set -euo pipefail

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURE_TEMPLATE="${SCRIPT_DIR}/fixtures/auth-token.yaml"
TMPDIR="$(mktemp -d)"
SUFFIX="$(uuidgen | tr '[:upper:]' '[:lower:]' | cut -c1-8)"
ORIGIN="dash0-cli_rt-${SUFFIX}"
MISSING_ORIGIN="dash0-cli_rt-missing-${SUFFIX}"
NAME="cli-rt-${SUFFIX}"
UPDATED_NAME="${NAME}-edited"
FIXTURE="${TMPDIR}/auth-token.yaml"
SECRET_FILE="${TMPDIR}/token.secret"
CREATED=0

cleanup() {
  if [ "$CREATED" = "1" ]; then
    echo "--- Cleanup: deleting auth token $ORIGIN ---"
    "$DASH0" -X auth-tokens delete "$ORIGIN" --force > /dev/null 2>&1 || true
  fi
  "$DASH0" -X auth-tokens delete "$MISSING_ORIGIN" --force > /dev/null 2>&1 || true
  rm -rf "$TMPDIR"
}
trap cleanup EXIT

file_mode() {
  stat -c %a "$1" 2>/dev/null || stat -f %Lp "$1"
}

sed -e "s|__ORIGIN__|${ORIGIN}|" -e "s|__NAME__|${NAME}|" "$FIXTURE_TEMPLATE" > "$FIXTURE"

echo "=== Auth token round-trip test ==="
echo "Origin: $ORIGIN"

# Step 1: Without --secret-file, a non-interactive create fails before creating anything
echo "--- Step 1: Create without --secret-file (expect: failure, nothing created) ---"
if "$DASH0" -X auth-tokens create -f "$FIXTURE" > /dev/null 2>&1; then
  CREATED=1
  echo "FAIL: create without --secret-file succeeded outside a terminal"
  exit 1
fi
if "$DASH0" -X auth-tokens get "$ORIGIN" > /dev/null 2>&1; then
  CREATED=1
  echo "FAIL: create without --secret-file created auth token '$ORIGIN'"
  exit 1
fi

# Step 2: Create with --secret-file
echo "--- Step 2: Create auth token with --secret-file ---"
if ! CREATE_OUTPUT=$("$DASH0" -X auth-tokens create -f "$FIXTURE" --secret-file "$SECRET_FILE"); then
  echo "FAIL: auth-tokens create failed"
  exit 1
fi
CREATED=1
echo "$CREATE_OUTPUT"
if echo "$CREATE_OUTPUT" | grep -q "auth_"; then
  echo "FAIL: create output contains a secret"
  exit 1
fi
MODE=$(file_mode "$SECRET_FILE")
if [ "$MODE" != "600" ]; then
  echo "FAIL: expected secret file mode 600, got $MODE"
  exit 1
fi
if ! grep -q "^auth_" "$SECRET_FILE"; then
  echo "FAIL: secret file does not hold an auth token"
  exit 1
fi

# Step 3: The secret works as a read-only token
echo "--- Step 3: Use the new secret to list dashboards ---"
if ! "$DASH0" dashboards list --auth-token-file "$SECRET_FILE" > /dev/null; then
  echo "FAIL: the delivered secret was not accepted by the API"
  exit 1
fi

# Step 4: List, get, and the JSON outputs carry no secret
echo "--- Step 4: List and get the auth token ---"
LIST_JSON=$("$DASH0" -X auth-tokens list -o json)
FOUND=$(echo "$LIST_JSON" | jq -r --arg origin "$ORIGIN" '[.[] | select(.metadata.labels["dash0.com/origin"] == $origin)][0].metadata.name // empty')
if [ "$FOUND" != "$NAME" ]; then
  echo "FAIL: could not find auth token '$ORIGIN' in list (found name: '$FOUND')"
  exit 1
fi
GET_JSON=$("$DASH0" -X auth-tokens get "$ORIGIN" -o json)
if echo "$LIST_JSON$GET_JSON" | grep -qF "$(cat "$SECRET_FILE")"; then
  echo "FAIL: list or get output contains the secret"
  exit 1
fi

# Step 5: Creating the same origin again is refused
echo "--- Step 5: Create the same origin again (expect: failure) ---"
if "$DASH0" -X auth-tokens create -f "$FIXTURE" --secret-file "${TMPDIR}/second.secret" > /dev/null 2>&1; then
  echo "FAIL: create with an existing origin succeeded"
  exit 1
fi

# Step 6: Update the name and verify the change
echo "--- Step 6: Update auth token ---"
EXPORT_FILE="${TMPDIR}/exported.yaml"
"$DASH0" -X auth-tokens get "$ORIGIN" -o yaml > "$EXPORT_FILE"
yq -i ".metadata.name = \"${UPDATED_NAME}\"" "$EXPORT_FILE"
if ! UPDATE_OUTPUT=$("$DASH0" -X auth-tokens update -f "$EXPORT_FILE"); then
  echo "FAIL: auth-tokens update failed"
  exit 1
fi
echo "$UPDATE_OUTPUT"
UPDATED=$("$DASH0" -X auth-tokens get "$ORIGIN" -o json | jq -r '.metadata.name')
if [ "$UPDATED" != "$UPDATED_NAME" ]; then
  echo "FAIL: expected name '$UPDATED_NAME' after update, got '$UPDATED'"
  exit 1
fi

# Step 7: Updating an origin that does not exist fails and creates nothing
echo "--- Step 7: Update a missing origin (expect: failure, nothing created) ---"
MISSING_FIXTURE="${TMPDIR}/missing.yaml"
sed -e "s|__ORIGIN__|${MISSING_ORIGIN}|" -e "s|__NAME__|${NAME}|" "$FIXTURE_TEMPLATE" > "$MISSING_FIXTURE"
if "$DASH0" -X auth-tokens update -f "$MISSING_FIXTURE" > /dev/null 2>&1; then
  echo "FAIL: update of a missing origin succeeded"
  exit 1
fi
if "$DASH0" -X auth-tokens get "$MISSING_ORIGIN" > /dev/null 2>&1; then
  echo "FAIL: update of a missing origin created auth token '$MISSING_ORIGIN'"
  exit 1
fi

# Step 8: Delete, then delete again with --force (idempotent)
echo "--- Step 8: Delete auth token ---"
if ! "$DASH0" -X auth-tokens delete "$ORIGIN" --force; then
  echo "FAIL: auth-tokens delete failed"
  exit 1
fi
CREATED=0
if ! "$DASH0" -X auth-tokens delete "$ORIGIN" --force; then
  echo "FAIL: second auth-tokens delete --force failed"
  exit 1
fi

echo "=== Auth token round-trip test PASSED ==="
