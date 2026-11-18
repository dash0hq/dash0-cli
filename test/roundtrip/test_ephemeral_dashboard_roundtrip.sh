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

echo "--- Step 6: The kind of an existing dashboard does not change ---"
cat > "${TMPDIR}/kind-change.yaml" << YAML
kind: EphemeralDashboard
metadata:
  name: preview-test
  dash0Extensions:
    id: $DASHBOARD_ID
spec:
  ttl: 1h
  display:
    name: Preview Test Dashboard
  layouts: []
  panels: {}
YAML
if APPLY7=$("$DASH0" apply -f "${TMPDIR}/kind-change.yaml" 2>&1); then
  echo "FAIL: expected apply of an EphemeralDashboard over the Dashboard '${DASHBOARD_ID}' to fail"
  echo "$APPLY7"
  exit 1
fi
if ! echo "$APPLY7" | grep -q "is stored as dashboard"; then
  echo "FAIL: expected the kind mismatch error, got:"
  echo "$APPLY7"
  exit 1
fi
# The CLI stops before the write, so send the same PUT directly to check that the API rejects it too.
cat > "${TMPDIR}/kind-change.json" << JSON
{"kind": "EphemeralDashboard", "metadata": {"name": "preview-test"}, "spec": {"ttl": "1h", "display": {"name": "Preview Test Dashboard"}, "layouts": [], "panels": {}}}
JSON
if PUT7=$("$DASH0" -X api PUT "/api/dashboards/${DASHBOARD_ID}" -f "${TMPDIR}/kind-change.json" 2>&1); then
  echo "FAIL: expected the API to reject an EphemeralDashboard over the Dashboard '${DASHBOARD_ID}'"
  echo "$PUT7"
  exit 1
fi
echo "$PUT7"
if ! "$DASH0" dashboards get "$DASHBOARD_ID" -o yaml | grep -q "^kind: Dashboard"; then
  echo "FAIL: expected '${DASHBOARD_ID}' to stay a permanent Dashboard"
  exit 1
fi

echo "--- Step 7: Delete removes the ephemeral dashboard ---"
"$DASH0" dashboards delete "$EPHEMERAL_ID" --force
# get also returns a deleted dashboard, so check that list no longer has it.
if ! LIST_JSON=$("$DASH0" dashboards list --all -o json); then
  echo "FAIL: dashboards list -o json failed"
  exit 1
fi
if echo "$LIST_JSON" | jq -e --arg id "$EPHEMERAL_ID" '.[] | select(.metadata.dash0Extensions.id == $id or .metadata.labels["dash0.com/origin"] == $id)' > /dev/null 2>&1; then
  echo "FAIL: expected '${EPHEMERAL_ID}' to be gone after delete"
  exit 1
fi

echo "=== Ephemeral dashboard round-trip test PASSED ==="
