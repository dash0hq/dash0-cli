package slos

import "github.com/spf13/cobra"

// NewSlosCmd creates the slos parent command
func NewSlosCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "slos",
		Short: "Manage Dash0 service level objectives (SLOs)",
		Long:  `Create, list, update, and delete service level objectives (SLOs) in Dash0. SLO documents use the OpenSLO v1 format (apiVersion: openslo.com/v1, kind: SLO).`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}
