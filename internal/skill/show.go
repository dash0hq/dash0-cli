package skill

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show [topic]",
		Short: "Print the dash0-cli Agent Skill content to stdout",
		Long: `Print the dash0-cli Agent Skill bundle to stdout without writing any files —
the disk-free path for CI or ephemeral agent sessions, or for environments
where the agent host can't be auto-detected.

With no argument, prints SKILL.md, the entry point, which includes an index
of every available topic. With a topic argument, prints that topic's
reference content raw.

Every response is plain markdown, in both human and agent mode — a skill's
content is prose meant to be read directly, not structured data.`,
		Example: `  # Print the entry-point SKILL.md, including the topic index
  dash0 skill show

  # Print a specific topic's reference content
  dash0 skill show dashboards
  dash0 skill show logs`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runShow(args)
		},
	}

	return cmd
}

func runShow(args []string) error {
	if len(args) == 0 {
		content, err := SkillMD()
		if err != nil {
			return err
		}
		fmt.Print(content)
		return nil
	}

	content, err := TopicContent(args[0])
	if err != nil {
		return err
	}
	fmt.Print(content)
	return nil
}
