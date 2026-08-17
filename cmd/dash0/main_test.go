package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/diff"
	"github.com/dash0hq/dash0-cli/internal/skill"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withAgentMode toggles agentmode.Enabled for the duration of the calling
// (sub)test and restores it on cleanup. Kept local because agentmode.Init
// resolves the whole precedence chain (env + flag + auto-detect) and here
// we want a plain override.
func withAgentMode(t *testing.T, enabled bool) {
	t.Helper()
	prev := agentmode.Enabled
	agentmode.Enabled = enabled
	t.Cleanup(func() { agentmode.Enabled = prev })
}

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

// newIsolatedRootForTraverseTest builds a fresh root+child command tree
// shaped like the real dash0 root command (a boolean persistent flag plus
// one subcommand with its own flags), so tests can exercise Traverse
// without touching the package-level rootCmd singleton. rootCmd is mutated
// as a side effect by cobra internals the first time anything calls
// rootCmd.Execute() anywhere in the test binary (e.g.
// TestRootCommandExecution) -- persistent flags get merged into its local
// flag set lazily, which would silently fix the exact bug this test exists
// to catch and make the regression test pass regardless of whether main()
// still carries the fix.
func newIsolatedRootForTraverseTest() (*cobra.Command, *cobra.Command) {
	root := &cobra.Command{Use: "dash0"}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	child := &cobra.Command{Use: "diff"}
	child.Flags().StringP("file", "f", "", "")
	root.AddCommand(child)
	return root, child
}

// TestTraverseTargetCommand is a regression test for a real cobra pitfall:
// Command.Traverse decides whether a "--name" token expects a following
// value by looking up the flag in c.Flags(), which does not include flags
// registered via PersistentFlags() until cobra's own mergePersistentFlags
// runs (normally during Execute, i.e. after Traverse). Without pre-merging
// persistent flags into the root command's own flag set (main()'s fix,
// right before its own Traverse call), a boolean persistent flag preceding
// the subcommand -- e.g. `dash0 --experimental diff ...`, the invocation
// form used throughout this CLI's own docs -- gets wrongly treated as
// expecting a value, swallowing the subcommand name as that value and
// making Traverse resolve to the root command instead of the real target.
// This mattered concretely for `dash0 diff`'s three-way exit code: main()'s
// exitCodeForError branches on the resolved command's name, so a
// misresolved target silently fell back to exit 1 instead of exit 2 on a
// genuine error.
func TestTraverseTargetCommand(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"persistent bool flag before subcommand", []string{"--experimental", "diff", "-f", "x.yaml"}, "diff"},
		{"shorthand persistent bool flag before subcommand", []string{"-X", "diff", "-f", "x.yaml"}, "diff"},
		{"no persistent flag", []string{"diff", "-f", "x.yaml"}, "diff"},
		{"persistent flag after subcommand", []string{"diff", "--experimental", "-f", "x.yaml"}, "diff"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, _ := newIsolatedRootForTraverseTest()
			// The fix under test: without this, a fresh root command (never
			// Executed, so cobra's own lazy persistent-flag merge hasn't run
			// yet) reproduces the bug.
			root.Flags().AddFlagSet(root.PersistentFlags())

			cmd, _, err := root.Traverse(tc.args)
			require.NoError(t, err)
			assert.Equal(t, tc.want, cmd.Name())
		})
	}
}

