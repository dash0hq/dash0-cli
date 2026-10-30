package awsmetricscrapeconfigs

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
		Short: "[experimental] Get the scrape config of an AWS integration",
		Long: `Retrieve the CloudWatch metric scrape config of an AWS integration by the
integration's id. An integration without a config shows no namespaces.

The YAML output can be edited and passed back to 'update -f' or 'apply -f'.` + internal.CONFIG_HINT,
		Example: `  # Show the config summary
  dash0 --experimental aws-metric-scrape-configs get <integration-id>

  # Export as YAML (suitable for re-applying)
  dash0 --experimental aws-metric-scrape-configs get <integration-id> -o yaml > scrape-config.yaml

  # Export as JSON
  dash0 --experimental aws-metric-scrape-configs get <integration-id> -o json`,
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

	cfg, err := apiClient.GetAwsMetricScrapeConfig(ctx, integrationID)
	if err != nil {
		return client.HandleAPIError(err, errorContext(integrationID))
	}
	if err := requireConfig(cfg, integrationID); err != nil {
		return err
	}

	switch format {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(cfg)
	case "yaml":
		data, err := sigsyaml.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("failed to marshal AWS metric scrape config as YAML: %w", err)
		}
		fmt.Print(string(data))
		return nil
	default:
		printSummary(cfg)
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

func printSummary(cfg *dash0api.AwsMetricScrapeConfigDefinition) {
	fmt.Printf("Kind: %s\n", displayKind)
	fmt.Printf("Integration: %s\n", dash0api.GetAwsMetricScrapeConfigName(cfg))
	fmt.Printf("Integration ID: %s\n", dash0api.GetAwsMetricScrapeConfigID(cfg))
	if labels := cfg.Metadata.Labels; labels != nil && labels.Dash0Comdataset != nil {
		fmt.Printf("Dataset: %s\n", *labels.Dash0Comdataset)
	}
	if origin := dash0api.GetAwsMetricScrapeConfigOrigin(cfg); origin != "" {
		fmt.Printf("Origin: %s\n", origin)
	}
	if len(cfg.Spec.Namespaces) == 0 {
		fmt.Println("Namespaces: none (nothing is scraped)")
	} else {
		fmt.Println("Namespaces:")
		for _, ns := range cfg.Spec.Namespaces {
			filtered := ""
			if ns.Filter != nil && len(*ns.Filter) > 0 {
				filtered = ", filtered"
			}
			fmt.Printf("  %s (%s%s)\n", ns.Namespace, pluralize(len(ns.Metrics), "metric"), filtered)
		}
	}
	if annotations := cfg.Metadata.Annotations; annotations != nil {
		if annotations.Dash0ComcreatedAt != nil {
			fmt.Printf("Created: %s\n", annotations.Dash0ComcreatedAt.Format("2006-01-02 15:04:05"))
		}
		if annotations.Dash0ComupdatedAt != nil {
			fmt.Printf("Updated: %s\n", annotations.Dash0ComupdatedAt.Format("2006-01-02 15:04:05"))
		}
	}
}

func pluralize(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
