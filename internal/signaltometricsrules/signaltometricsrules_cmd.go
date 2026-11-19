package signaltometricsrules

import "github.com/spf13/cobra"

// NewSignalToMetricsRulesCmd creates the signal-to-metrics-rules parent command.
func NewSignalToMetricsRulesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "signal-to-metrics-rules",
		Aliases: []string{"s2m"},
		Short:   "Manage Dash0 signal-to-metrics rules",
		Long: `Create, list, get, update, delete, and test signal-to-metrics rules in Dash0.

Signal-to-metrics rules are part of Signal Control. Each rule matches logs or
spans with a filter and turns the matching signals into a metric, so you can
keep the metric and drop the raw signals.

A rule is identified by its 'dash0.com/origin' label, which is the key that
'create' and 'apply' upsert by, or by its server-assigned ID.`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newDeleteCmd())
	cmd.AddCommand(newTestCmd())

	return cmd
}
