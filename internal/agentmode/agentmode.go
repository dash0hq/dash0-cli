package agentmode

import (
	"os"
	"strings"
)

// Enabled reports whether agent mode is active.
// The result is determined once, during Init, and cached for the lifetime of
// the process.
var Enabled bool

// Detected is the canonical slug of the AI agent driving the CLI when
// agent mode is active (e.g. "claude-code", "cursor", "aider"). It is
// "unknown" when agent mode is enabled explicitly via the flag or
// DASH0_AGENT_MODE but no known agent env var was found, and "" when
// agent mode is disabled.
var Detected string

// agentMatcher pairs an environment-variable name with the canonical slug
// for the AI agent it signals.
type agentMatcher struct {
	envVar string
	slug   string
}

// agentMatchers lists the environment variables whose presence indicates
// the CLI is being driven by a known AI coding agent, in priority order.
// The first matching entry wins, so vendor-specific markers come before
// the generic MCP session marker.
var agentMatchers = []agentMatcher{
	{"AIDER", "aider"},
	{"CLAUDE_CODE", "claude-code"},
	{"CLAUDECODE", "claude-code"},
	{"CLINE", "cline"},
	{"CLINE_TASK_ID", "cline"},
	{"CODEX", "codex"},
	{"OPENAI_CODEX", "codex"},
	{"CURSOR_AGENT", "cursor"},
	{"CURSOR_SESSION_ID", "cursor"},
	{"GITHUB_COPILOT", "copilot"},
	{"WINDSURF_AGENT", "windsurf"},
	{"WINDSURF_SESSION_ID", "windsurf"},
	{"MCP_SESSION_ID", "mcp"},
}

// Init resolves whether agent mode should be active according to the following
// priority (first match wins):
//
//  1. DASH0_AGENT_MODE=0|false  → disabled (overrides everything)
//  2. --agent-mode flag          → enabled  (passed as flagValue)
//  3. DASH0_AGENT_MODE=1|true   → enabled
//  4. Any known AI-agent env var → enabled
//
// Independently of which path enables agent mode, the environment is
// scanned for known AI-agent markers so Detected holds the most specific
// slug available (or "unknown" when none is found but agent mode is
// active anyway).
//
// Call Init once from main, before any output.
func Init(flagValue bool) {
	envVal := os.Getenv("DASH0_AGENT_MODE")
	lower := strings.ToLower(envVal)

	// Explicit disable overrides everything.
	if lower == "0" || lower == "false" {
		Enabled = false
		Detected = ""
		return
	}

	slug := DetectAgentSlug()

	// --agent-mode flag.
	if flagValue {
		Enabled = true
		Detected = slugOrUnknown(slug)
		return
	}

	// Explicit enable via env var.
	if lower == "1" || lower == "true" {
		Enabled = true
		Detected = slugOrUnknown(slug)
		return
	}

	// Auto-detect known AI agent environments.
	if slug != "" {
		Enabled = true
		Detected = slug
		return
	}

	Enabled = false
	Detected = ""
}

// DetectAgentSlug scans the environment for known AI-agent markers and
// returns the canonical slug of the first match (e.g. "claude-code",
// "cursor", "codex"), or "" if none is found. Unlike Detected, this is not
// affected by DASH0_AGENT_MODE — it reports which agent host the process is
// running under regardless of whether agent-mode output is enabled, which
// is what callers like internal/skill need for picking a host-specific
// directory.
func DetectAgentSlug() string {
	for _, m := range agentMatchers {
		if os.Getenv(m.envVar) != "" {
			return m.slug
		}
	}
	return ""
}

func slugOrUnknown(slug string) string {
	if slug == "" {
		return "unknown"
	}
	return slug
}
