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

type createFlags struct {
	ApiUrl    string
	AuthToken string
	File      string
	DryRun    bool
}

func newCreateCmd() *cobra.Command {
	flags := &createFlags{}

	cmd := &cobra.Command{
		Use:     "create -f <file>",
		Aliases: []string{"add"},
		Short:   "[experimental] Create a notification channel from a file",
		Long: `Create a new notification channel from a YAML or JSON definition file. Use '-f -' to read from stdin.

If the definition contains a dash0.com/origin label, the channel is created or replaced (PUT).
Otherwise, a new channel is created (POST) and the server assigns an ID.` + internal.CONFIG_HINT,
		Example: `  # Create from a YAML file
  dash0 --experimental notification-channels create -f channel.yaml

  # Create from stdin
  cat channel.yaml | dash0 --experimental notification-channels create -f -

  # Validate without creating
  dash0 --experimental notification-channels create -f channel.yaml --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runCreate(cmd.Context(), flags)
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().StringVarP(&flags.File, "file", "f", "", "Path to YAML or JSON definition file (use '-' for stdin)")
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false, "Validate without creating/updating")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

func runCreate(ctx context.Context, flags *createFlags) error {
	var channel dash0api.NotificationChannelDefinition
	if err := asset.ReadDefinition(flags.File, &channel, os.Stdin); err != nil {
		return fmt.Errorf("failed to read notification channel definition: %w", err)
	}

	if flags.DryRun {
		fmt.Println("Dry run: notification channel definition is valid")
		return nil
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	result, importErr := asset.ImportNotificationChannel(ctx, apiClient, &channel)
	if importErr != nil {
		return client.HandleAPIError(importErr, client.ErrorContext{
			AssetType: "notification channel",
			AssetName: dash0api.GetNotificationChannelName(&channel),
		})
	}

	if result.ID != "" {
		fmt.Printf("Notification channel %q %s (id: %s).\n", result.Name, result.Action, result.ID)
	} else {
		fmt.Printf("Notification channel %q %s.\n", result.Name, result.Action)
	}
	return nil
}
