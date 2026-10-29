package authtokens

import (
	"encoding/json"
	"fmt"
	"io"
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
		Short: "[experimental] Get an auth token by ID or origin",
		Long:  `Retrieve an auth token definition by its ID or origin. The output never contains the secret.` + internal.CONFIG_HINT,
		Example: `  # Get auth token details
  dash0 --experimental auth-tokens get <id>

  # Output as JSON
  dash0 --experimental auth-tokens get <id> -o json

  # Output as YAML
  dash0 --experimental auth-tokens get <id> -o yaml`,
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

	token, err := apiClient.GetAuthToken(ctx, originOrID)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: assetType, AssetID: originOrID})
	}
	// The API never returns the secret on a read; drop it anyway in case that changes.
	takeSecret(token)

	switch format {
	case getFormatJSON:
		return printJSON(os.Stdout, token)
	case getFormatYAML:
		return printYAML(os.Stdout, token)
	default:
		printDetails(token)
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

func printJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func printYAML(w io.Writer, value any) error {
	data, err := sigsyaml.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal auth token as YAML: %w", err)
	}
	_, err = w.Write(data)
	return err
}

func printDetails(token *dash0api.AuthTokenResponseDefinition) {
	fmt.Printf("Kind:    %s\n", token.Kind)
	fmt.Printf("Name:    %s\n", token.Metadata.Name)
	if id := idOf(&token.Metadata); id != "" {
		fmt.Printf("ID:      %s\n", id)
	}
	if origin := originOf(&token.Metadata); origin != "" {
		fmt.Printf("Origin:  %s\n", origin)
	}
	fmt.Printf("Actions: %s\n", actionsOf(token))
	if annotations := token.Metadata.Annotations; annotations != nil && annotations.Dash0ComcreatedAt != nil {
		fmt.Printf("Created: %s\n", annotations.Dash0ComcreatedAt.Format("2006-01-02 15:04:05"))
	}
}