// TestTraverseTargetCommand_ReproducesBugWithoutFix proves the fix is load-
// bearing: the same isolated, never-Executed root command tree without the
// AddFlagSet pre-merge misresolves a persistent bool flag preceding the
// subcommand, confirming TestTraverseTargetCommand isn't passing for some
// unrelated reason.
func TestTraverseTargetCommand_ReproducesBugWithoutFix(t *testing.T) {
	root, _ := newIsolatedRootForTraverseTest()

	cmd, _, err := root.Traverse([]string{"--experimental", "diff", "-f", "x.yaml"})
	require.NoError(t, err)
	assert.Equal(t, "dash0", cmd.Name(), "without the AddFlagSet pre-merge, Traverse should misresolve to the root command")
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
		// The hint leads with `dash0 skill show` (no arg) so an agent has
		// an immediate path to the topic index, then hands off to
		// `dash0 skill show <topic>` for a focused reference, and finally
		// mentions `dash0 skill install` for persistent setup.
		assert.Contains(t, err.Error(), "\nHint: start with `dash0 skill show` (no arg)")
		assert.Contains(t, err.Error(), "lists every available topic")
		assert.Contains(t, err.Error(), "dash0 skill show <topic>")
		assert.Contains(t, err.Error(), "dash0 skill install")
	})

	t.Run("no-op in human mode when the skill is already installed", func(t *testing.T) {
		// Human user seeing every error grow a "consult the skill" tail
		// would be noise — they read `dash0 --help` and that's it.
		os.Args = []string{"dash0"}
		t.Setenv("DASH0_NO_SKILL_HINT", "")
		withAgentMode(t, false)
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

	t.Run("agent mode with skill installed hints toward using it", func(t *testing.T) {
		// The bare "unknown command \"foobar\"" error told an agent nothing
		// about recovery when the skill was already installed; the hint now
		// points at the two resources that DO help — `dash0 skill show` and
		// `dash0 --agent-mode --help`.
		os.Args = []string{"dash0"}
		t.Setenv("DASH0_NO_SKILL_HINT", "")
		withAgentMode(t, true)
		dir := t.TempDir()
		hostDir := filepath.Join(dir, ".claude", "skills", "dash0-cli")
		require.NoError(t, os.MkdirAll(filepath.Join(hostDir, "references"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(hostDir, "SKILL.md"), []byte("x"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(hostDir, skill.Manifest[0].RelPath), []byte("x"), 0o644))
		t.Chdir(dir)

		err := withSkillHint(errors.New("boom"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "boom")
		assert.Contains(t, err.Error(), "\nHint: consult the installed dash0-cli Agent Skill")
		// The hint spells out the discovery workflow: `dash0 skill show`
		// (no arg) surfaces the topic index the agent needs to see before
		// it can pass a topic name to `dash0 skill show <topic>`.
		assert.Contains(t, err.Error(), "dash0 skill show` (no arg)")
		assert.Contains(t, err.Error(), "lists every available topic")
		assert.Contains(t, err.Error(), "dash0 skill show <topic>")
		assert.Contains(t, err.Error(), "dash0 --agent-mode --help")
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
		assert.Contains(t, err.Error(), "\nHint: start with `dash0 skill show`")
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
		assert.Contains(t, err.Error(), "\nHint: start with `dash0 skill show`",
			"explicit CLI --flag=false must trump an env-var suppression")
	})

	t.Run("--no-skill-hint=0 also disables suppression", func(t *testing.T) {
		os.Args = []string{"dash0", "--no-skill-hint=0", "dashboards", "list"}
		t.Setenv("DASH0_NO_SKILL_HINT", "1")
		t.Chdir(t.TempDir())

		err := withSkillHint(errors.New("boom"))
		assert.Contains(t, err.Error(), "\nHint: start with `dash0 skill show`")
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

// TestExitCodeForError pins dash0 diff's three-way exit code (0 clean -- not
// exercised here since exitCodeForError is only called when err != nil, 1
// differences pending, 2 genuine error) against every other command's
// uniform 1-on-any-error convention.
func TestExitCodeForError(t *testing.T) {
	genericErr := errors.New("boom")
	pendingErr := &diff.PendingDifferencesError{Count: 3}

	cases := []struct {
		name    string
		cmdName string
		err     error
		want    int
	}{
		{"diff genuine error exits 2", "diff", genericErr, 2},
		{"diff pending differences exits 1", "diff", pendingErr, 1},
		{"diff pending differences wrapped still exits 1", "diff", fmt.Errorf("wrapped: %w", pendingErr), 1},
		{"non-diff command exits 1 on any error", "apply", genericErr, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, exitCodeForError(tc.cmdName, tc.err))
		})
	}
}
