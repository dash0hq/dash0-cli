package notificationchannels

import "github.com/spf13/cobra"

// NewNotificationChannelsCmd creates the notification-channels parent command.
func NewNotificationChannelsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "notification-channels",
		Short: "[experimental] Manage notification channels",
		Long:  `Create, list, get, update, and delete notification channels in your Dash0 organization.`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}
