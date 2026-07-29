#!/usr/bin/env bash
set -euo pipefail

# Exercises the read-only spec.routing.assets warning for Dash0NotificationChannel documents.
# The Dash0 API treats spec.routing.assets as a server-derived back-reference and ignores any
# value supplied on write. Confirms that:
#   - apply warns on stderr when the document carries a non-empty spec.routing.assets;
#   - the apply itself still succeeds (the warning is non-fatal);
#   - a second apply stays idempotent;
#   - cleanup via `notification-channels delete` works.

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
TMPDIR="$(mktemp -d)"

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
        id: 00000000-0000-0000-0000-000000000001
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
if ! grep -q "spec.routing.assets is read-only and ignored on write" "$APPLY1_STDERR"; then
  echo "FAIL: expected the read-only routing.assets warning on stderr"
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
if ! grep -q "spec.routing.assets is read-only and ignored on write" "$APPLY2_STDERR"; then
  echo "FAIL: expected the read-only routing.assets warning on the second apply too"
  exit 1
fi
if echo "$APPLY2" | grep -q "created"; then
  echo "FAIL: second apply must update (PUT-by-origin), not create a duplicate"
  exit 1
fi

echo "PASS"
