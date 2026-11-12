#!/usr/bin/env bash
set -euo pipefail

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
TMPDIR="$(mktemp -d)"

SUFFIX="$(uuidgen | tr '[:upper:]' '[:lower:]' | tr -d '-')"
DASHBOARD_ID="test-preview-${SUFFIX}"
PREVIEW_ID=""
EPHEMERAL_ID="test-ephemeral-${SUFFIX}"

cleanup() {
  if [ -n "$PREVIEW_ID" ]; then
    "$DASH0" dashboards delete "$PREVIEW_ID" --force >/dev/null 2>&1 || true
  fi
  "$DASH0" dashboards delete "$DASHBOARD_ID" --force >/dev/null 2>&1 || true
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
    id: $DASHBOARD_ID
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

echo "--- Step 1: First preview (expect: created under a derived id) ---"
PREVIEW1=$("$DASH0" -X dashboards preview -f "${TMPDIR}/dashboards" --ttl 1h -o json)
echo "$PREVIEW1"
PREVIEW_ID=$(echo "$PREVIEW1" | jq -r '.[0].id')
if [ -z "$PREVIEW_ID" ] || [ "$PREVIEW_ID" = "null" ] || [ "$PREVIEW_ID" = "$DASHBOARD_ID" ]; then
  echo "FAIL: expected a preview id other than '${DASHBOARD_ID}', got '${PREVIEW_ID}'"
  exit 1
fi
if [ "$(echo "$PREVIEW1" | jq -r '.[0].action')" != "created" ]; then
  echo "FAIL: expected the first preview to be created"
  exit 1
fi
if ! echo "$PREVIEW1" | jq -r '.[0].url' | grep -q "dashboard_id=${PREVIEW_ID}"; then
  echo "FAIL: expected the preview URL to point at '${PREVIEW_ID}'"
  exit 1
fi

echo "--- Step 2: Second preview (expect: same id, updated) ---"
PREVIEW2=$("$DASH0" -X dashboards preview -f "${TMPDIR}/dashboards" --ttl 1h -o json)
echo "$PREVIEW2"
if [ "$(echo "$PREVIEW2" | jq -r '.[0].id')" != "$PREVIEW_ID" ] || [ "$(echo "$PREVIEW2" | jq -r '.[0].action')" != "updated" ]; then
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

echo "--- Step 5: Applying the previewed Dashboard works while its preview exists ---"
APPLY6=$("$DASH0" apply -f "${TMPDIR}/dashboards/dashboard.yaml")
echo "$APPLY6"
if ! echo "$APPLY6" | grep -q "created"; then
  echo "FAIL: expected the Dashboard '${DASHBOARD_ID}' to be created next to its preview"
  exit 1
fi
if ! "$DASH0" dashboards get "$DASHBOARD_ID" -o yaml | grep -q "^kind: Dashboard"; then
  echo "FAIL: expected '${DASHBOARD_ID}' to be a permanent Dashboard"
  exit 1
fi
if ! "$DASH0" dashboards get "$PREVIEW_ID" -o yaml | grep -q "^kind: EphemeralDashboard"; then
  echo "FAIL: expected the preview '${PREVIEW_ID}' to stay an EphemeralDashboard"
  exit 1
fi

echo "--- Step 6: Delete removes the ephemeral dashboard ---"
"$DASH0" dashboards delete "$EPHEMERAL_ID" --force
if "$DASH0" dashboards get "$EPHEMERAL_ID" >/dev/null 2>&1; then
  echo "FAIL: expected '${EPHEMERAL_ID}' to be gone after delete"
  exit 1
fi

echo "=== Ephemeral dashboard round-trip test PASSED ==="
