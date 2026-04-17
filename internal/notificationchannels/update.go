package notificationchannels

import (
	"context"
	"fmt"
	"os"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/spf13/cobra"
)

type updateFlags struct {
	ApiUrl    string
	AuthToken string
	File      string
	DryRun    bool
}

func newUpdateCmd() *cobra.Command {
	flags := &updateFlags{}

	cmd := &cobra.Command{
		Use:   "update [id] -f <file>",
		Short: "[experimental] Update a notification channel from a file",
		Long: `Update an existing notification channel from a YAML or JSON definition file. Use '-f -' to read from stdin.

If the ID argument is omitted, the ID is extracted from the file content.` + internal.CONFIG_HINT,
		Example: `  # Update a notification channel from a file
  dash0 --experimental notification-channels update <id> -f channel.yaml

  # Update using the ID from the file
  dash0 --experimental notification-channels update -f channel.yaml

  # Export, edit, and update
  dash0 --experimental notification-channels get <id> -o yaml > channel.yaml
  # edit channel.yaml
  dash0 --experimental notification-channels update -f channel.yaml`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runUpdate(cmd.Context(), args, flags)
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().StringVarP(&flags.File, "file", "f", "", "Path to YAML or JSON definition file (use '-' for stdin)")
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false, "Validate without updating")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

func runUpdate(ctx context.Context, args []string, flags *updateFlags) error {
	var channel dash0api.NotificationChannelDefinition
	if err := asset.ReadDefinition(flags.File, &channel, os.Stdin); err != nil {
		return fmt.Errorf("failed to read notification channel definition: %w", err)
	}

	var id string
	fileID := dash0api.GetNotificationChannelID(&channel)
	if len(args) == 1 {
		id = args[0]
		if fileID != "" && fileID != id {
			return fmt.Errorf("the ID argument %q does not match the ID in the file %q", id, fileID)
		}
	} else {
		id = fileID
		if id == "" {
			return fmt.Errorf("no notification channel ID provided as argument, and the file does not contain an ID")
		}
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	before, err := apiClient.GetNotificationChannel(ctx, id)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "notification channel",
			AssetID:   id,
		})
	}

	if flags.DryRun {
		return asset.PrintDiff(os.Stdout, "Notification channel", channel.Metadata.Name, before, &channel)
	}

	result, err := apiClient.UpdateNotificationChannel(ctx, id, &channel)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "notification channel",
			AssetID:   id,
			AssetName: dash0api.GetNotificationChannelName(&channel),
		})
	}

	return asset.PrintDiff(os.Stdout, "Notification channel", dash0api.GetNotificationChannelName(result), before, result)
}
