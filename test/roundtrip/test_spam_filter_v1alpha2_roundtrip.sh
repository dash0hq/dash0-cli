#!/usr/bin/env bash
set -euo pipefail

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
FIXTURE="${FIXTURES}/spam-filter-v1alpha2.yaml"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

ASSET_NAME=$(yq '.metadata.name' "$FIXTURE")

echo "=== Spam filter v1alpha2 round-trip test ==="
echo "Asset name: $ASSET_NAME"

# Step 1: Create from v1alpha2 fixture
echo "--- Step 1: Create spam filter from v1alpha2 fixture ---"
if ! CREATE_OUTPUT=$("$DASH0" --experimental spam-filters create -f "$FIXTURE"); then
  echo "FAIL: spam-filters create failed"
  exit 1
fi
echo "$CREATE_OUTPUT"
if ! echo "$CREATE_OUTPUT" | grep -q "$ASSET_NAME"; then
  echo "FAIL: create output does not mention asset name '$ASSET_NAME'"
  exit 1
fi

# Step 2: List spam filters and find the created asset by name.
# Note: the list endpoint returns v1alpha1-shaped items even for filters that
# were created via v1alpha2. We only need the ID here, which lives in the
# same metadata.labels location in both schemas.
echo "--- Step 2: List spam filters and find created asset ---"
if ! LIST_JSON=$("$DASH0" --experimental spam-filters list --all -o json); then
  echo "FAIL: spam-filters list -o json failed"
  exit 1
fi
ID=$(echo "$LIST_JSON" | jq -r --arg name "$ASSET_NAME" '[.[] | select(.metadata.name == $name)][0].metadata.labels["dash0.com/id"] // empty')
if [ -z "$ID" ]; then
  echo "FAIL: Could not find created spam filter '$ASSET_NAME' in list"
  exit 1
fi
echo "Created spam filter ID: $ID"

# Step 3: Get by ID — verify the server returned a v1alpha2 representation
# (apiVersion: v1alpha2 and spec.context as a scalar, not spec.contexts).
echo "--- Step 3: Get spam filter by ID and verify it is returned as v1alpha2 ---"
if ! GET_YAML=$("$DASH0" --experimental spam-filters get "$ID" -o yaml); then
  echo "FAIL: spam-filters get -o yaml failed"
  exit 1
fi
echo "$GET_YAML"
if ! echo "$GET_YAML" | grep -q "^apiVersion: v1alpha2$"; then
  echo "FAIL: server did not return apiVersion v1alpha2; got:"
  echo "$GET_YAML" | grep -i apiversion || echo "  (no apiVersion line)"
  exit 1
fi
if ! echo "$GET_YAML" | grep -qE "^\s*context:\s*log\s*$"; then
  echo "FAIL: spec.context (scalar) missing or wrong; expected 'context: log'"
  exit 1
fi
if echo "$GET_YAML" | grep -qE "^\s*contexts:"; then
  echo "FAIL: server returned v1alpha1 'contexts:' array for a v1alpha2 filter"
  exit 1
fi

# Step 4: Default-format get prints a single "Context:" line (not "Contexts:").
echo "--- Step 4: Default-format get prints v1alpha2 'Context:' line ---"
if ! GET_DEFAULT=$("$DASH0" --experimental spam-filters get "$ID"); then
  echo "FAIL: spam-filters get failed"
  exit 1
fi
echo "$GET_DEFAULT"
if ! echo "$GET_DEFAULT" | grep -qE "^Context: log$"; then
  echo "FAIL: default get output missing 'Context: log' line"
  exit 1
fi
if echo "$GET_DEFAULT" | grep -qE "^Contexts:"; then
  echo "FAIL: default get output should not include v1alpha1 'Contexts:' line"
  exit 1
fi

# Step 5: Export to YAML and re-import via update — verifies the v1alpha2 PUT path.
echo "--- Step 5: Export v1alpha2 YAML and re-import via update ---"
"$DASH0" --experimental spam-filters get "$ID" -o yaml > "${TMPDIR}/exported.yaml"
if ! grep -q "^apiVersion: v1alpha2$" "${TMPDIR}/exported.yaml"; then
  echo "FAIL: exported YAML is not v1alpha2"
  exit 1
fi
if ! "$DASH0" --experimental spam-filters update "$ID" -f "${TMPDIR}/exported.yaml"; then
  echo "FAIL: spam-filters update failed"
  exit 1
fi

# Step 6: Update without an explicit ID arg — ID must be taken from the file's
# dash0.com/id label (exported in Step 5).
echo "--- Step 6: Update using ID embedded in exported YAML ---"
if ! "$DASH0" --experimental spam-filters update -f "${TMPDIR}/exported.yaml"; then
  echo "FAIL: spam-filters update (ID from file) failed"
  exit 1
fi

# Step 7: Delete (version-agnostic endpoint).
echo "--- Step 7: Delete spam filter ---"
if ! "$DASH0" --experimental spam-filters delete "$ID" --force; then
  echo "FAIL: spam-filters delete failed"
  exit 1
fi

# Step 8: Verify deletion.
echo "--- Step 8: Verify deletion ---"
if ! LIST_JSON=$("$DASH0" --experimental spam-filters list --all -o json); then
  echo "FAIL: spam-filters list -o json failed"
  exit 1
fi
if echo "$LIST_JSON" | jq -e --arg id "$ID" '.[] | select(.metadata.labels["dash0.com/id"] == $id)' > /dev/null 2>&1; then
  echo "FAIL: Spam filter '$ID' still exists after deletion"
  exit 1
fi

echo "=== Spam filter v1alpha2 round-trip test PASSED ==="
