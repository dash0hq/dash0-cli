package pods

import (
	"github.com/spf13/cobra"
)

// NewPodsCmd creates the pods command group.
func NewPodsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pods",
		Short: "[experimental] Watch Kubernetes pods in Dash0",
		Long:  `Watch the telemetry Dash0 holds for a Kubernetes pod.`,
	}

	cmd.AddCommand(newWatchCmd())

	return cmd
}
