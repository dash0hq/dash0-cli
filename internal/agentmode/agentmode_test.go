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

func TestInitNoDetection(t *testing.T) {
	t.Setenv("DASH0_AGENT_MODE", "")
	// Unset all agent env vars
	for _, v := range agentEnvVars {
		t.Setenv(v, "")
	}
	Init(false)
	assert.False(t, Enabled)
}
