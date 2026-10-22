package datasets

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/asset"
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

	format, err := parseGetFormat(flags.Output)
	if err != nil {
		return err
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset, err := apiClient.GetDataset(ctx, originOrID)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: "dataset", AssetID: originOrID})
	}
	asset.SortDatasetPermissions(dataset)

	switch format {
	case getFormatJSON:
		return printJSON(dataset)
	case getFormatYAML:
		data, err := sigsyaml.Marshal(dataset)
		if err != nil {
			return fmt.Errorf("failed to marshal dataset as YAML: %w", err)
		}
		fmt.Print(string(data))
		return nil
	default:
		printDetails(dataset)
		return nil
	}
}

type getFormat string

const (
	getFormatTable getFormat = "table"
	getFormatJSON  getFormat = "json"
	getFormatYAML  getFormat = "yaml"
)

func parseGetFormat(s string) (getFormat, error) {
	switch strings.ToLower(s) {
	case "":
		if agentmode.Enabled {
			return getFormatJSON, nil
		}
		return getFormatTable, nil
	case "table":
		return getFormatTable, nil
	case "json":
		return getFormatJSON, nil
	case "yaml", "yml":
		return getFormatYAML, nil
	default:
		return "", fmt.Errorf("unknown output format: %s (valid formats: table, json, yaml)", s)
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
	if origin := asset.DatasetOrigin(dataset); origin != "" {
		fmt.Printf("Origin: %s\n", origin)
	}
}
