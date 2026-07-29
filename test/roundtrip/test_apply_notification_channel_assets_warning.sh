#!/usr/bin/env bash
set -euo pipefail

# Exercises the API-managed spec.routing.assets handling for Dash0NotificationChannel documents.
# The Dash0 API treats spec.routing.assets as a server-derived back-reference and ignores any
# value supplied on write. Confirms that:
#   - apply warns on stderr when the document carries a non-empty spec.routing.assets;
#   - the apply itself still succeeds (the warning is non-fatal);
#   - a second apply stays idempotent;
#   - the fabricated asset entry is NOT persisted server-side (checked via the raw API);
#   - a channel bound by a real check rule shows the binding via the raw API, but
#     `get -o yaml`/`-o json` omit the API-managed field from exports;
#   - a get -> update roundtrip of the bound channel succeeds without a warning;
#   - `notification-channels update` with hand-added assets still warns;
#   - cleanup via `check-rules delete` and `notification-channels delete` works.

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
TMPDIR="$(mktemp -d)"

WARNING_PATTERN="spec.routing.assets is API-managed and ignored on write"
FABRICATED_ID="00000000-0000-0000-0000-000000000001"

ORIGIN="apply-assets-warning-channel-$(uuidgen | tr '[:upper:]' '[:lower:]')"
CHECK_RULE_ID=$(uuidgen | tr '[:upper:]' '[:lower:]')
YAML_FILE="${TMPDIR}/notification-channel.yaml"
cat > "$YAML_FILE" <<EOF
kind: Dash0NotificationChannel
metadata:
  name: Apply Assets Warning Notification Channel
  labels:
    dash0.com/origin: ${ORIGIN}
spec:
  type: webhook
  config:
    url: https://httpbin.org/post
    method: POST
  routing:
    filters: []
    assets:
      - kind: check_rule
        id: ${FABRICATED_ID}
        name: does-not-matter
        dataset: default
EOF

echo "=== Notification channel routing.assets warning test ==="
echo "Origin: $ORIGIN"

cleanup() {
  "$DASH0" check-rules delete "$CHECK_RULE_ID" --force > /dev/null 2>&1 || true
  "$DASH0" -X notification-channels delete "$ORIGIN" --force > /dev/null 2>&1 || true
}
trap 'cleanup; rm -rf "$TMPDIR"' EXIT

# Step 1: First apply — expect a warning on stderr AND a successful create.
echo "--- Step 1: First apply (expect: warning + created) ---"
APPLY1_STDERR="${TMPDIR}/apply1.stderr"
APPLY1=$("$DASH0" apply -f "$YAML_FILE" 2> "$APPLY1_STDERR")
echo "$APPLY1"
cat "$APPLY1_STDERR"
if ! grep -q "$WARNING_PATTERN" "$APPLY1_STDERR"; then
  echo "FAIL: expected the API-managed routing.assets warning on stderr"
  exit 1
fi
if ! echo "$APPLY1" | grep -q "created"; then
  echo "FAIL: expected 'created' in first apply output — the warning must not block the apply"
  exit 1
fi

# Step 2: Second apply — still warns, still idempotent.
echo "--- Step 2: Second apply (expect: warning + no duplicate) ---"
APPLY2_STDERR="${TMPDIR}/apply2.stderr"
APPLY2=$("$DASH0" apply -f "$YAML_FILE" 2> "$APPLY2_STDERR")
echo "$APPLY2"
if ! grep -q "$WARNING_PATTERN" "$APPLY2_STDERR"; then
  echo "FAIL: expected the API-managed routing.assets warning on the second apply too"
  exit 1
fi
if echo "$APPLY2" | grep -q "created"; then
  echo "FAIL: second apply must update (PUT-by-origin), not create a duplicate"
  exit 1
fi

