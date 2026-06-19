package failedchecks

import "github.com/spf13/cobra"

// NewFailedChecksCmd creates the failed-checks parent command.
func NewFailedChecksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "failed-checks",
		Short: "Query Dash0 failed check instances (active or historical issues)",
		Long:  `Query failed check instances from Dash0 alerting. A failed check instance is an active or recently resolved issue raised by a check rule.`,
	}

	cmd.AddCommand(newQueryCmd())

	return cmd
}
