#!/usr/bin/env bash
set -euo pipefail

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
FIXTURE_TEMPLATE="${FIXTURES}/team.yaml"
TMPDIR="$(mktemp -d)"
UNIQUE_ID="roundtrip-test-$(date +%s)-$$"
ORIGIN="cli-roundtrip-${UNIQUE_ID}"
DISPLAY_NAME="CLI Roundtrip Team ${UNIQUE_ID}"
UPDATED_DISPLAY_NAME="${DISPLAY_NAME} (edited)"
FIXTURE="${TMPDIR}/team.yaml"
TEAM_ID=""

cleanup() {
  if [ -n "$TEAM_ID" ]; then
    echo "--- Cleanup: deleting team $TEAM_ID ---"
    "$DASH0" -X teams delete "$TEAM_ID" --force 2>/dev/null || true
  fi
  rm -rf "$TMPDIR"
}
trap cleanup EXIT

echo "=== Team declarative round-trip test ==="
echo "Origin: $ORIGIN"
echo "Display name: $DISPLAY_NAME"

# Step 0: Declarative team management needs at least two members: one that
# stays across the edit, one that gets dropped.
echo "--- Step 0: Discover organization members ---"
MEMBERS_JSON=$("$DASH0" -X members list -o json)
MEMBER_COUNT=$(echo "$MEMBERS_JSON" | jq 'length')
if [ "$MEMBER_COUNT" -lt 2 ]; then
  echo "SKIP: need at least 2 organization members, found $MEMBER_COUNT"
  exit 0
fi
MEMBER_EMAIL_1=$(echo "$MEMBERS_JSON" | jq -r '.[0].spec.display.email // empty')
MEMBER_EMAIL_2=$(echo "$MEMBERS_JSON" | jq -r '.[1].spec.display.email // empty')
if [ -z "$MEMBER_EMAIL_1" ] || [ -z "$MEMBER_EMAIL_2" ]; then
  echo "SKIP: first two organization members are missing email addresses"
  exit 0
fi
echo "Using members: $MEMBER_EMAIL_1, $MEMBER_EMAIL_2"

# Render the fixture template with the resolved emails and unique origin.
sed \
  -e "s|__ORIGIN__|${ORIGIN}|" \
  -e "s|__DISPLAY_NAME__|${DISPLAY_NAME}|" \
  -e "s|__MEMBER_EMAIL_1__|${MEMBER_EMAIL_1}|" \
  -e "s|__MEMBER_EMAIL_2__|${MEMBER_EMAIL_2}|" \
  "$FIXTURE_TEMPLATE" > "$FIXTURE"

# Step 1: Declarative create from fixture. The fixture carries a
# dash0.com/origin label, so the CLI upserts (PUT-by-origin) rather than
# creates via POST.
echo "--- Step 1: Create team from fixture (declarative) ---"
if ! CREATE_OUTPUT=$("$DASH0" -X teams create -f "$FIXTURE"); then
  echo "FAIL: teams create -f failed"
  exit 1
fi
echo "$CREATE_OUTPUT"
if ! echo "$CREATE_OUTPUT" | grep -q "$DISPLAY_NAME"; then
  echo "FAIL: create output does not mention display name '$DISPLAY_NAME'"
  exit 1
fi

# Step 2: Locate the team by origin.
echo "--- Step 2: Locate the created team by origin ---"
LIST_JSON=$("$DASH0" -X teams list -o json)
TEAM_ID=$(echo "$LIST_JSON" | jq -r --arg origin "$ORIGIN" '[.[] | select(.origin == $origin)][0].id // empty')
if [ -z "$TEAM_ID" ]; then
  echo "FAIL: could not find team with origin '$ORIGIN' in list"
  exit 1
fi
echo "Team ID: $TEAM_ID"

# Step 3: Idempotent re-apply must report 'updated' and not produce a
# duplicate.
echo "--- Step 3: Re-apply same fixture — verify idempotency ---"
if ! REAPPLY_OUTPUT=$("$DASH0" -X teams create -f "$FIXTURE"); then
  echo "FAIL: idempotent re-apply failed"
  exit 1
fi
echo "$REAPPLY_OUTPUT"
if ! echo "$REAPPLY_OUTPUT" | grep -q "updated"; then
  echo "FAIL: re-apply did not report 'updated' action"
  exit 1
fi
COUNT=$("$DASH0" -X teams list -o json \
  | jq --arg origin "$ORIGIN" '[.[] | select(.origin == $origin)] | length')
if [ "$COUNT" != "1" ]; then
  echo "FAIL: expected exactly 1 team with origin '$ORIGIN', found $COUNT"
  exit 1
fi

# Step 4: Fetch the team as YAML and assert (a) apiVersion is populated,
# (b) spec.members render as emails (not raw UUIDs), (c) annotations
# dash0.com/created-at and dash0.com/updated-at are populated.
echo "--- Step 4: Get team as JSON and inspect the envelope ---"
if ! "$DASH0" -X teams get "$TEAM_ID" -o yaml > "${TMPDIR}/exported.yaml"; then
  echo "FAIL: teams get -o yaml failed"
  exit 1
