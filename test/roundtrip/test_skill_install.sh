#!/usr/bin/env bash
# shellcheck disable=SC2030,SC2031
# Every per-host test runs its `export`+`unset` sequence inside a `(...)`
# subshell precisely so the env changes stay scoped to that test — the
# "modification is local to subshell" warning is the intended behavior,
# not a bug, since we don't read those vars back in the parent shell.
#
# Round-trip test for `dash0 skill install` / `dash0 skill show`.
#
# These commands are local-only (no Dash0 API, no OTLP), so this script does
# not depend on any profile or credentials. It exercises:
#
#   1. install into a scratch dir under each of the four supported agent
#      hosts (claude-code, codex, cursor, copilot) — asserting that ONLY
#      that host's directory is populated;
#   2. install byte-for-byte parity against internal/skill/content/;
#   3. idempotency — running install twice yields the same content;
#   4. the two failure paths (no agent detected, detected-but-unsupported)
#      exit non-zero with actionable messages and write no files;
#   5. `skill show` prints SKILL.md and every topic referenced by the bundle.
#
# The set of agent-detection env vars must match agentmode/agentmode.go's
# agentMatchers list. Clearing them all up front makes each per-host test
# reproducible regardless of which agent runs the suite.
set -euo pipefail

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
DASH0="${REPO_ROOT}/build/dash0"
CONTENT_DIR="${REPO_ROOT}/internal/skill/content"

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

# The complete env-var matcher list from internal/agentmode/agentmode.go.
# Kept in sync with the list `internal/skill/install_test.go` also mirrors.
AGENT_ENV_VARS=(
  AIDER
  CLAUDE_CODE CLAUDECODE
  CLINE CLINE_TASK_ID
  CODEX OPENAI_CODEX
  CURSOR_AGENT CURSOR_SESSION_ID
  GITHUB_COPILOT
  WINDSURF_AGENT WINDSURF_SESSION_ID
  MCP_SESSION_ID
)

clear_agent_env() {
  for v in "${AGENT_ENV_VARS[@]}"; do
    unset "$v"
  done
}

# Assert: install under host_env=1 writes exactly host_dir under the scratch
# root, and NO other supported host directory appears.
assert_install_host() {
  local case_name="$1"
  local host_env="$2"
  local host_dir="$3"

  local scratch="${TMPDIR}/${case_name}"
  mkdir -p "$scratch"

  (
    clear_agent_env
    export "${host_env}=1"
    "$DASH0" skill install --dir "$scratch"
  ) >/dev/null

  # SKILL.md and every reference topic must exist.
  if [ ! -f "${scratch}/${host_dir}/SKILL.md" ]; then
    echo "FAIL[${case_name}]: ${host_dir}/SKILL.md not written"
    return 1
  fi
  if [ ! -d "${scratch}/${host_dir}/references" ]; then
    echo "FAIL[${case_name}]: ${host_dir}/references dir not written"
    return 1
  fi

  # Content parity: installed files must match internal/skill/content byte-for-byte.
  if ! diff -q "${CONTENT_DIR}/SKILL.md" "${scratch}/${host_dir}/SKILL.md" >/dev/null; then
    echo "FAIL[${case_name}]: installed SKILL.md differs from internal/skill/content/SKILL.md"
    return 1
  fi
  if ! diff -r "${CONTENT_DIR}/references" "${scratch}/${host_dir}/references" >/dev/null; then
    echo "FAIL[${case_name}]: installed references differ from internal/skill/content/references"
    return 1
  fi

  # No other supported host's directory should exist. The set of "other"
  # directories mirrors internal/skill/bundle.go's supportedHosts.
  local other_dirs=(".claude/skills/dash0-cli" ".agents/skills/dash0-cli" ".cursor/skills/dash0-cli" ".github/skills/dash0-cli")
  for other in "${other_dirs[@]}"; do
    if [ "$other" = "$host_dir" ]; then
      continue
    fi
    if [ -e "${scratch}/${other}" ]; then
      echo "FAIL[${case_name}]: unexpected write at ${other} for host ${host_env}"
      return 1
    fi
  done

  echo "PASS[${case_name}]: install → ${host_dir}"
}

