package agentmode

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitExplicitDisableOverridesAll(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "false")
	t.Setenv("CLAUDE_CODE", "1")
	Init(true)
	assert.False(t, Enabled, "DASH0_AGENT_MODE=false must override flag and auto-detection")
}

func TestInitExplicitDisableZero(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "0")
	Init(true)
	assert.False(t, Enabled)
}

func TestInitFlagEnablesAgentMode(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "")
	Init(true)
	assert.True(t, Enabled)
}

func TestInitEnvVarEnablesAgentMode(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "true")
	Init(false)
	assert.True(t, Enabled)
}

func TestInitEnvVarOne(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "1")
	Init(false)
	assert.True(t, Enabled)
}

func TestInitAutoDetectClaudeCode(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "")
	t.Setenv("CLAUDE_CODE", "1")
	Init(false)
	assert.True(t, Enabled)
}

func TestInitAutoDetectMCPSession(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "")
	t.Setenv("MCP_SESSION_ID", "abc")
	Init(false)
	assert.True(t, Enabled)
}

func TestInitAutoDetectCursor(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "")
	t.Setenv("CURSOR_SESSION_ID", "xyz")
	Init(false)
	assert.True(t, Enabled)
}

func TestInitAutoDetectWindsurf(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "")
	t.Setenv("WINDSURF_SESSION_ID", "xyz")
	Init(false)
	assert.True(t, Enabled)
}

func TestInitAutoDetectCline(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "")
	t.Setenv("CLINE_TASK_ID", "xyz")
	Init(false)
	assert.True(t, Enabled)
}

func TestInitAutoDetectCodex(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "")
	t.Setenv("CODEX", "1")
	Init(false)
	assert.True(t, Enabled)
}

func TestInitAutoDetectGitHubCopilot(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "")
	t.Setenv("GITHUB_COPILOT", "1")
	Init(false)
	assert.True(t, Enabled)
}

func TestInitAutoDetectAider(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "")
	t.Setenv("AIDER", "1")
	Init(false)
	assert.True(t, Enabled)
}

func TestInitAutoDetectClaudeCodeAlt(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "")
	t.Setenv("CLAUDECODE", "1")
	Init(false)
	assert.True(t, Enabled)
}

func TestInitAutoDetectClineAlt(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "")
	t.Setenv("CLINE", "1")
	Init(false)
	assert.True(t, Enabled)
}

func TestInitAutoDetectCursorAgent(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "")
	t.Setenv("CURSOR_AGENT", "1")
	Init(false)
	assert.True(t, Enabled)
}

func TestInitAutoDetectOpenAICodex(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "")
	t.Setenv("OPENAI_CODEX", "1")
	Init(false)
	assert.True(t, Enabled)
}

func TestInitAutoDetectWindsurfAgent(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "")
	t.Setenv("WINDSURF_AGENT", "1")
	Init(false)
	assert.True(t, Enabled)
}

func TestInitNoDetection(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "")
	// Unset all agent env vars
	for _, m := range agentMatchers {
		t.Setenv(m.envVar, "")
	}
	Init(false)
	assert.False(t, Enabled)
	assert.Equal(t, "", Detected)
}

func TestInitDetectsClaudeCodeSlug(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "")
	for _, m := range agentMatchers {
		t.Setenv(m.envVar, "")
	}
	t.Setenv("CLAUDE_CODE", "1")
	Init(false)
	assert.True(t, Enabled)
	assert.Equal(t, "claude-code", Detected)
}

func TestInitDetectsClaudeCodeAltSlug(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "")
	for _, m := range agentMatchers {
		t.Setenv(m.envVar, "")
	}
	t.Setenv("CLAUDECODE", "1")
	Init(false)
	assert.Equal(t, "claude-code", Detected)
}

func TestInitDetectsCursorSlug(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "")
	for _, m := range agentMatchers {
		t.Setenv(m.envVar, "")
	}
	t.Setenv("CURSOR_SESSION_ID", "xyz")
	Init(false)
	assert.Equal(t, "cursor", Detected)
}

func TestInitDetectsCursorAgentSlug(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "")
	for _, m := range agentMatchers {
		t.Setenv(m.envVar, "")
	}
	t.Setenv("CURSOR_AGENT", "1")
	Init(false)
	assert.Equal(t, "cursor", Detected)
}

func TestInitDetectsAiderSlug(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "")
	for _, m := range agentMatchers {
		t.Setenv(m.envVar, "")
	}
	t.Setenv("AIDER", "1")
	Init(false)
	assert.Equal(t, "aider", Detected)
}

func TestInitDetectsClineSlug(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "")
	for _, m := range agentMatchers {
		t.Setenv(m.envVar, "")
	}
	t.Setenv("CLINE_TASK_ID", "xyz")
	Init(false)
	assert.Equal(t, "cline", Detected)
}

func TestInitDetectsCodexSlug(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "")
	for _, m := range agentMatchers {
		t.Setenv(m.envVar, "")
	}
	t.Setenv("CODEX", "1")
	Init(false)
	assert.Equal(t, "codex", Detected)
}

func TestInitDetectsOpenAICodexSlug(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "")
	for _, m := range agentMatchers {
		t.Setenv(m.envVar, "")
	}
	t.Setenv("OPENAI_CODEX", "1")
	Init(false)
	assert.Equal(t, "codex", Detected)
}

func TestInitDetectsCopilotSlug(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "")
	for _, m := range agentMatchers {
		t.Setenv(m.envVar, "")
	}
	t.Setenv("GITHUB_COPILOT", "1")
	Init(false)
	assert.Equal(t, "copilot", Detected)
}

func TestInitDetectsWindsurfSlug(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "")
	for _, m := range agentMatchers {
		t.Setenv(m.envVar, "")
	}
	t.Setenv("WINDSURF_SESSION_ID", "xyz")
	Init(false)
	assert.Equal(t, "windsurf", Detected)
}

func TestInitDetectsMCPSlug(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "")
	for _, m := range agentMatchers {
		t.Setenv(m.envVar, "")
	}
	t.Setenv("MCP_SESSION_ID", "abc")
	Init(false)
	assert.Equal(t, "mcp", Detected)
}

func TestInitVendorWinsOverMCP(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "")
	for _, m := range agentMatchers {
		t.Setenv(m.envVar, "")
	}
	t.Setenv("CLAUDE_CODE", "1")
	t.Setenv("MCP_SESSION_ID", "abc")
	Init(false)
	assert.Equal(t, "claude-code", Detected, "vendor-specific marker should win over generic MCP marker")
}

func TestInitFlagWithoutEnvVarReportsUnknown(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "")
	for _, m := range agentMatchers {
		t.Setenv(m.envVar, "")
	}
	Init(true)
	assert.True(t, Enabled)
	assert.Equal(t, "unknown", Detected)
}

func TestInitFlagWithEnvVarReportsSpecificSlug(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "")
	for _, m := range agentMatchers {
		t.Setenv(m.envVar, "")
	}
	t.Setenv("CLAUDE_CODE", "1")
	Init(true)
	assert.Equal(t, "claude-code", Detected)
}

func TestInitExplicitEnableEnvVarWithoutAgentVarReportsUnknown(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "true")
	for _, m := range agentMatchers {
		t.Setenv(m.envVar, "")
	}
	Init(false)
	assert.True(t, Enabled)
	assert.Equal(t, "unknown", Detected)
}

func TestInitExplicitDisableClearsDetected(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "false")
	t.Setenv("CLAUDE_CODE", "1")
	Init(true)
	assert.False(t, Enabled)
	assert.Equal(t, "", Detected)
}
