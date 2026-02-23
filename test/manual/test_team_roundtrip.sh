#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
UNIQUE_ID="roundtrip-test-$(date +%s)-$$"
TEAM_NAME="Manual Test Team ${UNIQUE_ID}"

echo "=== Team round-trip test ==="
echo "Team name: $TEAM_NAME"

# Step 0: Discover existing members to use in the test
echo "--- Step 0: Discover organization members ---"
MEMBERS_JSON=$("$DASH0" -X members list -o json)
MEMBER_COUNT=$(echo "$MEMBERS_JSON" | jq 'length')
if [ "$MEMBER_COUNT" -lt 1 ]; then
  echo "SKIP: need at least 1 organization member, found $MEMBER_COUNT"
  exit 0
fi

MEMBER_ID=$(echo "$MEMBERS_JSON" | jq -r '.[0].metadata.labels["dash0.com/id"]')
MEMBER_EMAIL=$(echo "$MEMBERS_JSON" | jq -r '.[0].spec.display.email')
echo "Using member: $MEMBER_EMAIL (ID: $MEMBER_ID)"

# Step 1: Create a team
echo "--- Step 1: Create team ---"
CREATE_OUTPUT=$("$DASH0" -X teams create "$TEAM_NAME")
echo "$CREATE_OUTPUT"
if ! echo "$CREATE_OUTPUT" | grep -q "$TEAM_NAME"; then
  echo "FAIL: create output does not mention team name"
  exit 1
fi

# Step 2: List teams and find the created team
echo "--- Step 2: List teams and find created team ---"
LIST_JSON=$("$DASH0" -X teams list -o json)
TEAM_ID=$(echo "$LIST_JSON" | jq -r --arg name "$TEAM_NAME" '[.[] | select(.name == $name)][0].id // empty')
if [ -z "$TEAM_ID" ]; then
  echo "FAIL: could not find created team '$TEAM_NAME' in list"
  exit 1
fi
echo "Created team ID: $TEAM_ID"

# Step 3: Get team details
echo "--- Step 3: Get team details ---"
GET_JSON=$("$DASH0" -X teams get "$TEAM_ID" -o json)
GOT_NAME=$(echo "$GET_JSON" | jq -r '.team.spec.display.name // empty')
if [ "$GOT_NAME" != "$TEAM_NAME" ]; then
  echo "FAIL: expected team name '$TEAM_NAME', got '$GOT_NAME'"
  exit 1
fi
echo "Team name matches: $GOT_NAME"

# Step 4: Add a member by ID
echo "--- Step 4: Add member by ID ---"
ADD_OUTPUT=$("$DASH0" -X teams add-members "$TEAM_ID" "$MEMBER_ID")
echo "$ADD_OUTPUT"
if ! echo "$ADD_OUTPUT" | grep -q "1 member added"; then
  echo "FAIL: add-members by ID did not succeed"
  exit 1
fi

# Step 5: Verify member appears in team
echo "--- Step 5: Verify member is in team ---"
TEAM_MEMBERS_JSON=$("$DASH0" -X teams list-members "$TEAM_ID" -o json)
if ! echo "$TEAM_MEMBERS_JSON" | jq -e --arg email "$MEMBER_EMAIL" '.[] | select(.spec.display.email == $email)' > /dev/null 2>&1; then
  echo "FAIL: member $MEMBER_EMAIL not found in team"
  exit 1
fi
echo "Member $MEMBER_EMAIL found in team"

# Step 6: Remove the member by ID
echo "--- Step 6: Remove member by ID ---"
REMOVE_OUTPUT=$("$DASH0" -X teams remove-members "$TEAM_ID" "$MEMBER_ID" --force)
echo "$REMOVE_OUTPUT"
if ! echo "$REMOVE_OUTPUT" | grep -q "1 member removed"; then
  echo "FAIL: remove-members by ID did not succeed"
  exit 1
fi

# Step 7: Add a member by email address
echo "--- Step 7: Add member by email ---"
ADD_EMAIL_OUTPUT=$("$DASH0" -X teams add-members "$TEAM_ID" "$MEMBER_EMAIL")
echo "$ADD_EMAIL_OUTPUT"
if ! echo "$ADD_EMAIL_OUTPUT" | grep -q "1 member added"; then
  echo "FAIL: add-members by email did not succeed"
  exit 1
fi

# Step 8: Verify member appears in team again
echo "--- Step 8: Verify member is in team after email add ---"
TEAM_MEMBERS_JSON=$("$DASH0" -X teams list-members "$TEAM_ID" -o json)
if ! echo "$TEAM_MEMBERS_JSON" | jq -e --arg email "$MEMBER_EMAIL" '.[] | select(.spec.display.email == $email)' > /dev/null 2>&1; then
  echo "FAIL: member $MEMBER_EMAIL not found in team after email add"
  exit 1
fi
echo "Member $MEMBER_EMAIL found in team"

# Step 9: Remove the member by email address
echo "--- Step 9: Remove member by email ---"
REMOVE_EMAIL_OUTPUT=$("$DASH0" -X teams remove-members "$TEAM_ID" "$MEMBER_EMAIL" --force)
echo "$REMOVE_EMAIL_OUTPUT"
if ! echo "$REMOVE_EMAIL_OUTPUT" | grep -q "1 member removed"; then
  echo "FAIL: remove-members by email did not succeed"
  exit 1
fi

# Step 10: Delete the team
echo "--- Step 10: Delete team ---"
if ! "$DASH0" -X teams delete "$TEAM_ID" --force; then
  echo "FAIL: teams delete failed"
  exit 1
fi

# Step 11: Verify deletion
echo "--- Step 11: Verify deletion ---"
LIST_JSON=$("$DASH0" -X teams list -o json)
if echo "$LIST_JSON" | jq -e --arg id "$TEAM_ID" '.[] | select(.id == $id)' > /dev/null 2>&1; then
  echo "FAIL: team '$TEAM_ID' still exists after deletion"
  exit 1
fi

echo "=== Team round-trip test PASSED ==="
