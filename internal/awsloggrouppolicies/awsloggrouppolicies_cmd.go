package awsloggrouppolicies

import "github.com/spf13/cobra"

// NewAwsLogGroupPoliciesCmd creates the aws-log-group-policies parent command.
func NewAwsLogGroupPoliciesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "aws-log-group-policies",
		Short: "[experimental] Manage the log-group instrumentation policies of AWS integrations",
		Long: `Get, replace, and delete the log-group instrumentation policy of an AWS
integration.

Each AWS integration has at most one policy, addressed by the integration's id.
The policy forwards every CloudWatch log group whose own tags match one of its
rules: rules are ORed, and the conditions within a rule are ANDed. Log groups
selected by hand keep forwarding regardless of the policy.

There is no create or list: 'update' creates or replaces the policy, and
'delete' removes it while keeping the log groups it forwards, unless
--remove-forwarding is given.`,
	}

	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}
