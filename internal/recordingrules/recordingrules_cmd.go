package recordingrules

import "github.com/spf13/cobra"

// NewRecordingRulesCmd creates the recording-rules parent command
func NewRecordingRulesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recording-rules",
		Short: "Manage Dash0 recording rules",
		Long:  `Create, list, update, and delete recording rules (Prometheus recording rules) in Dash0`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}
