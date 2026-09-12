#!/usr/bin/env bash
# Ensures a Dash0 CLI profile exists, reusing an already-active one (e.g. from the setup action)
# when present instead of always creating a fresh "default" profile.
#
# Environment variables:
#   INPUT_API_URL     - Dash0 API URL to set (optional).
#   INPUT_OTLP_URL    - Dash0 OTLP URL to set (optional).
#   INPUT_AUTH_TOKEN  - Dash0 auth token to set (optional).
#   INPUT_DATASET     - Dash0 dataset to set (optional).
#   REQUIRE_PROFILE   - "true" if the caller cannot proceed without a profile (hard error when
#                       none exists and no connection inputs were given); "false" if the caller
#                       has a fallback path (e.g. a dry-run that makes no API calls).

set -euo pipefail

ARGS=()
[ -n "${INPUT_API_URL:-}" ] && ARGS+=(--api-url "$INPUT_API_URL")
[ -n "${INPUT_OTLP_URL:-}" ] && ARGS+=(--otlp-url "$INPUT_OTLP_URL")
[ -n "${INPUT_AUTH_TOKEN:-}" ] && ARGS+=(--auth-token "$INPUT_AUTH_TOKEN")
[ -n "${INPUT_DATASET:-}" ] && ARGS+=(--dataset "$INPUT_DATASET")

# If an active profile already exists (e.g., from the setup action), update it with any
# connection inputs provided to this action. `-o json` is preferred over the human-readable
# default because agent mode -- auto-detected from env vars like GITHUB_COPILOT, which a runner
# may have set for unrelated reasons -- would otherwise switch `dash0 config show`'s own output
# to JSON too, and a plain-text `grep '^Profile:'` would then silently find nothing, causing this
# script to overwrite an existing profile instead of updating it.
set +e
JSON_OUT=$(dash0 config show -o json 2>/dev/null)
JSON_RC=$?
set -e

if [ "$JSON_RC" -eq 0 ]; then
  PROFILE_NAME=$(echo "$JSON_OUT" | jq -r '.profile.value // empty')
else
  # Fall back to parsing human-readable output for CLI versions that predate `config show -o
  # json` (introduced alongside agent mode itself in v1.8.0) -- a CLI that old cannot have
  # auto-detected agent mode either, so the JSON-output trap this whole function guards against
  # cannot occur here.
  PROFILE_LINE=$(dash0 config show 2>/dev/null | grep '^Profile:' || true)
  if [ -n "$PROFILE_LINE" ] && ! echo "$PROFILE_LINE" | grep -q '(none)'; then
    PROFILE_NAME=$(echo "$PROFILE_LINE" | sed 's/^Profile:[[:space:]]*//' | sed 's/[[:space:]]*(.*//')
  else
    PROFILE_NAME=""
  fi
fi

if [ -n "$PROFILE_NAME" ]; then
  if [ ${#ARGS[@]} -gt 0 ]; then
    dash0 config profiles update "$PROFILE_NAME" "${ARGS[@]}"
  fi
  exit 0
fi

# No active profile.
if [ ${#ARGS[@]} -eq 0 ]; then
  if [ "${REQUIRE_PROFILE:-true}" = "true" ]; then
    echo "::error::No active profile and no connection inputs provided."
    exit 1
  fi
  echo "No active profile and no connection inputs provided; proceeding without one. This is only safe for a call path that makes no Dash0 API calls -- any other invocation will fail once it needs to reach the Dash0 API."
  exit 0
fi

dash0 config profiles create default "${ARGS[@]}"
dash0 config profiles select default
