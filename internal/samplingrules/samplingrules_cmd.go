package samplingrules

import "github.com/spf13/cobra"

// NewSamplingRulesCmd creates the sampling-rules parent command.
func NewSamplingRulesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sampling-rules",
		Short: "Manage Dash0 sampling rules",
		Long: `Create, list, get, update, and delete sampling rules in Dash0.

Sampling rules are part of Signal Control. Each rule decides which traces
Dash0 keeps for a dataset: a share of all traces, traces with an error, traces
matching an OTTL condition, or a combination, with an optional rate limit.

Sampling is off for a dataset until it has an enabled rule. From then on,
Dash0 drops every trace that no enabled rule keeps, and dropped traces cannot
be recovered. The CLI warns when a write turns sampling on.

A rule is identified by its 'dash0.com/origin' label, which is the key that
'create' and 'apply' upsert by, or by its ID. Writing and deleting sampling
rules requires the organization admin role.`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}
