package slos

import (
	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/spf13/cobra"
)

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

// sloOrigin returns the dash0.com/origin label of an SLO, or "" when absent.
// The API client ships GetSLOID but no GetSLOOrigin, so the label is read
// directly — the same thing asset.ImportSLO does. Origin matters for output
// because it is the SLO upsert key: SLO ids are server-assigned, so origin is
// the identifier users pin in version control and script against.
func sloOrigin(slo *dash0api.SloDefinition) string {
	if slo == nil || slo.Metadata.Labels == nil || slo.Metadata.Labels.Dash0Comorigin == nil {
		return ""
	}
	return *slo.Metadata.Labels.Dash0Comorigin
}
