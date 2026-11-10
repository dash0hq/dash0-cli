#!/usr/bin/env bash
set -euo pipefail

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
TMPDIR="$(mktemp -d)"

SUFFIX="$(uuidgen | tr '[:upper:]' '[:lower:]' | tr -d '-')"
PREVIEW_ID="test-preview-${SUFFIX}"
EPHEMERAL_ID="test-ephemeral-${SUFFIX}"

cleanup() {
  "$DASH0" dashboards delete "$PREVIEW_ID" --force >/dev/null 2>&1 || true
  "$DASH0" dashboards delete "$EPHEMERAL_ID" --force >/dev/null 2>&1 || true
  rm -rf "$TMPDIR"
}
trap cleanup EXIT

echo "=== Ephemeral dashboard round-trip test ==="

mkdir -p "${TMPDIR}/dashboards"
cat > "${TMPDIR}/dashboards/dashboard.yaml" << YAML
kind: Dashboard
metadata:
  name: preview-test
  dash0Extensions:
    id: $PREVIEW_ID
spec:
  display:
    name: Preview Test Dashboard
  layouts: []
  panels: {}
YAML

cat > "${TMPDIR}/ephemeral.yaml" << YAML
kind: EphemeralDashboard
metadata:
  name: ephemeral-test
  dash0Extensions:
    id: $EPHEMERAL_ID
spec:
  ttl: 1h
  display:
    name: Ephemeral Test Dashboard
  layouts: []
  panels: {}
YAML

echo "--- Step 1: First preview (expect: created) ---"
PREVIEW1=$("$DASH0" -X dashboards preview -f "${TMPDIR}/dashboards" --ttl 1h)
echo "$PREVIEW1"
if ! echo "$PREVIEW1" | grep -q "${PREVIEW_ID}.*created.*dashboard_id=${PREVIEW_ID}"; then
  echo "FAIL: expected '${PREVIEW_ID}' created with its URL in the first preview"
  exit 1
fi

echo "--- Step 2: Second preview (expect: updated, no duplicate) ---"
PREVIEW2=$("$DASH0" -X dashboards preview -f "${TMPDIR}/dashboards" --ttl 1h)
echo "$PREVIEW2"
if ! echo "$PREVIEW2" | grep -q "${PREVIEW_ID}.*updated"; then
  echo "FAIL: expected '${PREVIEW_ID}' updated in the second preview"
  exit 1
fi

echo "--- Step 3: The preview is an EphemeralDashboard with the ttl ---"
GET3=$("$DASH0" dashboards get "$PREVIEW_ID" -o yaml)
if ! echo "$GET3" | grep -q "^kind: EphemeralDashboard"; then
  echo "FAIL: expected kind EphemeralDashboard for '${PREVIEW_ID}'"
  echo "$GET3"
  exit 1
fi
if ! echo "$GET3" | grep -q "ttl: 1h"; then
  echo "FAIL: expected spec.ttl 1h for '${PREVIEW_ID}'"
  echo "$GET3"
  exit 1
fi

echo "--- Step 4: Apply an EphemeralDashboard twice (expect: created, then updated) ---"
APPLY4=$("$DASH0" apply -f "${TMPDIR}/ephemeral.yaml")
echo "$APPLY4"
if ! echo "$APPLY4" | grep -q "Ephemeral dashboard.*created"; then
  echo "FAIL: expected 'Ephemeral dashboard ... created' on first apply"
  exit 1
fi
APPLY5=$("$DASH0" apply -f "${TMPDIR}/ephemeral.yaml")
echo "$APPLY5"
if echo "$APPLY5" | grep -q "created"; then
  echo "FAIL: unexpected 'created' on second apply"
  exit 1
fi

echo "--- Step 5: Applying a Dashboard under the preview's id fails ---"
if "$DASH0" apply -f "${TMPDIR}/dashboards/dashboard.yaml" 2>"${TMPDIR}/kind-change.err"; then
  echo "FAIL: expected apply of a Dashboard over an EphemeralDashboard to fail"
  exit 1
fi
cat "${TMPDIR}/kind-change.err"

echo "--- Step 6: Delete removes the ephemeral dashboard ---"
"$DASH0" dashboards delete "$EPHEMERAL_ID" --force
if "$DASH0" dashboards get "$EPHEMERAL_ID" >/dev/null 2>&1; then
  echo "FAIL: expected '${EPHEMERAL_ID}' to be gone after delete"
  exit 1
fi

echo "=== Ephemeral dashboard round-trip test PASSED ==="
