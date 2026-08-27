#!/usr/bin/env bash
set -euo pipefail

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

SUFFIX="$(uuidgen | tr '[:upper:]' '[:lower:]' | tr -d '-' | cut -c1-12)"
DASHBOARD_ORIGIN="since-idem-dashboard-${SUFFIX}"
REMOVE_ORIGIN="since-idem-remove-${SUFFIX}"

echo "=== apply --since idempotency test (suffix: $SUFFIX) ==="

git_repo() {
  git -C "$TMPDIR" "$@"
}

git init -q -b main "$TMPDIR"
git_repo config user.email "roundtrip-test@example.com"
git_repo config user.name "Roundtrip Test"
git_repo config commit.gpgsign false

cat > "${TMPDIR}/keep.yaml" << YAML
apiVersion: dash0.com/v1alpha1
kind: Dashboard
metadata:
  name: keep-${SUFFIX}
  dash0extensions:
    id: ${DASHBOARD_ORIGIN}
spec:
  display:
    name: Since Idempotency Keep ${SUFFIX}
  layouts: []
  panels: {}
YAML

cat > "${TMPDIR}/remove.yaml" << YAML
apiVersion: dash0.com/v1alpha1
kind: Dashboard
metadata:
  name: remove-${SUFFIX}
  dash0extensions:
    id: ${REMOVE_ORIGIN}
spec:
  display:
    name: Since Idempotency Remove ${SUFFIX}
  layouts: []
  panels: {}
YAML

echo "--- Step 1: create the 'before' state ---"
git_repo add -A
git_repo commit -q -m "before"
BEFORE_SHA=$(git_repo rev-parse HEAD)
"$DASH0" apply -f "$TMPDIR" > /dev/null

echo "--- Step 2: remove one dashboard, commit ---"
rm "${TMPDIR}/remove.yaml"
git_repo add -A
git_repo commit -q -m "remove one dashboard"
FIRST_SINCE_SHA=$(git_repo rev-parse HEAD)

echo "--- Step 3: first apply --since (expect: one deletion) ---"
FIRST_OUTPUT=$("$DASH0" --experimental apply -f "$TMPDIR" --since "$BEFORE_SHA" --force)
echo "$FIRST_OUTPUT"
if ! echo "$FIRST_OUTPUT" | grep -q "$REMOVE_ORIGIN"; then
  echo "FAIL: first apply --since did not delete '$REMOVE_ORIGIN'"
  exit 1
fi

echo "--- Step 4: second apply --since against the new baseline (expect: no deletions) ---"
# Nothing changed between FIRST_SINCE_SHA and now -- the second run must not
# error on an already-gone asset or repeat the deletion.
SECOND_OUTPUT=$("$DASH0" --experimental apply -f "$TMPDIR" --since "$FIRST_SINCE_SHA" --force)
echo "$SECOND_OUTPUT"
if echo "$SECOND_OUTPUT" | grep -qi "deleted"; then
  echo "FAIL: second apply --since reported a deletion; expected none"
  exit 1
fi

echo "--- Step 5: verify the removed dashboard stays gone ---"
if "$DASH0" dashboards list --all -o json | jq -e --arg id "$REMOVE_ORIGIN" '.[] | select(.metadata.dash0Extensions.id == $id)' > /dev/null 2>&1; then
  echo "FAIL: dashboard '$REMOVE_ORIGIN' reappeared"
  exit 1
fi

echo "--- Cleanup ---"
"$DASH0" dashboards delete "$DASHBOARD_ORIGIN" --force || true

echo "=== apply --since idempotency test PASSED ==="
