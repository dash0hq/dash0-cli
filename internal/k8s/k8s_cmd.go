package k8s

import (
	"github.com/dash0hq/dash0-cli/internal/k8s/pods"
	"github.com/spf13/cobra"
)

// NewK8sCmd creates the k8s parent command.
func NewK8sCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "k8s",
		Short: "[experimental] Show Dash0 telemetry for Kubernetes workloads",
		Long:  `Show the telemetry Dash0 holds for Kubernetes workloads.`,
	}

	cmd.AddCommand(pods.NewPodsCmd())

	return cmd
}
