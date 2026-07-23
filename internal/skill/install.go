package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/spf13/cobra"
)

type installFlags struct {
	Dir string
}

func newInstallCmd() *cobra.Command {
	flags := &installFlags{}

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the dash0-cli Agent Skill into the current project",
		Long: `Detect which supported AI coding agent (Claude Code, Codex, Cursor, or GitHub
Copilot) is driving the current session, and install the dash0-cli Agent
Skill — SKILL.md plus reference docs — into that agent's conventional
skills directory in the current project.

Future agent sessions in this project then discover dash0-cli's command
surface automatically, without spending turns on --help exploration.

If no supported agent host can be detected, install fails and points at the
standards-based alternative (` + "`npx skills add dash0hq/dash0-cli`" + ` or
` + "`gh skill install dash0hq/dash0-cli`" + `) instead of guessing where to
write files.`,
		Example: `  # Install the skill for the detected agent host
  dash0 skill install

  # Install into a different directory than the current one
  dash0 skill install --dir ./my-project`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runInstall(flags)
		},
	}

	cmd.Flags().StringVar(&flags.Dir, "dir", "", "Directory to install into (default: current directory)")

	return cmd
}

func runInstall(flags *installFlags) error {
	baseDir := flags.Dir
	if baseDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to resolve current directory: %w", err)
		}
		baseDir = wd
	}

	slug := agentmode.DetectAgentSlug()
	if slug == "" {
		return fmt.Errorf(
			"could not detect an AI coding agent in this environment (checked for: %s)\nHint: install the dash0-cli skill via a standards-based route instead — `npx skills add dash0hq/dash0-cli` or `gh skill install dash0hq/dash0-cli` — or run `dash0 skill show` to print the skill without writing files",
			strings.Join(supportedHostDisplayNames(), ", "),
		)
	}
	host, ok := findSupportedHost(slug)
	if !ok {
		return fmt.Errorf(
			"detected agent host %q, which is not yet a supported install target (supported: %s)\nHint: install the dash0-cli skill via a standards-based route instead — `npx skills add dash0hq/dash0-cli` or `gh skill install dash0hq/dash0-cli` — or run `dash0 skill show` to print the skill without writing files",
			slug,
			strings.Join(supportedHostDisplayNames(), ", "),
		)
	}

	targetDir := filepath.Join(baseDir, host.Dir)
	written, err := writeBundle(targetDir)
	if err != nil {
		return err
	}

	fmt.Printf("Installed dash0-cli skill (%d files) to %s (detected: %s)\n", len(written), host.Dir, slug)
	return nil
}

// writeBundle writes SKILL.md and every reference topic into targetDir,
// creating directories as needed, and returns the paths written (relative
// to targetDir).
//
// The references/ subdirectory is cleared before writing so that a manifest
// shrinkage across releases (a topic removed) does not leave orphaned files
// in an installed bundle. SKILL.md is overwritten in place — losing extra
// user-added files at the bundle root would be surprising.
func writeBundle(targetDir string) ([]string, error) {
	var written []string

	if err := ensureDir(targetDir); err != nil {
		return nil, err
	}

	skillMD, err := SkillMD()
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(targetDir, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
		return nil, fmt.Errorf("failed to write SKILL.md: %w", err)
	}
	written = append(written, "SKILL.md")

	refsDir := filepath.Join(targetDir, "references")
	if err := os.RemoveAll(refsDir); err != nil {
		return nil, fmt.Errorf("failed to clear stale references at %s: %w", refsDir, err)
	}
	if err := ensureDir(refsDir); err != nil {
		return nil, err
	}

	for _, entry := range Manifest {
		content, err := TopicContent(entry.Topic)
		if err != nil {
			return nil, err
		}
		relName := filepath.Base(entry.RelPath)
		if err := os.WriteFile(filepath.Join(refsDir, relName), []byte(content), 0o644); err != nil {
			return nil, fmt.Errorf("failed to write %s: %w", entry.RelPath, err)
		}
		written = append(written, entry.RelPath)
	}

	return written, nil
}

// ensureDir creates dir with MkdirAll, but pre-checks for a blocking regular
// file so the resulting error names the file instead of surfacing the opaque
// "not a directory" ENOTDIR from MkdirAll.
func ensureDir(dir string) error {
	if fi, err := os.Lstat(dir); err == nil && !fi.IsDir() {
		return fmt.Errorf(
			"cannot create %s: a file (not a directory) exists at that path. Remove it or pass a different --dir.",
			dir,
		)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create %s: %w", dir, err)
	}
	return nil
}
