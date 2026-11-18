package awsloggrouppolicies

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/spf13/cobra"
	sigsyaml "sigs.k8s.io/yaml"
)

type getFlags struct {
	connectionFlags
	Output string
}

func newGetCmd() *cobra.Command {
	flags := &getFlags{}

	cmd := &cobra.Command{
		Use:   "get <integration-id>",
		Short: "[experimental] Get the log-group policy of an AWS integration",
		Long: `Retrieve the log-group instrumentation policy of an AWS integration by the
integration's id. A deleted policy is shown with its deletion time and what
happened to the log groups it forwarded.

The YAML output can be edited and passed back to 'update -f' or 'apply -f'.` + internal.CONFIG_HINT,
		Example: `  # Show the policy summary
  dash0 --experimental aws-log-group-policies get <integration-id>

  # Export as YAML (suitable for re-applying)
  dash0 --experimental aws-log-group-policies get <integration-id> -o yaml > policy.yaml

  # Export as JSON
  dash0 --experimental aws-log-group-policies get <integration-id> -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runGet(cmd.Context(), args[0], flags)
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().StringVarP(&flags.Output, "output", "o", "", "Output format: table, json, yaml (default: table; json in agent mode)")

	return cmd
}

func runGet(ctx context.Context, integrationID string, flags *getFlags) error {
	format, err := parseGetFormat(flags.Output)
	if err != nil {
		return err
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	policy, err := apiClient.GetAwsLogGroupInstrumentationPolicy(ctx, integrationID)
	if err != nil {
		return client.HandleAPIError(err, errorContext(integrationID))
	}

	switch format {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(policy)
	case "yaml":
		data, err := sigsyaml.Marshal(policy)
		if err != nil {
			return fmt.Errorf("failed to marshal AWS log-group policy as YAML: %w", err)
		}
		fmt.Print(string(data))
		return nil
	default:
		printSummary(policy)
		return nil
	}
}

func parseGetFormat(s string) (string, error) {
	switch strings.ToLower(s) {
	case "":
		if agentmode.Enabled {
			return "json", nil
		}
		return "table", nil
	case "table", "json":
		return strings.ToLower(s), nil
	case "yaml", "yml":
		return "yaml", nil
	default:
		return "", fmt.Errorf("unknown output format: %s (valid formats: table, json, yaml)", s)
	}
}

func printSummary(policy *dash0api.AwsLogGroupInstrumentationPolicy) {
	fmt.Printf("Kind: %s\n", displayKind)
	fmt.Printf("Integration ID: %s\n", dash0api.GetAwsLogGroupInstrumentationPolicyID(policy))
	labels := policy.Metadata.Labels
	if labels != nil && labels.Dash0Comdataset != nil {
		fmt.Printf("Dataset: %s\n", *labels.Dash0Comdataset)
	}
	if origin := dash0api.GetAwsLogGroupInstrumentationPolicyOrigin(policy); origin != "" {
		fmt.Printf("Origin: %s\n", origin)
	}
	if revision, ok := dash0api.GetAwsLogGroupInstrumentationPolicyRevision(policy); ok {
		fmt.Printf("Revision: %d\n", revision)
	}
	onNoLongerMatching := dash0api.AwsLogGroupPolicyDispositionKeep
	if policy.Spec.OnNoLongerMatching != nil {
		onNoLongerMatching = *policy.Spec.OnNoLongerMatching
	}
	fmt.Printf("On no longer matching: %s\n", onNoLongerMatching)
	if len(policy.Spec.Rules) == 0 {
		fmt.Println("Rules: none (matches no log group)")
	} else {
		fmt.Println("Rules (any rule matches; all conditions of a rule must hold):")
		for i, rule := range policy.Spec.Rules {
			conditions := make([]string, len(rule.Conditions))
			for j, condition := range rule.Conditions {
				conditions[j] = formatCondition(condition)
			}
			fmt.Printf("  %d. %s\n", i+1, strings.Join(conditions, " AND "))
		}
	}
	if annotations := policy.Metadata.Annotations; annotations != nil {
		if annotations.Dash0ComcreatedAt != nil {
			fmt.Printf("Created: %s\n", annotations.Dash0ComcreatedAt.Format("2006-01-02 15:04:05"))
		}
		if annotations.Dash0ComupdatedAt != nil {
			fmt.Printf("Updated: %s\n", annotations.Dash0ComupdatedAt.Format("2006-01-02 15:04:05"))
		}
		if annotations.Dash0ComdeletedAt != nil {
			fmt.Printf("Deleted: %s\n", annotations.Dash0ComdeletedAt.Format("2006-01-02 15:04:05"))
		}
		if annotations.Dash0ComdeletionDisposition != nil {
			fmt.Printf("Deletion disposition: %s\n", *annotations.Dash0ComdeletionDisposition)
		}
	}
}

func formatCondition(condition dash0api.AwsLogGroupPolicyCondition) string {
	switch {
	case condition.Values != nil:
		return fmt.Sprintf("%s %s [%s]", condition.Key, condition.Operator, strings.Join(*condition.Values, ", "))
	case condition.Value != nil:
		return fmt.Sprintf("%s %s %q", condition.Key, condition.Operator, *condition.Value)
	default:
		return fmt.Sprintf("%s %s", condition.Key, condition.Operator)
	}
}