fi
EXPORTED_JSON=$("$DASH0" -X teams get "$TEAM_ID" -o json)
API_VERSION=$(echo "$EXPORTED_JSON" | jq -r '.apiVersion // empty')
if [ "$API_VERSION" != "dash0.com/v1alpha1" ]; then
  echo "FAIL: expected apiVersion=dash0.com/v1alpha1, got '$API_VERSION'"
  exit 1
fi
CREATED_AT=$(echo "$EXPORTED_JSON" | jq -r '.metadata.annotations["dash0.com/created-at"] // empty')
UPDATED_AT=$(echo "$EXPORTED_JSON" | jq -r '.metadata.annotations["dash0.com/updated-at"] // empty')
if [ -z "$CREATED_AT" ] || [ -z "$UPDATED_AT" ]; then
  echo "FAIL: server did not populate dash0.com/created-at or dash0.com/updated-at"
  exit 1
fi
if ! echo "$EXPORTED_JSON" | jq -e --arg e "$MEMBER_EMAIL_1" '.spec.members | any(. == $e)' > /dev/null; then
  echo "FAIL: exported spec.members does not contain $MEMBER_EMAIL_1 (expected email, not raw UUID)"
  exit 1
fi
if ! echo "$EXPORTED_JSON" | jq -e --arg e "$MEMBER_EMAIL_2" '.spec.members | any(. == $e)' > /dev/null; then
  echo "FAIL: exported spec.members does not contain $MEMBER_EMAIL_2"
  exit 1
fi

# Step 5: Rewrite the exported fixture — rename the team and drop the second
# member — and apply via 'dash0 apply', which must route Dash0Team through
# the same upsert path.
echo "--- Step 5: Edit exported YAML and apply (declarative update) ---"
# Strip server-managed annotations before re-apply so the request looks like
# a hand-authored fixture on the way back in.
sed \
  -e '/^  annotations:/,/^  labels:/{/^  labels:/!d;}' \
  -e "s|- ${MEMBER_EMAIL_2}||" \
  -e "s|${DISPLAY_NAME}|${UPDATED_DISPLAY_NAME}|" \
  "${TMPDIR}/exported.yaml" \
  | sed '/^$/d' > "${TMPDIR}/updated.yaml"

if ! APPLY_OUTPUT=$("$DASH0" -X apply -f "${TMPDIR}/updated.yaml"); then
  echo "FAIL: apply -f on edited YAML failed"
  exit 1
fi
echo "$APPLY_OUTPUT"
if ! echo "$APPLY_OUTPUT" | grep -q "Dash0Team"; then
  echo "FAIL: apply output did not mention Dash0Team"
  exit 1
fi

# Step 6: Verify the update — display name changed, membership shrunk,
# updated-at advanced, still exactly one team with this origin.
echo "--- Step 6: Verify post-update state ---"
POST_JSON=$("$DASH0" -X teams get "$TEAM_ID" -o json)
POST_NAME=$(echo "$POST_JSON" | jq -r '.spec.display.name')
if [ "$POST_NAME" != "$UPDATED_DISPLAY_NAME" ]; then
  echo "FAIL: expected display name '$UPDATED_DISPLAY_NAME', got '$POST_NAME'"
  exit 1
fi
POST_MEMBER_COUNT=$(echo "$POST_JSON" | jq '.spec.members | length')
if [ "$POST_MEMBER_COUNT" != "1" ]; then
  echo "FAIL: expected 1 member after edit, got $POST_MEMBER_COUNT"
  exit 1
fi
if ! echo "$POST_JSON" | jq -e --arg e "$MEMBER_EMAIL_1" '.spec.members | any(. == $e)' > /dev/null; then
  echo "FAIL: post-edit team should still contain $MEMBER_EMAIL_1"
  exit 1
fi
if echo "$POST_JSON" | jq -e --arg e "$MEMBER_EMAIL_2" '.spec.members | any(. == $e)' > /dev/null; then
  echo "FAIL: post-edit team should not contain $MEMBER_EMAIL_2"
  exit 1
fi
POST_UPDATED_AT=$(echo "$POST_JSON" | jq -r '.metadata.annotations["dash0.com/updated-at"] // empty')
if [ "$POST_UPDATED_AT" = "$UPDATED_AT" ]; then
  echo "FAIL: dash0.com/updated-at did not advance after edit ($POST_UPDATED_AT)"
  exit 1
fi
COUNT=$("$DASH0" -X teams list -o json \
  | jq --arg origin "$ORIGIN" '[.[] | select(.origin == $origin)] | length')
if [ "$COUNT" != "1" ]; then
  echo "FAIL: after edit expected 1 team with origin '$ORIGIN', found $COUNT"
  exit 1
fi

# Step 7: Delete and verify.
echo "--- Step 7: Delete team ---"
if ! "$DASH0" -X teams delete "$TEAM_ID" --force; then
  echo "FAIL: teams delete failed"
  exit 1
fi
LIST_JSON=$("$DASH0" -X teams list -o json)
if echo "$LIST_JSON" | jq -e --arg id "$TEAM_ID" '.[] | select(.id == $id)' > /dev/null 2>&1; then
  echo "FAIL: team '$TEAM_ID' still exists after deletion"
  exit 1
fi

TEAM_ID=""  # prevent trap from re-deleting

echo "=== Team declarative round-trip test PASSED ==="