test_install_all_hosts() {
  assert_install_host "claude-code" "CLAUDE_CODE"    ".claude/skills/dash0-cli"
  assert_install_host "codex"        "CODEX"          ".agents/skills/dash0-cli"
  assert_install_host "cursor"       "CURSOR_AGENT"   ".cursor/skills/dash0-cli"
  assert_install_host "copilot"      "GITHUB_COPILOT" ".github/skills/dash0-cli"
}

test_idempotent() {
  local scratch="${TMPDIR}/idempotency"
  mkdir -p "$scratch"

  (
    clear_agent_env
    export CLAUDE_CODE=1
    "$DASH0" skill install --dir "$scratch"
    "$DASH0" skill install --dir "$scratch"
  ) >/dev/null

  # Second install must yield the same content as internal/skill/content.
  if ! diff -q "${CONTENT_DIR}/SKILL.md" "${scratch}/.claude/skills/dash0-cli/SKILL.md" >/dev/null; then
    echo "FAIL[idempotency]: second install's SKILL.md diverged"
    return 1
  fi
  if ! diff -r "${CONTENT_DIR}/references" "${scratch}/.claude/skills/dash0-cli/references" >/dev/null; then
    echo "FAIL[idempotency]: second install's references diverged"
    return 1
  fi
  echo "PASS[idempotency]: two consecutive installs produce identical content"
}

test_no_agent_detected() {
  local scratch="${TMPDIR}/no-agent"
  mkdir -p "$scratch"

  local output
  local rc=0
  output=$(
    clear_agent_env
    "$DASH0" skill install --dir "$scratch" 2>&1
  ) || rc=$?

  if [ "$rc" -eq 0 ]; then
    echo "FAIL[no-agent]: install unexpectedly succeeded with no agent env set"
    return 1
  fi
  if ! echo "$output" | grep -q "could not detect an AI coding agent"; then
    echo "FAIL[no-agent]: expected 'could not detect an AI coding agent' in error, got: $output"
    return 1
  fi
  if ! echo "$output" | grep -q "dash0 skill show"; then
    echo "FAIL[no-agent]: error should mention 'dash0 skill show' as fallback, got: $output"
    return 1
  fi
  if [ -n "$(ls -A "$scratch" 2>/dev/null || true)" ]; then
    echo "FAIL[no-agent]: expected no files written, but scratch dir is non-empty"
    return 1
  fi
  echo "PASS[no-agent]: fails cleanly, no files written"
}

test_detected_but_unsupported() {
  local scratch="${TMPDIR}/unsupported"
  mkdir -p "$scratch"

  local output
  local rc=0
  output=$(
    clear_agent_env
    export AIDER=1
    "$DASH0" skill install --dir "$scratch" 2>&1
  ) || rc=$?

  if [ "$rc" -eq 0 ]; then
    echo "FAIL[unsupported]: install unexpectedly succeeded under AIDER=1"
    return 1
  fi
  if ! echo "$output" | grep -q "not yet a supported install target"; then
    echo "FAIL[unsupported]: expected 'not yet a supported install target' in error, got: $output"
    return 1
  fi
  if ! echo "$output" | grep -q "aider"; then
    echo "FAIL[unsupported]: error should name the detected slug 'aider', got: $output"
    return 1
  fi
  if [ -n "$(ls -A "$scratch" 2>/dev/null || true)" ]; then
    echo "FAIL[unsupported]: expected no files written, but scratch dir is non-empty"
    return 1
  fi
  echo "PASS[unsupported]: detected-but-unsupported host fails cleanly with a specific message"
}

