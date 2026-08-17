#!/usr/bin/env bash
set -euo pipefail

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

SUFFIX="$(uuidgen | tr '[:upper:]' '[:lower:]' | tr -d '-' | cut -c1-12)"

echo "=== apply --since round-trip test (suffix: $SUFFIX) ==="

# NOTE: a PrometheusRule alerting-rule partial-removal scenario is
# deliberately not covered here -- see
# https://github.com/dash0hq/dash0-cli/issues/254. A CRD with multiple
# alerts sharing one dash0.com/id never produces more than one live check
# rule via ordinary create/apply (each alert's PUT overwrites the previous
# one under that shared id), so the "one alert removed while another
# survives as its own check rule" scenario --since's alert-partial-removal
# detection is designed for cannot be constructed against the real API.

git_repo() {
  git -C "$TMPDIR" "$@"
}

git init -q -b main "$TMPDIR"
git_repo config user.email "roundtrip-test@example.com"
git_repo config user.name "Roundtrip Test"
git_repo config commit.gpgsign false

# ---------------------------------------------------------------------------
# Scenario A: whole-file deletion
# ---------------------------------------------------------------------------
DASHBOARD_ORIGIN="since-rt-dashboard-${SUFFIX}"
cat > "${TMPDIR}/keep-dashboard.yaml" << YAML
apiVersion: dash0.com/v1alpha1
kind: Dashboard
metadata:
  name: keep-dashboard-${SUFFIX}
  dash0extensions:
    id: ${DASHBOARD_ORIGIN}
spec:
  display:
    name: Since RT Keep Dashboard ${SUFFIX}
  layouts: []
  panels: {}
YAML

REMOVE_DASHBOARD_ORIGIN="since-rt-remove-dashboard-${SUFFIX}"
cat > "${TMPDIR}/remove-dashboard.yaml" << YAML
apiVersion: dash0.com/v1alpha1
kind: Dashboard
metadata:
  name: remove-dashboard-${SUFFIX}
  dash0extensions:
    id: ${REMOVE_DASHBOARD_ORIGIN}
spec:
  display:
    name: Since RT Remove Dashboard ${SUFFIX}
  layouts: []
  panels: {}
YAML

# ---------------------------------------------------------------------------
# Scenario B: multi-document partial deletion (a View survives, a CheckRule
# in the same file is removed).
# ---------------------------------------------------------------------------
VIEW_ID="since-rt-view-${SUFFIX}"
CHECKRULE_ID="since-rt-checkrule-${SUFFIX}"
cat > "${TMPDIR}/multi-doc.yaml" << YAML
apiVersion: dash0.com/v1alpha1
kind: View
metadata:
  name: since-rt-view-${SUFFIX}
  labels:
    dash0.com/id: ${VIEW_ID}
spec:
  display:
    name: Since RT View ${SUFFIX}
  type: logs
---
apiVersion: dash0.com/v1alpha1
kind: CheckRule
id: ${CHECKRULE_ID}
name: Since RT Check Rule ${SUFFIX}
expression: up == 0
YAML

echo "--- Step 1: Create the 'before' commit (all assets present) ---"
git_repo add -A
git_repo commit -q -m "add before state"
BEFORE_SHA=$(git_repo rev-parse HEAD)
echo "before SHA: $BEFORE_SHA"

echo "--- Step 2: Apply the 'before' state so the assets actually exist ---"
APPLY_BEFORE=$("$DASH0" apply -f "$TMPDIR")
echo "$APPLY_BEFORE"
if ! echo "$APPLY_BEFORE" | grep -q "created"; then
  echo "FAIL: expected 'created' when applying the before state"
  exit 1
fi

echo "--- Step 3: Mutate the working tree (remove one asset per scenario) ---"
rm "${TMPDIR}/remove-dashboard.yaml"
cat > "${TMPDIR}/multi-doc.yaml" << YAML
apiVersion: dash0.com/v1alpha1
kind: View
metadata:
  name: since-rt-view-${SUFFIX}
  labels:
    dash0.com/id: ${VIEW_ID}
spec:
  display:
    name: Since RT View ${SUFFIX}
  type: logs
YAML
git_repo add -A
git_repo commit -q -m "remove dashboard and checkrule document"

echo "--- Step 4: apply --since (expect 2 deletions) ---"
SINCE_OUTPUT=$("$DASH0" --experimental apply -f "$TMPDIR" --since "$BEFORE_SHA" --force)
echo "$SINCE_OUTPUT"

FAIL=0
if ! echo "$SINCE_OUTPUT" | grep -q "$REMOVE_DASHBOARD_ORIGIN"; then
  echo "FAIL: whole-file dashboard deletion did not mention its origin"
  FAIL=1
fi
if ! echo "$SINCE_OUTPUT" | grep -q "$CHECKRULE_ID"; then
  echo "FAIL: multi-document check-rule deletion did not mention its id"
  FAIL=1
fi
if [ "$FAIL" -ne 0 ]; then
  exit 1
fi

echo "--- Step 5: verify the removed assets are gone ---"
# Both dashboards and check rules are soft-deleted server-side: `get` by id
# keeps returning the record, so absence from `list --all` is the reliable
# deletion signal (matches the other roundtrip tests' convention).
if "$DASH0" dashboards list --all -o json | jq -e --arg id "$REMOVE_DASHBOARD_ORIGIN" '.[] | select(.metadata.dash0Extensions.id == $id)' > /dev/null 2>&1; then
  echo "FAIL: dashboard '$REMOVE_DASHBOARD_ORIGIN' still exists after --since deletion"
  FAIL=1
fi
if "$DASH0" check-rules list --all -o json | jq -e --arg id "$CHECKRULE_ID" '.[] | select(.id == $id)' > /dev/null 2>&1; then
  echo "FAIL: check rule '$CHECKRULE_ID' still exists after --since deletion"
  FAIL=1
fi
if [ "$FAIL" -ne 0 ]; then
  exit 1
fi

echo "--- Step 6: verify the surviving assets are still present ---"
if ! "$DASH0" dashboards get "$DASHBOARD_ORIGIN" > /dev/null 2>&1; then
  echo "FAIL: surviving dashboard '$DASHBOARD_ORIGIN' is missing"
  FAIL=1
fi
if ! "$DASH0" views get "$VIEW_ID" > /dev/null 2>&1; then
  echo "FAIL: surviving view '$VIEW_ID' is missing"
  FAIL=1
fi
if [ "$FAIL" -ne 0 ]; then
  exit 1
fi

echo "--- Cleanup: delete the surviving assets ---"
"$DASH0" dashboards delete "$DASHBOARD_ORIGIN" --force || true
"$DASH0" views delete "$VIEW_ID" --force || true

echo "=== apply --since round-trip test PASSED ==="
