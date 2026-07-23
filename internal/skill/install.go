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
	hostDir, ok := supportedHosts[slug]
	if !ok {
		return fmt.Errorf(
			"could not detect a supported agent host in this environment (checked: %s)\nHint: install the dash0-cli skill instead with `npx skills add dash0hq/dash0-cli` or `gh skill install dash0hq/dash0-cli`",
			strings.Join(supportedHostNames, ", "),
		)
	}

	targetDir := filepath.Join(baseDir, hostDir)
	written, err := writeBundle(targetDir)
	if err != nil {
		return err
	}

	fmt.Printf("Installed dash0-cli skill (%d files) to %s (detected: %s)\n", len(written), hostDir, slug)
	return nil
}

// writeBundle writes SKILL.md and every reference topic into targetDir,
// creating directories as needed, and returns the paths written (relative
// to targetDir).
func writeBundle(targetDir string) ([]string, error) {
	var written []string

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create %s: %w", targetDir, err)
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
	if err := os.MkdirAll(refsDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create %s: %w", refsDir, err)
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
