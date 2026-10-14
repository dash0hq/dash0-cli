package datasets

import (
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
	ApiUrl    string
	AuthToken string
	Output    string
}

func newGetCmd() *cobra.Command {
	flags := &getFlags{}

	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "[experimental] Get a dataset by ID",
		Long:  `Retrieve a dataset definition by its ID (slug) or origin.` + internal.CONFIG_HINT,
		Example: `  # Get dataset details
  dash0 --experimental datasets get <id>

  # Output as JSON
  dash0 --experimental datasets get <id> -o json

  # Output as YAML
  dash0 --experimental datasets get <id> -o yaml`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runGet(cmd, args[0], flags)
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().StringVarP(&flags.Output, "output", "o", "", "Output format: table, json, yaml (default: table)")

	return cmd
}

func runGet(cmd *cobra.Command, originOrID string, flags *getFlags) error {
	ctx := cmd.Context()

	switch strings.ToLower(flags.Output) {
	case "", "table", "json", "yaml", "yml":
	default:
		return fmt.Errorf("unknown output format: %s (valid formats: table, json, yaml)", flags.Output)
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset, err := apiClient.GetDataset(ctx, originOrID)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: "dataset", AssetID: originOrID})
	}

	switch strings.ToLower(flags.Output) {
	case "json":
		return printJSON(dataset)
	case "yaml", "yml":
		data, err := sigsyaml.Marshal(dataset)
		if err != nil {
			return fmt.Errorf("failed to marshal dataset as YAML: %w", err)
		}
		fmt.Print(string(data))
		return nil
	case "":
		if agentmode.Enabled {
			return printJSON(dataset)
		}
		printDetails(dataset)
		return nil
	case "table":
		printDetails(dataset)
		return nil
	default:
		return fmt.Errorf("unknown output format: %s (valid formats: table, json, yaml)", flags.Output)
	}
}

func printJSON(dataset *dash0api.DatasetDefinition) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(dataset)
}

func printDetails(dataset *dash0api.DatasetDefinition) {
	fmt.Printf("Kind:   %s\n", dataset.Kind)
	fmt.Printf("Name:   %s\n", dash0api.GetDatasetName(dataset))
	if id := dash0api.GetDatasetID(dataset); id != "" {
		fmt.Printf("ID:     %s\n", id)
	}
	if l := dataset.Metadata.Labels; l != nil && l.Dash0Comorigin != nil && *l.Dash0Comorigin != "" {
		fmt.Printf("Origin: %s\n", *l.Dash0Comorigin)
	}
}
