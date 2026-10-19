#!/usr/bin/env bash
set -euo pipefail

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

# SLO IDs (dash0.com/id) are server-assigned (e.g. "slo_01k...") and cannot be
# chosen by the client. The client-settable stable upsert key is
# dash0.com/origin — that is what `apply` uses to PUT create-or-replace, so
# origin is the key that makes apply idempotent (see ImportSLO).
ORIGIN="cli-roundtrip-$(uuidgen | tr '[:upper:]' '[:lower:]')"
YAML_FILE="${TMPDIR}/slo.yaml"

# Count the SLOs carrying our injected origin, polling until the expected count
# is reached. A single-shot assertion is flaky: writes are not immediately
# visible to list queries on the dev tenant. Observed in CI, `slos get "$ORIGIN"`
# succeeded while `slos list --all` still returned 0 for the same origin ~0.6s
# after the restore PUT. The log and span roundtrips already poll for the same
# reason.
wait_for_origin_count() {
  local expected="$1"
  local max_attempts=6
  local delay=2
  local attempt count
  for attempt in $(seq 1 "$max_attempts"); do
    count=$("$DASH0" slos list --all -o json \
      | jq --arg o "$ORIGIN" '[.[] | select(.metadata.labels["dash0.com/origin"] == $o)] | length')
    if [ "$count" = "$expected" ]; then
      echo "Matching SLOs: $count (attempt $attempt)"
      return 0
    fi
    if [ "$attempt" -eq "$max_attempts" ]; then
      echo "FAIL: expected exactly $expected SLO(s) with origin '$ORIGIN', found $count after $max_attempts attempts"
      return 1
    fi
    echo "Attempt $attempt/$max_attempts: found $count, expected $expected — retrying in ${delay}s..."
    sleep "$delay"
    delay=$((delay * 2))
  done
}

echo "=== SLO apply idempotency test ==="
echo "Origin: $ORIGIN"

# Inject the origin into the fixture (the client-settable upsert key).
ORIGIN="$ORIGIN" yq '.metadata.labels."dash0.com/origin" = env(ORIGIN)' "$FIXTURES/slo.yaml" > "$YAML_FILE"

# Step 1: First apply — should create the SLO. The printed id is the
# server-assigned slo_... id, not the origin, so we do not assert on it here.
echo "--- Step 1: First apply (expect: created) ---"
APPLY1=$("$DASH0" apply -f "$YAML_FILE")
echo "$APPLY1"
if ! echo "$APPLY1" | grep -qE 'created[[:space:]]*$'; then
  echo "FAIL: expected 'created' in first apply output"
  exit 1
fi

# Step 2: Apply the same file again. This is the real idempotency assertion:
# re-applying an unchanged document must be a no-op, which `apply` reports as
# "no changes". Asserting only the absence of a second "created" would prove no
# duplicate was made, not that the apply changed nothing.
#
# `apply` renders a before/after diff whenever it upserts an existing asset, and
# prints "no changes" when that diff is empty. Both sides are normalized through
# StripSLOServerFields, so the dash0.com/updated-at and dash0.com/version values
# the server bumps on every PUT — even for a byte-identical body — do not count
# as changes.
echo "--- Step 2: Second apply (expect: no changes) ---"
APPLY2=$("$DASH0" apply -f "$YAML_FILE")
echo "$APPLY2"
if ! echo "$APPLY2" | grep -q 'no changes'; then
  echo "FAIL: second apply did not report 'no changes' — re-applying an unchanged SLO is not a no-op"
  exit 1
fi
# Match the "created" action word at end of line only — not the
# "dash0.com/created-at:" annotation that appears in a diff.
if echo "$APPLY2" | grep -qE 'created[[:space:]]*$'; then
  echo "FAIL: unexpected 'created' on second apply — duplicate was created"
  exit 1
fi
# Likewise anchored: an "updated" action word would mean a diff was rendered
# rather than the no-op path. Anchoring matters because "dash0.com/updated-at"
# appears in diff text and would false-match an unanchored grep.
if echo "$APPLY2" | grep -qE 'updated[[:space:]]*$'; then
  echo "FAIL: second apply reported 'updated' — expected a no-op reporting 'no changes'"
  exit 1
fi

# Step 3: Verify exactly one SLO exists with the injected origin (no duplicates),
# and that it is reachable by origin (GET/DELETE accept origin-or-id).
echo "--- Step 3: Verify a single SLO with the expected origin ---"
if ! "$DASH0" slos get "$ORIGIN" > /dev/null; then
  echo "FAIL: slos get '$ORIGIN' failed after second apply"
  exit 1
fi
if ! wait_for_origin_count 1; then
  echo "FAIL: expected exactly 1 SLO with origin '$ORIGIN' after the second apply (duplicate created)"
  exit 1
fi

# Step 4: Delete the SLO (by origin).
echo "--- Step 4: Delete ---"
DELETE4=$("$DASH0" slos delete "$ORIGIN" --force)
echo "$DELETE4"
if ! echo "$DELETE4" | grep -qE 'deleted[[:space:]]*$'; then
  echo "FAIL: expected 'deleted' in delete output"
  exit 1
fi

# Step 5: The API soft-deletes SLOs, so GET still returns the record
# after delete. `slos get` must show it as deleted.
echo "--- Step 5: Get after delete (expect: Deleted line) ---"
GET5=$("$DASH0" slos get "$ORIGIN")
echo "$GET5"
if ! echo "$GET5" | grep -q "^Deleted: "; then
  echo "FAIL: expected a 'Deleted:' line in slos get output after delete"
  exit 1
fi

# Step 6: Apply again after deletion. Apply calls PUT (upsert), which restores
# the soft-deleted SLO and reports it as "restored".
echo "--- Step 6: Apply after delete (expect: restored) ---"
APPLY6=$("$DASH0" apply -f "$YAML_FILE")
echo "$APPLY6"
if ! echo "$APPLY6" | grep -qE 'restored[[:space:]]*$'; then
  echo "FAIL: expected 'restored' in apply output after delete"
  exit 1
fi

# Step 7: Verify the restored asset is active (not marked deleted, and listed once).
echo "--- Step 7: Verify restored asset is active ---"
if ! GET7=$("$DASH0" slos get "$ORIGIN"); then
  echo "FAIL: slos get '$ORIGIN' failed after re-apply"
  exit 1
fi
if echo "$GET7" | grep -q "^Deleted: "; then
  echo "FAIL: slos get still shows '$ORIGIN' as deleted after re-apply"
  exit 1
fi
if ! wait_for_origin_count 1; then
  echo "FAIL: expected exactly 1 SLO with origin '$ORIGIN' after re-apply"
  exit 1
fi

# Cleanup.
CLEANUP=$("$DASH0" slos delete "$ORIGIN" --force)
echo "$CLEANUP"
if ! echo "$CLEANUP" | grep -qE 'deleted[[:space:]]*$'; then
  echo "FAIL: expected 'deleted' in cleanup output"
  exit 1
fi

echo "=== SLO apply idempotency test PASSED ==="
