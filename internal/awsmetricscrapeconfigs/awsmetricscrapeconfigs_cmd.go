package awsmetricscrapeconfigs

import "github.com/spf13/cobra"

// NewAwsMetricScrapeConfigsCmd creates the aws-metric-scrape-configs parent command.
func NewAwsMetricScrapeConfigsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "aws-metric-scrape-configs",
		Short: "[experimental] Manage the CloudWatch metric scrape configs of AWS integrations",
		Long: `Get, replace, and clear the CloudWatch metric scrape config of an AWS integration.

Every AWS integration has exactly one scrape config, addressed by the
integration's id. It lists the CloudWatch namespaces and metrics Dash0 collects.
There is no create or list: 'update' replaces the whole config, and 'delete'
clears it without deleting the integration.`,
	}

	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}