test_show_reprints_bundle() {
  local out
  out=$("$DASH0" skill show)
  if ! echo "$out" | grep -q "^name: dash0-cli"; then
    echo "FAIL[show]: SKILL.md frontmatter 'name: dash0-cli' not found in stdout"
    return 1
  fi

  # Every topic must be printable.
  for topic_path in "${CONTENT_DIR}/references"/*.md; do
    local topic
    topic="$(basename "$topic_path" .md)"
    local out
    out=$("$DASH0" skill show "$topic")
    if [ -z "$out" ]; then
      echo "FAIL[show]: topic '${topic}' printed empty output"
      return 1
    fi
    if ! echo "$out" | grep -q "Generated by internal/skill/gen"; then
      echo "FAIL[show]: topic '${topic}' missing generator header"
      return 1
    fi
  done
  local topic_count
  topic_count=$(find "${CONTENT_DIR}/references" -maxdepth 1 -name '*.md' -type f | wc -l | tr -d ' ')
  echo "PASS[show]: SKILL.md + all ${topic_count} topics reprintable"
}

test_show_unknown_topic_lists_valid() {
  local output
  local rc=0
  output=$("$DASH0" skill show bogus-topic 2>&1) || rc=$?
  if [ "$rc" -eq 0 ]; then
    echo "FAIL[show-unknown]: skill show unexpectedly succeeded for bogus topic"
    return 1
  fi
  if ! echo "$output" | grep -q "unknown skill topic"; then
    echo "FAIL[show-unknown]: error should say 'unknown skill topic', got: $output"
    return 1
  fi
  if ! echo "$output" | grep -q "Hint: valid topics are:"; then
    echo "FAIL[show-unknown]: error should list valid topics, got: $output"
    return 1
  fi
  echo "PASS[show-unknown]: unknown topic error carries the actionable topic list"
}

test_agent_mode_unknown_command_hints_use_the_skill() {
  # Reproduces the exact case that motivated the "use it, don't just
  # install it" side of withSkillHint: skill IS installed AND agent mode
  # is on AND the agent invokes an unknown command. The JSON error must
  # carry a hint pointing at `dash0 skill show` and `--agent-mode --help`,
  # not just report the bare "unknown command" and leave the agent stuck.
  local scratch="${TMPDIR}/agent-hint"
  mkdir -p "$scratch"
  (
    clear_agent_env
    export CLAUDE_CODE=1
    "$DASH0" skill install --dir "$scratch"
  ) >/dev/null

  local output
  local rc=0
  output=$(
    clear_agent_env
    export DASH0_AGENT_MODE=1
    cd "$scratch"
    "$DASH0" foobar 2>&1
  ) || rc=$?

  if [ "$rc" -eq 0 ]; then
    echo "FAIL[agent-hint]: unknown command unexpectedly succeeded"
    return 1
  fi
  if ! echo "$output" | grep -q "consult the installed dash0-cli Agent Skill"; then
    echo "FAIL[agent-hint]: expected 'consult the installed dash0-cli Agent Skill' in JSON output, got: $output"
    return 1
  fi
  if ! echo "$output" | grep -q "dash0 skill show"; then
    echo "FAIL[agent-hint]: expected 'dash0 skill show' in hint, got: $output"
    return 1
  fi
  if ! echo "$output" | grep -q "dash0 --agent-mode --help"; then
    echo "FAIL[agent-hint]: expected 'dash0 --agent-mode --help' in hint, got: $output"
    return 1
  fi
  echo "PASS[agent-hint]: agent-mode error with installed skill carries the reuse hint"
}

echo "=== Skill install/show round-trip test ==="
test_install_all_hosts
test_idempotent
test_no_agent_detected
test_detected_but_unsupported
test_show_reprints_bundle
test_show_unknown_topic_lists_valid
test_agent_mode_unknown_command_hints_use_the_skill
echo "All skill install/show checks passed."
