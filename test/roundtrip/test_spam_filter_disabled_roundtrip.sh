#!/usr/bin/env bash
set -euo pipefail

# Exercises a disabled spam filter (dash0.com/enabled: "false") against a real
# Dash0 environment, for both v1alpha1 and v1alpha2. The API reads a write
# without the annotation as enabled, so every write path must send it:
#   - create keeps the filter disabled;
#   - list and get show it as disabled;
#   - reapplying the exported YAML, and the original file, keeps it disabled;
#   - update can enable it, and apply can disable it again.
#
# Each stage injects a unique origin, so the filter is upserted by origin and
# a leaked filter never collides with a real one. The filter is disabled for
# most of the run, so a leaked one drops no telemetry.

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
TMPDIR="$(mktemp -d)"

ORIGINS=()
cleanup() {
  for origin in "${ORIGINS[@]+"${ORIGINS[@]}"}"; do
    "$DASH0" spam-filters delete "$origin" --force > /dev/null 2>&1 || true
  done
  rm -rf "$TMPDIR"
}
trap cleanup EXIT

# assert_enabled checks the dash0.com/enabled state of the filter with the
# given origin in three places: get -o json, the get summary, and list -o json.
assert_enabled() {
  local origin="$1"
  local want="$2"
  local step="$3"

  local annotation
  annotation=$("$DASH0" spam-filters get "$origin" -o json | jq -r '.metadata.annotations["dash0.com/enabled"] // "true"')
  if [ "$annotation" != "$want" ]; then
    echo "FAIL ($step): expected dash0.com/enabled '$want' in get -o json, got '$annotation'"
    return 1
  fi

  local summary
  summary=$("$DASH0" spam-filters get "$origin")
  if ! echo "$summary" | grep -q "^Enabled: ${want}$"; then
    echo "FAIL ($step): expected 'Enabled: ${want}' in get output, got:"
    echo "$summary"
    return 1
  fi

  local listed
  listed=$("$DASH0" spam-filters list --all -o json | jq -r --arg o "$origin" '[.[] | select(.metadata.labels["dash0.com/origin"] == $o)][0].metadata.annotations["dash0.com/enabled"] // "true"')
  if [ "$listed" != "$want" ]; then
    echo "FAIL ($step): expected dash0.com/enabled '$want' in list -o json, got '$listed'"
    return 1
  fi
}

run_disabled_stage() {
  local fixture="$1"
  local api_version="$2"

  local origin
  origin="roundtrip-disabled-${api_version}-$(uuidgen | tr '[:upper:]' '[:lower:]')"
  ORIGINS+=("$origin")
  local yaml_file="${TMPDIR}/spam-filter-${api_version}.yaml"

  echo "=== Disabled spam filter (${api_version}) round-trip ==="
  echo "Origin: $origin"

  ORIGIN="$origin" yq '.metadata.labels."dash0.com/origin" = env(ORIGIN)' "$fixture" > "$yaml_file"
  if [ "$(yq '.metadata.annotations."dash0.com/enabled"' "$yaml_file")" != "false" ]; then
    echo "FAIL: fixture $fixture is not disabled"
    return 1
  fi

  echo "--- Step 1: Create the disabled filter ---"
  "$DASH0" spam-filters create -f "$yaml_file"
  assert_enabled "$origin" "false" "after create"

  echo "--- Step 2: List shows the filter as disabled ---"
  local row
  row=$("$DASH0" spam-filters list --all -o wide | grep -F "$origin" || true)
  if [ -z "$row" ]; then
    echo "FAIL: filter with origin '$origin' not found in list -o wide"
    return 1
  fi
  echo "$row"
  if ! echo "$row" | grep -qw "false"; then
    echo "FAIL: expected the ENABLED column to read 'false'"
    return 1
  fi

  echo "--- Step 3: Reapply the exported YAML (expect: still disabled) ---"
  "$DASH0" spam-filters get "$origin" -o yaml > "${TMPDIR}/exported-${api_version}.yaml"
  local reapply
  reapply=$("$DASH0" apply -f "${TMPDIR}/exported-${api_version}.yaml")
  echo "$reapply"
  if echo "$reapply" | grep -q "created"; then
    echo "FAIL: reapplying the export created a duplicate"
    return 1
  fi
  assert_enabled "$origin" "false" "after reapplying the export"

  echo "--- Step 4: Reapply the original file (expect: still disabled) ---"
  "$DASH0" apply -f "$yaml_file"
  assert_enabled "$origin" "false" "after reapplying the original file"

  echo "--- Step 5: Enable the filter with update ---"
  yq '.metadata.annotations."dash0.com/enabled" = "true"' "$yaml_file" > "${TMPDIR}/enabled-${api_version}.yaml"
  "$DASH0" spam-filters update -f "${TMPDIR}/enabled-${api_version}.yaml"
  assert_enabled "$origin" "true" "after update"

  echo "--- Step 6: Disable the filter again with apply ---"
  "$DASH0" apply -f "$yaml_file"
  assert_enabled "$origin" "false" "after apply"

  echo "--- Step 7: Delete ---"
  "$DASH0" spam-filters delete "$origin" --force
}

run_disabled_stage "${FIXTURES}/spam-filter-disabled.yaml" "v1alpha1"
run_disabled_stage "${FIXTURES}/spam-filter-v1alpha2-disabled.yaml" "v1alpha2"

echo "=== Disabled spam filter round-trip test PASSED ==="
