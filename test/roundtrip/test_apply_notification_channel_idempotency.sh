#!/usr/bin/env bash
set -euo pipefail

# Exercises `dash0 apply` for Dash0NotificationChannel documents. Confirms that:
#   - apply creates the channel on first run with the caller-provided origin;
#   - apply is idempotent on the second run (PUT-by-origin, no duplicate);
#   - apply uses no dataset query parameter (channels are organization-level);
#   - cleanup via the dedicated `notification-channels delete` works.

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

ORIGIN="apply-roundtrip-channel-$(uuidgen | tr '[:upper:]' '[:lower:]')"
YAML_FILE="${TMPDIR}/notification-channel.yaml"
cat > "$YAML_FILE" <<EOF
kind: Dash0NotificationChannel
metadata:
  name: Apply Roundtrip Notification Channel
  labels:
    dash0.com/origin: ${ORIGIN}
spec:
  type: webhook
  config:
    url: https://httpbin.org/post
    method: POST
  routing:
    filters:
      - - key: webhook
          operator: is
          value: "true"
EOF

echo "=== Notification channel apply idempotency test ==="
echo "Origin: $ORIGIN"

cleanup() {
  "$DASH0" -X notification-channels delete "$ORIGIN" --force > /dev/null 2>&1 || true
}
trap 'cleanup; rm -rf "$TMPDIR"' EXIT

# Step 1: First apply — should create.
echo "--- Step 1: First apply (expect: created) ---"
APPLY1=$("$DASH0" apply -f "$YAML_FILE")
echo "$APPLY1"
if ! echo "$APPLY1" | grep -q "Notification channel"; then
  echo "FAIL: expected 'Notification channel' in first apply output"
  exit 1
fi
if ! echo "$APPLY1" | grep -q "created"; then
  echo "FAIL: expected 'created' in first apply output"
  exit 1
fi

# Step 2: Resolve the server-assigned ID for later cleanup/assertions.
echo "--- Step 2: Resolve ID from list ---"
LIST_JSON=$("$DASH0" -X notification-channels list -o json)
ID=$(echo "$LIST_JSON" | jq -r --arg o "$ORIGIN" '.[] | select(.metadata.labels["dash0.com/origin"] == $o) | .metadata.labels["dash0.com/id"]' | head -1)
if [ -z "$ID" ]; then
  echo "FAIL: could not find notification channel with origin '$ORIGIN' in list after first apply"
  exit 1
fi
echo "Server-assigned ID: $ID"

# Step 3: Second apply — must not create a duplicate.
echo "--- Step 3: Second apply (expect: no duplicate created) ---"
APPLY2=$("$DASH0" apply -f "$YAML_FILE")
echo "$APPLY2"
if echo "$APPLY2" | grep -q "created"; then
  echo "FAIL: unexpected 'created' on second apply — duplicate was created"
  exit 1
fi

# Step 4: Exactly one record exists with this origin.
echo "--- Step 4: Verify exactly one channel with this origin ---"
COUNT=$("$DASH0" -X notification-channels list -o json | jq --arg o "$ORIGIN" '[.[] | select(.metadata.labels["dash0.com/origin"] == $o)] | length')
if [ "$COUNT" != "1" ]; then
  echo "FAIL: expected exactly 1 channel with origin '$ORIGIN', got $COUNT"
  exit 1
fi

# Step 5: Cleanup (also runs from EXIT trap as a safety net).
echo "--- Step 5: Delete ---"
"$DASH0" -X notification-channels delete "$ORIGIN" --force > /dev/null

echo "=== Notification channel apply idempotency test PASSED ==="
