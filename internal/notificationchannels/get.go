package notificationchannels

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
		Short: "[experimental] Get a notification channel by ID",
		Long:  `Retrieve a notification channel definition by its ID or origin.` + internal.CONFIG_HINT,
		Example: `  # Get notification channel details
  dash0 --experimental notification-channels get <id>

  # Output as JSON
  dash0 --experimental notification-channels get <id> -o json

  # Output as YAML
  dash0 --experimental notification-channels get <id> -o yaml`,
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

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	channel, err := apiClient.GetNotificationChannel(ctx, originOrID)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "notification channel",
			AssetID:   originOrID,
		})
	}

	dash0api.SetNotificationChannelIDIfAbsent(channel, originOrID)

	switch strings.ToLower(flags.Output) {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(channel)
	case "yaml", "yml":
		data, err := sigsyaml.Marshal(channel)
		if err != nil {
			return fmt.Errorf("failed to marshal notification channel as YAML: %w", err)
		}
		fmt.Print(string(data))
		return nil
	case "":
		if agentmode.Enabled {
			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")
			return encoder.Encode(channel)
		}
		return printNotificationChannelDetails(channel)
	case "table":
		return printNotificationChannelDetails(channel)
	default:
		return fmt.Errorf("unknown output format: %s (valid formats: table, json, yaml)", flags.Output)
	}
}

func printNotificationChannelDetails(channel *dash0api.NotificationChannelDefinition) error {
	id := dash0api.GetNotificationChannelID(channel)
	origin := dash0api.GetNotificationChannelOrigin(channel)

	fmt.Printf("Kind:  %s\n", channel.Kind)
	fmt.Printf("Name:  %s\n", dash0api.GetNotificationChannelName(channel))
	fmt.Printf("Type:  %s\n", channel.Spec.Type)
	if id != "" {
		fmt.Printf("ID:    %s\n", id)
	}
	if origin != "" {
		fmt.Printf("Origin: %s\n", origin)
	}
	return nil
}
