#!/usr/bin/env bash
set -euo pipefail

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

SUFFIX="$(uuidgen | tr '[:upper:]' '[:lower:]' | tr -d '-' | cut -c1-12)"
DASHBOARD_ORIGIN="since-edge-dashboard-${SUFFIX}"

echo "=== apply --since ref edge cases test (suffix: $SUFFIX) ==="

git_repo() {
  git -C "$TMPDIR" "$@"
}

git init -q -b main "$TMPDIR"
git_repo config user.email "roundtrip-test@example.com"
git_repo config user.name "Roundtrip Test"
git_repo config commit.gpgsign false

cat > "${TMPDIR}/dashboard.yaml" << YAML
apiVersion: dash0.com/v1alpha1
kind: Dashboard
metadata:
  name: edge-${SUFFIX}
  dash0extensions:
    id: ${DASHBOARD_ORIGIN}
spec:
  display:
    name: Since Edge Case ${SUFFIX}
  layouts: []
  panels: {}
YAML
git_repo add -A
git_repo commit -q -m "initial commit"

echo "--- Scenario 1: all-zeros sentinel (first push to a new branch) ---"
ZEROS="0000000000000000000000000000000000000000"
if OUTPUT=$("$DASH0" --experimental apply -f "$TMPDIR" --since "$ZEROS" --dry-run 2>&1); then
  echo "FAIL: apply --since <all-zeros> --dry-run should have failed"
  exit 1
fi
echo "$OUTPUT"
if ! echo "$OUTPUT" | grep -qi "all-zeros"; then
  echo "FAIL: expected the all-zeros-specific error message"
  exit 1
fi

echo "--- Scenario 2: non-ancestor ref (simulated force-push) ---"
cat > "${TMPDIR}/view.yaml" << YAML
apiVersion: dash0.com/v1alpha1
kind: View
metadata:
  name: designated-ref-marker-${SUFFIX}
  labels:
    dash0.com/id: since-edge-marker-${SUFFIX}
spec:
  display:
    name: Since Edge Marker ${SUFFIX}
  type: logs
YAML
git_repo add -A
git_repo commit -q -m "add designated ref commit"
DESIGNATED_REF=$(git_repo rev-parse HEAD)

# Simulate a force-push: hard-reset back to the initial commit, then commit
# again, orphaning DESIGNATED_REF (still resolvable by SHA, not an ancestor).
FIRST_COMMIT=$(git_repo log --oneline | tail -1 | awk '{print $1}')
git_repo reset -q --hard "$FIRST_COMMIT"
cat > "${TMPDIR}/other.yaml" << YAML
apiVersion: dash0.com/v1alpha1
kind: View
metadata:
  name: other-view-${SUFFIX}
  labels:
    dash0.com/id: since-edge-other-${SUFFIX}
spec:
  display:
    name: Since Edge Other ${SUFFIX}
  type: logs
YAML
git_repo add -A
git_repo commit -q -m "post-force-push commit"

if git_repo merge-base --is-ancestor "$DESIGNATED_REF" HEAD; then
  echo "FAIL: test setup broken -- designated ref is still an ancestor of HEAD"
  exit 1
fi

echo "--- Scenario 2a: no --force, no terminal (expect: creates/updates still apply, only the deletion phase hard-fails) ---"
if OUTPUT=$("$DASH0" --experimental apply -f "$TMPDIR" --since "$DESIGNATED_REF" < /dev/null 2>&1); then
  echo "FAIL: apply --since <non-ancestor> with no terminal and no --force should have failed"
  exit 1
fi
echo "$OUTPUT"
if ! echo "$OUTPUT" | grep -qi "not an ancestor"; then
  echo "FAIL: expected the non-ancestor warning/error"
  exit 1
fi

echo "--- Scenario 2b: --force bypasses the confirmation prompt ---"
FORCE_OUTPUT=$("$DASH0" --experimental apply -f "$TMPDIR" --since "$DESIGNATED_REF" --force 2>&1)
echo "$FORCE_OUTPUT"
if ! echo "$FORCE_OUTPUT" | grep -qi "not an ancestor"; then
  echo "FAIL: expected the non-ancestor warning even with --force"
  exit 1
fi
if ! echo "$FORCE_OUTPUT" | grep -q "since-edge-marker-${SUFFIX}"; then
  echo "FAIL: --force did not delete the view removed relative to the non-ancestor ref"
  exit 1
fi

echo "--- Verify the view from the orphaned ref was deleted ---"
if "$DASH0" views list --all -o json | jq -e --arg id "since-edge-marker-${SUFFIX}" '.[] | select(.metadata.labels["dash0.com/id"] == $id or .metadata.labels["dash0.com/origin"] == $id)' > /dev/null 2>&1; then
  echo "FAIL: view 'since-edge-marker-${SUFFIX}' still exists after --force deletion"
  exit 1
fi

echo "--- Cleanup ---"
"$DASH0" views delete "since-edge-other-${SUFFIX}" --force || true
"$DASH0" dashboards delete "$DASHBOARD_ORIGIN" --force || true

echo "=== apply --since ref edge cases test PASSED ==="
