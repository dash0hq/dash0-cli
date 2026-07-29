#!/usr/bin/env bash
set -euo pipefail

# Exercises the API-managed spec.routing.assets warning for Dash0NotificationChannel documents.
# The Dash0 API treats spec.routing.assets as a server-derived back-reference and ignores any
# value supplied on write. Confirms that:
#   - apply warns on stderr when the document carries a non-empty spec.routing.assets;
#   - the apply itself still succeeds (the warning is non-fatal);
#   - a second apply stays idempotent;
#   - the fabricated asset entry is NOT persisted server-side;
#   - `notification-channels update` warns only when the file's assets differ from the server's
#     (a get -> edit -> update roundtrip stays silent);
#   - cleanup via `notification-channels delete` works.

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
TMPDIR="$(mktemp -d)"

WARNING_PATTERN="spec.routing.assets is API-managed and ignored on write"
FABRICATED_ID="00000000-0000-0000-0000-000000000001"

ORIGIN="apply-assets-warning-channel-$(uuidgen | tr '[:upper:]' '[:lower:]')"
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

# Step 3: The fabricated asset entry must not have been persisted — the API ignores the field.
echo "--- Step 3: Fabricated asset not persisted server-side ---"
SERVER_ASSETS=$("$DASH0" -X notification-channels get "$ORIGIN" -o json | jq -c '.spec.routing.assets')
echo "Server assets: $SERVER_ASSETS"
if echo "$SERVER_ASSETS" | grep -q "$FABRICATED_ID"; then
  echo "FAIL: the fabricated asset entry was persisted — the API must ignore spec.routing.assets on write"
  exit 1
fi

# Step 4: get -> update roundtrip must stay silent — the file carries the server's own assets.
echo "--- Step 4: get -> update roundtrip (expect: no warning) ---"
ROUNDTRIP_FILE="${TMPDIR}/roundtrip.yaml"
"$DASH0" -X notification-channels get "$ORIGIN" -o yaml > "$ROUNDTRIP_FILE"
UPDATE1_STDERR="${TMPDIR}/update1.stderr"
"$DASH0" -X notification-channels update "$ORIGIN" -f "$ROUNDTRIP_FILE" > /dev/null 2> "$UPDATE1_STDERR"
cat "$UPDATE1_STDERR"
if grep -q "$WARNING_PATTERN" "$UPDATE1_STDERR"; then
  echo "FAIL: a get -> update roundtrip must not warn — the assets match what the server reports"
  exit 1
fi

# Step 5: update with assets differing from the server's — expect the warning.
echo "--- Step 5: update with differing assets (expect: warning) ---"
UPDATE2_STDERR="${TMPDIR}/update2.stderr"
"$DASH0" -X notification-channels update "$ORIGIN" -f "$YAML_FILE" > /dev/null 2> "$UPDATE2_STDERR"
cat "$UPDATE2_STDERR"
if ! grep -q "$WARNING_PATTERN" "$UPDATE2_STDERR"; then
  echo "FAIL: expected the warning when the file's assets differ from the server's"
  exit 1
fi

echo "=== Notification channel routing.assets warning test PASSED ==="
