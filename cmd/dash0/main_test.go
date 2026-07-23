package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/skill"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRootCommandExecution tests the root command execution
func TestRootCommandExecution(t *testing.T) {
	// Save the original stdout
	stdout := os.Stdout

	// Create a pipe to capture output
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Execute the root command
	rootCmd.SetArgs([]string{"--help"})
	err := rootCmd.Execute()

	// Close the write end of the pipe
	w.Close()
	os.Stdout = stdout

	// Read the output
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)

	// Verify the command executed without error
	if err != nil {
		t.Errorf("Root command failed: %v", err)
	}

	// Verify the help output contains expected content
	if !bytes.Contains(buf.Bytes(), []byte("Command line interface for interacting with Dash0 services")) {
		t.Errorf("Help output did not contain expected content")
	}
}

// TestWithSkillHint covers the agent-mode error hint pointing at
// `dash0 skill install`, added centrally in printError.
func TestWithSkillHint(t *testing.T) {
	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })

	t.Run("adds hint when skill is not installed and not suppressed", func(t *testing.T) {
		os.Args = []string{"dash0"}
		t.Setenv("DASH0_NO_SKILL_HINT", "")
		t.Chdir(t.TempDir())

		err := withSkillHint(errors.New("boom"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "boom")
		assert.Contains(t, err.Error(), "\nHint: run `dash0 skill install`")
	})

	t.Run("no-op when the skill is already installed in the current directory", func(t *testing.T) {
		os.Args = []string{"dash0"}
		t.Setenv("DASH0_NO_SKILL_HINT", "")
		dir := t.TempDir()
		hostDir := filepath.Join(dir, ".claude", "skills", "dash0-cli")
		require.NoError(t, os.MkdirAll(filepath.Join(hostDir, "references"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(hostDir, "SKILL.md"), []byte("x"), 0o644))
		// IsInstalled requires SKILL.md AND a reference topic to guard against
		// partial installs silently suppressing the hint.
		require.NoError(t, os.WriteFile(filepath.Join(hostDir, skill.Manifest[0].RelPath), []byte("x"), 0o644))
		t.Chdir(dir)

		err := withSkillHint(errors.New("boom"))
		assert.Equal(t, "boom", err.Error())
	})

	t.Run("no-op when suppressed via DASH0_NO_SKILL_HINT", func(t *testing.T) {
		os.Args = []string{"dash0"}
		t.Setenv("DASH0_NO_SKILL_HINT", "1")
		t.Chdir(t.TempDir())

		err := withSkillHint(errors.New("boom"))
		assert.Equal(t, "boom", err.Error())
	})

	t.Run("DASH0_NO_SKILL_HINT=true also suppresses", func(t *testing.T) {
		os.Args = []string{"dash0"}
		t.Setenv("DASH0_NO_SKILL_HINT", "TRUE")
		t.Chdir(t.TempDir())

		err := withSkillHint(errors.New("boom"))
		assert.Equal(t, "boom", err.Error())
	})

	t.Run("DASH0_NO_SKILL_HINT=false does not suppress", func(t *testing.T) {
		os.Args = []string{"dash0"}
		t.Setenv("DASH0_NO_SKILL_HINT", "false")
		t.Chdir(t.TempDir())

		err := withSkillHint(errors.New("boom"))
		assert.Contains(t, err.Error(), "\nHint: run `dash0 skill install`")
	})

	t.Run("no-op when suppressed via --no-skill-hint flag", func(t *testing.T) {
		os.Args = []string{"dash0", "--no-skill-hint", "dashboards", "list"}
		t.Setenv("DASH0_NO_SKILL_HINT", "")
		t.Chdir(t.TempDir())

		err := withSkillHint(errors.New("boom"))
		assert.Equal(t, "boom", err.Error())
	})

	t.Run("--no-skill-hint=true suppresses", func(t *testing.T) {
		os.Args = []string{"dash0", "--no-skill-hint=true", "dashboards", "list"}
		t.Setenv("DASH0_NO_SKILL_HINT", "")
		t.Chdir(t.TempDir())

		err := withSkillHint(errors.New("boom"))
		assert.Equal(t, "boom", err.Error())
	})

	t.Run("--no-skill-hint=1 suppresses", func(t *testing.T) {
		os.Args = []string{"dash0", "--no-skill-hint=1", "dashboards", "list"}
		t.Setenv("DASH0_NO_SKILL_HINT", "")
		t.Chdir(t.TempDir())

		err := withSkillHint(errors.New("boom"))
		assert.Equal(t, "boom", err.Error())
	})

	t.Run("--no-skill-hint=false overrides DASH0_NO_SKILL_HINT=1", func(t *testing.T) {
		os.Args = []string{"dash0", "--no-skill-hint=false", "dashboards", "list"}
		t.Setenv("DASH0_NO_SKILL_HINT", "1")
		t.Chdir(t.TempDir())

		err := withSkillHint(errors.New("boom"))
		assert.Contains(t, err.Error(), "\nHint: run `dash0 skill install`",
			"explicit CLI --flag=false must trump an env-var suppression")
	})

	t.Run("--no-skill-hint=0 also disables suppression", func(t *testing.T) {
		os.Args = []string{"dash0", "--no-skill-hint=0", "dashboards", "list"}
		t.Setenv("DASH0_NO_SKILL_HINT", "1")
		t.Chdir(t.TempDir())

		err := withSkillHint(errors.New("boom"))
		assert.Contains(t, err.Error(), "\nHint: run `dash0 skill install`")
	})

	t.Run("does not stack a second hint onto an error that already has one", func(t *testing.T) {
		os.Args = []string{"dash0"}
		t.Setenv("DASH0_NO_SKILL_HINT", "")
		t.Chdir(t.TempDir())

		original := errors.New("profile is OAuth-typed but not authenticated.\nHint: run `dash0 login`")
		err := withSkillHint(original)
		assert.Equal(t, original.Error(), err.Error())
	})

	t.Run("no-op when detected agent is not a supported install target", func(t *testing.T) {
		// Under aider/cline/windsurf/mcp, `dash0 skill install` would fail
		// with "not yet a supported install target" — pointing the user at
		// it repeatedly on every error is a hint loop with no viable action.
		os.Args = []string{"dash0"}
		t.Setenv("DASH0_NO_SKILL_HINT", "")
		t.Setenv("AIDER", "1")
		t.Setenv("CLAUDE_CODE", "")
		t.Setenv("CLAUDECODE", "")
		t.Setenv("CODEX", "")
		t.Setenv("CURSOR_AGENT", "")
		t.Setenv("GITHUB_COPILOT", "")
		t.Chdir(t.TempDir())

		err := withSkillHint(errors.New("boom"))
		assert.Equal(t, "boom", err.Error(), "detected-but-unsupported agents get no skill-install nudge")
	})
}

// TestFlagValue covers the manual flag scanning used before cobra parses flags.
func TestFlagValue(t *testing.T) {
	cases := []struct {
		name string
		args []string
		flag string
		want string
	}{
		{"not present", []string{"foo", "bar"}, "profile", ""},
		{"space-separated", []string{"--profile", "prod", "cmd"}, "profile", "prod"},
		{"equals form", []string{"--profile=prod", "cmd"}, "profile", "prod"},
		{"value missing at end", []string{"--profile"}, "profile", ""},
		{"empty equals value", []string{"--profile=", "cmd"}, "profile", ""},
		{"stops at --", []string{"--", "--profile", "prod"}, "profile", ""},
		{"does not match prefix only", []string{"--profiled", "prod"}, "profile", ""},
		{"first match wins", []string{"--profile", "first", "--profile", "second"}, "profile", "first"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := flagValue(tc.args, tc.flag)
			if got != tc.want {
				t.Errorf("flagValue(%v, %q) = %q, want %q", tc.args, tc.flag, got, tc.want)
			}
		})
	}
}
