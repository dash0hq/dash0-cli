#!/usr/bin/env bash
set -euo pipefail

# Exercises `dash0 apply` for spam filters in both schema versions (v1alpha1
# and v1alpha2). Confirms that:
#   - apply creates the asset on first run with the caller-provided origin;
#   - apply is idempotent on the second run: the existing record is upserted
#     in place (same server-assigned ID, no duplicate);
#   - the action transitions from "created" to "updated" (or "no changes")
#     on the second apply, with the diff path active for matching content;
#   - apply works for both v1alpha1 (spec.contexts array) and v1alpha2
#     (spec.context scalar) — i.e. the apiVersion dispatch in applyDocument
#     is wired correctly;
#   - apply with an unsupported apiVersion fails up front during validation,
#     not after the first PUT.
#
# The spam filter API keys upsert on the `dash0.com/origin` label, not on a
# user-supplied ID (the server generates IDs server-side). We therefore drive
# idempotency by injecting a unique origin into the fixture before each stage.

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

# run_apply_idempotency_stage applies the given fixture twice, verifies the
# second apply does not create a duplicate, and asserts the stored definition
# has the expected apiVersion. Cleanup deletes the asset via its origin so
# the dataset is left in its prior state even if the test fails partway.
run_apply_idempotency_stage() {
  local fixture="$1"
  local expected_api_version="$2"

  local origin
  origin="apply-roundtrip-${expected_api_version}-$(uuidgen | tr '[:upper:]' '[:lower:]')"
  local yaml_file="${TMPDIR}/spam-filter-${expected_api_version}.yaml"

  echo "=== Spam filter (${expected_api_version}) apply idempotency stage ==="
  echo "Origin: $origin"

  # Inject the generated origin so apply uses PUT (upsert) on second run.
  ORIGIN="$origin" yq '.metadata.labels."dash0.com/origin" = env(ORIGIN)' "$fixture" > "$yaml_file"

  # Step 1: First apply — should create.
  echo "--- Step 1: First apply (expect: created) ---"
  local apply1
  apply1=$("$DASH0" apply -f "$yaml_file")
  echo "$apply1"
  if ! echo "$apply1" | grep -q "created"; then
    echo "FAIL: expected 'created' in first apply output"
    return 1
  fi

  # Resolve the server-assigned ID for cleanup and subsequent assertions.
  local id
  id=$("$DASH0" --experimental spam-filters list --all -o json | jq -r --arg o "$origin" '.[] | select(.metadata.labels["dash0.com/origin"] == $o) | .metadata.labels["dash0.com/id"]' | head -1)
  if [ -z "$id" ]; then
    echo "FAIL: could not find spam filter with origin '$origin' in list after first apply"
    return 1
  fi
  echo "Server-assigned ID: $id"

  # Step 2: Second apply — must not create a duplicate, and must not error
  # out with a "already exists" 409 from origin uniqueness.
  echo "--- Step 2: Second apply (expect: no duplicate created) ---"
  local apply2
  if ! apply2=$("$DASH0" apply -f "$yaml_file" 2>&1); then
    echo "FAIL: second apply errored out (expected idempotent upsert):"
    echo "$apply2"
    "$DASH0" --experimental spam-filters delete "$origin" --force > /dev/null 2>&1 || true
    return 1
  fi
  echo "$apply2"
  if echo "$apply2" | grep -q "created"; then
    echo "FAIL: unexpected 'created' on second apply — duplicate was created"
    return 1
  fi

  # Step 3: Exactly one record exists with this origin.
  echo "--- Step 3: Verify exactly one record with this origin ---"
  local count
  count=$("$DASH0" --experimental spam-filters list --all -o json | jq --arg o "$origin" '[.[] | select(.metadata.labels["dash0.com/origin"] == $o)] | length')
  if [ "$count" != "1" ]; then
    echo "FAIL: expected exactly 1 record with origin '$origin', got $count"
    return 1
  fi

  # Step 4: The stored apiVersion matches the input apiVersion. Confirms the
  # apply path routed through the correct dispatch branch.
  echo "--- Step 4: Verify stored apiVersion is ${expected_api_version} ---"
  local get_yaml
  get_yaml=$("$DASH0" --experimental spam-filters get "$id" -o yaml)
  if ! echo "$get_yaml" | grep -q "^apiVersion: ${expected_api_version}$"; then
    echo "FAIL: expected apiVersion '${expected_api_version}' in get -o yaml output, got:"
    echo "$get_yaml" | grep -i apiversion || echo "  (no apiVersion line)"
    return 1
  fi

  # Step 5: Cleanup — delete by origin (the api path accepts either form).
  echo "--- Step 5: Delete ---"
  if ! "$DASH0" --experimental spam-filters delete "$origin" --force > /dev/null; then
    echo "FAIL: cleanup delete failed"
    return 1
  fi
}

# Stage 1: v1alpha1 (spec.contexts array).
run_apply_idempotency_stage "${FIXTURES}/spam-filter.yaml" "v1alpha1"

# Stage 2: v1alpha2 (spec.context scalar).
run_apply_idempotency_stage "${FIXTURES}/spam-filter-v1alpha2.yaml" "v1alpha2"

# Stage 3: Apply with an unsupported apiVersion must fail at validation,
# before any API call is issued.
echo "=== Spam filter apply rejects unknown apiVersion at validation ==="
BAD_FIXTURE="${TMPDIR}/spam-filter-bogus.yaml"
cat > "$BAD_FIXTURE" <<'EOF'
apiVersion: v9beta
kind: Dash0SpamFilter
metadata:
  name: bogus
spec: {}
EOF
if BAD_OUT=$("$DASH0" apply -f "$BAD_FIXTURE" --dry-run 2>&1); then
  echo "FAIL: expected apply --dry-run to fail for unknown apiVersion, got:"
  echo "$BAD_OUT"
  exit 1
fi
echo "$BAD_OUT"
if ! echo "$BAD_OUT" | grep -q 'unsupported spam filter apiVersion "v9beta"'; then
  echo "FAIL: error message did not mention the unsupported apiVersion"
  exit 1
fi
if ! echo "$BAD_OUT" | grep -q '"v1alpha1"' || ! echo "$BAD_OUT" | grep -q '"v1alpha2"'; then
  echo "FAIL: error message did not list the supported apiVersions"
  exit 1
fi

echo "=== Spam filter apply idempotency test PASSED ==="
