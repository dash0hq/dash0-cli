package skill

import "github.com/spf13/cobra"

// NewSkillCmd creates the parent `skill` command, which manages the
// dash0-cli Agent Skill for AI coding agents.
func NewSkillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Manage the dash0-cli Agent Skill for AI coding agents",
		Long: `Manage the dash0-cli Agent Skill: SKILL.md plus reference docs, following the
open Agent Skills specification (agentskills.io), that teaches AI coding
agents (Claude Code, Cursor, Codex, GitHub Copilot, and others) how to use
this CLI without spending turns on --help exploration.

Unlike the rest of the CLI, these commands never call the Dash0 API — they
only read embedded content and write to the local filesystem — so they take
no --api-url, --auth-token, or --dataset flags, and are not gated behind
--experimental.`,
	}

	cmd.AddCommand(newInstallCmd())
	cmd.AddCommand(newShowCmd())

	return cmd
}
