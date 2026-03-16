package recordingrulegroups

import (
	"context"
	"fmt"
	"os"

	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/output"
	"github.com/spf13/cobra"
)

func newGetCmd() *cobra.Command {
	var flags asset.GetFlags

	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get a recording rule group by ID",
		Long:  `Retrieve a recording rule group definition by its origin or ID.` + internal.CONFIG_HINT,
		Example: `  # Show recording rule group summary
  dash0 recording-rule-groups get <id>

  # Export as YAML (suitable for re-applying)
  dash0 recording-rule-groups get <id> -o yaml > group.yaml

  # Export as JSON
  dash0 recording-rule-groups get <id> -o json`,
		Args: cobra.ExactArgs(1),
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

	group, err := apiClient.GetRecordingRuleGroup(ctx, id, client.ResolveDataset(ctx, flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "recording rule group",
			AssetID:   id,
		})
	}

	format, err := output.ParseFormat(flags.Output)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(format, os.Stdout)

	switch format {
	case output.FormatJSON, output.FormatYAML:
		return formatter.Print(group)
	default:
		fmt.Printf("Name: %s\n", asset.ExtractRecordingRuleGroupName(group))
		fmt.Printf("Enabled: %t\n", group.Spec.Enabled)
		fmt.Printf("Interval: %s\n", group.Spec.Interval)
		if group.Metadata.Labels != nil {
			if group.Metadata.Labels.Dash0Comdataset != nil {
				fmt.Printf("Dataset: %s\n", *group.Metadata.Labels.Dash0Comdataset)
			}
			if group.Metadata.Labels.Dash0Comorigin != nil {
				fmt.Printf("Origin: %s\n", *group.Metadata.Labels.Dash0Comorigin)
			}
		}
		if deeplinkURL := asset.DeeplinkURL(apiUrl, "recording rule group", id); deeplinkURL != "" {
			fmt.Printf("URL: %s\n", deeplinkURL)
		}
		return nil
	}
}
