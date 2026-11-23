package automerge

import "github.com/spf13/cobra"

// NewAutoMergeCmd creates the automerge parent command.
func NewAutoMergeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "automerge",
		Short: "[experimental] Manage AutoMerge",
		Long:  `Read and change how AutoMerge scores, approves, and merges pull requests.`,
	}
	cmd.AddCommand(newSettingsCmd())
	return cmd
}

func newSettingsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "settings",
		Short: "[experimental] Get and replace the AutoMerge settings of a repository",
		Long: `Get and replace the AutoMerge settings of a repository: the bot mode, the trigger,
the scoring criteria and their thresholds, the path rules, and the reviewer.

Every repository AutoMerge observes has exactly one settings document, addressed
by the repository's 'owner/repo' slug or its id. A repository that was never
configured returns the defaults. There is no create or delete: 'update' replaces
the whole spec.

What can be configured about scoring is softThreshold (the overall score a pull
request needs) and each criterion's threshold (its floor) and multiplier (its
weight). The coloured score bands in the AutoMerge UI are display only.`,
	}
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newUpdateCmd())
	return cmd
}
