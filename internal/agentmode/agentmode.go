package agentmode

import (
	"os"
	"strings"
)

// Enabled reports whether agent mode is active.
// The result is determined once, during Init, and cached for the lifetime of
// the process.
var Enabled bool

// agentEnvVars lists environment variables whose presence indicates the CLI is
// being driven by an AI coding agent.
var agentEnvVars = []string{
	"CLAUDE_CODE",
	"MCP_SESSION_ID",
	"CURSOR_SESSION_ID",
	"WINDSURF_SESSION_ID",
	"CLINE_TASK_ID",
	"CODEX",
	"GITHUB_COPILOT",
	"AIDER",
}

// Init resolves whether agent mode should be active according to the following
// priority (first match wins):
//
//  1. DASH0_AGENT_MODE=0|false  → disabled (overrides everything)
//  2. --agent-mode flag          → enabled  (passed as flagValue)
//  3. DASH0_AGENT_MODE=1|true   → enabled
//  4. Any known AI-agent env var → enabled
//
// Call Init once from main, before any output.
func Init(flagValue bool) {
	envVal := os.Getenv("DASH0_AGENT_MODE")
	lower := strings.ToLower(envVal)

	// Explicit disable overrides everything.
	if lower == "0" || lower == "false" {
		Enabled = false
		return
	}

	// --agent-mode flag.
	if flagValue {
		Enabled = true
		return
	}

	// Explicit enable via env var.
	if lower == "1" || lower == "true" {
		Enabled = true
		return
	}

	// Auto-detect known AI agent environments.
	for _, v := range agentEnvVars {
		if os.Getenv(v) != "" {
			Enabled = true
			return
		}
	}

	Enabled = false
}