# Step 3: The fabricated asset entry must not have been persisted — checked via the raw API,
# since the CLI's own get/list outputs omit the API-managed field.
echo "--- Step 3: Fabricated asset not persisted server-side ---"
SERVER_CHANNEL="${TMPDIR}/server-channel.json"
"$DASH0" -X api "/api/notification-channels/${ORIGIN}" --dataset "" > "$SERVER_CHANNEL"
if grep -q "$FABRICATED_ID" "$SERVER_CHANNEL"; then
  echo "FAIL: the fabricated asset entry was persisted — the API must ignore spec.routing.assets on write"
  exit 1
fi
CHANNEL_ID=$(jq -r '.metadata.labels["dash0.com/id"]' "$SERVER_CHANNEL")
if [ -z "$CHANNEL_ID" ] || [ "$CHANNEL_ID" = "null" ]; then
  echo "FAIL: could not resolve the channel's server-assigned UUID"
  exit 1
fi

# Step 4: Bind a real check rule to the channel, confirm the server-side back-reference exists,
# and confirm the CLI export omits it.
echo "--- Step 4: Bound channel — server shows assets, export omits them ---"
CHECK_RULE_FILE="${TMPDIR}/check-rule.yaml"
cat > "$CHECK_RULE_FILE" <<EOF
id: ${CHECK_RULE_ID}
name: Assets Warning Binding Rule
summary: Binds the assets-warning test channel
expression: vector(0) > \$__threshold
for: 0s
interval: 1m0s
enabled: false
thresholds: {}
annotations:
  summary: Binds the assets-warning test channel
  dash0.com/notification-channel-ids: ${CHANNEL_ID}
EOF
APPLY_RULE=$("$DASH0" apply -f "$CHECK_RULE_FILE")
echo "$APPLY_RULE"
"$DASH0" -X api "/api/notification-channels/${ORIGIN}" --dataset "" > "$SERVER_CHANNEL"
if ! jq -e '.spec.routing.assets | length > 0' "$SERVER_CHANNEL" > /dev/null; then
  echo "FAIL: expected the raw API to report the check-rule binding in spec.routing.assets"
  exit 1
fi
EXPORT_FILE="${TMPDIR}/exported-channel.yaml"
"$DASH0" -X notification-channels get "$ORIGIN" -o yaml > "$EXPORT_FILE"
if grep -q "$CHECK_RULE_ID" "$EXPORT_FILE"; then
  echo "FAIL: get -o yaml must omit the API-managed spec.routing.assets back-reference"
  exit 1
fi

# Step 5: get -> update roundtrip of the bound channel must succeed without a warning.
echo "--- Step 5: get -> update roundtrip (expect: no warning) ---"
UPDATE1_STDERR="${TMPDIR}/update1.stderr"
if ! "$DASH0" -X notification-channels update -f "$EXPORT_FILE" > /dev/null 2> "$UPDATE1_STDERR"; then
  cat "$UPDATE1_STDERR"
  echo "FAIL: the get -> update roundtrip must succeed"
  exit 1
fi
cat "$UPDATE1_STDERR"
if grep -q "$WARNING_PATTERN" "$UPDATE1_STDERR"; then
  echo "FAIL: a get -> update roundtrip must not warn — the export carries no assets"
  exit 1
fi

# Step 6: update with hand-added assets — expect the warning, and the update still succeeds.
echo "--- Step 6: update with hand-added assets (expect: warning) ---"
UPDATE2_STDERR="${TMPDIR}/update2.stderr"
if ! "$DASH0" -X notification-channels update "$ORIGIN" -f "$YAML_FILE" > /dev/null 2> "$UPDATE2_STDERR"; then
  cat "$UPDATE2_STDERR"
  echo "FAIL: the update with hand-added assets must succeed"
  exit 1
fi
cat "$UPDATE2_STDERR"
if ! grep -q "$WARNING_PATTERN" "$UPDATE2_STDERR"; then
  echo "FAIL: expected the warning when the definition carries hand-added assets"
  exit 1
fi

echo "=== Notification channel routing.assets warning test PASSED ==="
