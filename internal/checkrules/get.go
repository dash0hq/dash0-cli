package checkrules

import (
	"context"
	"fmt"
	"os"

	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/output"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/spf13/cobra"
)

func newGetCmd() *cobra.Command {
	var flags asset.GetFlags

	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get a check rule by ID",
		Long:  `Retrieve a check rule definition by its ID`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGet(cmd.Context(), args[0], &flags)
		},
	}

	asset.RegisterGetFlags(cmd, &flags)
	return cmd
}

func runGet(ctx context.Context, id string, flags *asset.GetFlags) error {
	apiUrl := client.ResolveApiUrl(ctx, flags.ApiUrl)
	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	rule, err := apiClient.GetCheckRule(ctx, id, client.ResolveDataset(ctx, flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "check rule",
			AssetID:   id,
		})
	}

	// The API does not return the rule ID in the response body. Restore it
	// so that exported YAML can be re-applied (the import API uses the ID
	// for upsert).
	if rule.Id == nil {
		rule.Id = &id
	}

	format, err := output.ParseFormat(flags.Output)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(format, os.Stdout)

	switch format {
	case output.FormatJSON, output.FormatYAML:
		return formatter.Print(rule)
	default:
		fmt.Printf("Name: %s\n", rule.Name)
		dataset := ""
		if rule.Dataset != nil {
			dataset = *rule.Dataset
		}
		fmt.Printf("Dataset: %s\n", dataset)
		fmt.Printf("Expression: %s\n", rule.Expression)
		if rule.Enabled != nil {
			fmt.Printf("Enabled: %t\n", *rule.Enabled)
		}
		if rule.Description != nil {
			fmt.Printf("Description: %s\n", *rule.Description)
		}
		if deeplinkURL := asset.DeeplinkURL(apiUrl, "check rule", id); deeplinkURL != "" {
			fmt.Printf("URL: %s\n", deeplinkURL)
		}
		return nil
	}
}
