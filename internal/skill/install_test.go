package skill

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// agentEnvVars mirrors internal/agentmode's matcher list. It's duplicated
// here (rather than imported, since it's unexported there) so tests can
// reliably clear every agent-detection marker regardless of what the
// process happens to be running under (this test suite itself commonly
// runs inside Claude Code, which sets CLAUDECODE=1 in its own environment).
var agentEnvVars = []string{
	"AIDER", "CLAUDE_CODE", "CLAUDECODE", "CLINE", "CLINE_TASK_ID",
	"CODEX", "OPENAI_CODEX", "CURSOR_AGENT", "CURSOR_SESSION_ID",
	"GITHUB_COPILOT", "WINDSURF_AGENT", "WINDSURF_SESSION_ID", "MCP_SESSION_ID",
}

func clearAgentEnv(t *testing.T) {
	t.Helper()
	for _, v := range agentEnvVars {
		t.Setenv(v, "")
	}
}

func TestInstallWritesToDetectedHostDirectoryOnly(t *testing.T) {
	cases := []struct {
		name   string
		envVar string
		hostDir string
	}{
		{"claude-code", "CLAUDE_CODE", ".claude/skills/dash0-cli"},
		{"codex", "CODEX", ".agents/skills/dash0-cli"},
		{"cursor", "CURSOR_AGENT", ".cursor/skills/dash0-cli"},
		{"copilot", "GITHUB_COPILOT", ".github/skills/dash0-cli"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearAgentEnv(t)
			t.Setenv(tc.envVar, "1")

			dir := t.TempDir()
			require.NoError(t, runInstall(&installFlags{Dir: dir}))

			for _, entry := range Manifest {
				assert.FileExists(t, filepath.Join(dir, tc.hostDir, entry.RelPath))
			}
			assert.FileExists(t, filepath.Join(dir, tc.hostDir, "SKILL.md"))

			// No other host's directory should have been written.
			for otherSlug, otherHostDir := range supportedHosts {
				if otherHostDir == tc.hostDir {
					continue
				}
				_, err := os.Stat(filepath.Join(dir, otherHostDir))
				assert.True(t, os.IsNotExist(err), "unexpected directory written for host %q", otherSlug)
			}
		})
	}
}

func TestInstallIsIdempotent(t *testing.T) {
	clearAgentEnv(t)
	t.Setenv("CLAUDE_CODE", "1")

	dir := t.TempDir()
	require.NoError(t, runInstall(&installFlags{Dir: dir}))

	skillMDPath := filepath.Join(dir, ".claude/skills/dash0-cli/SKILL.md")
	first, err := os.ReadFile(skillMDPath)
	require.NoError(t, err)

	require.NoError(t, runInstall(&installFlags{Dir: dir}))
	second, err := os.ReadFile(skillMDPath)
	require.NoError(t, err)

	assert.Equal(t, first, second)
}

func TestInstallFailsWithoutSupportedHostAndWritesNoFiles(t *testing.T) {
	clearAgentEnv(t)

	dir := t.TempDir()
	err := runInstall(&installFlags{Dir: dir})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not detect a supported agent host")
	assert.Contains(t, err.Error(), "npx skills add dash0hq/dash0-cli")
	assert.Contains(t, err.Error(), "gh skill install dash0hq/dash0-cli")

	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "install must not write any files when no host is detected")
}
