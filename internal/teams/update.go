package teams

import (
	"fmt"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/spf13/cobra"
)

type updateFlags struct {
	ApiUrl    string
	AuthToken string
	Name      string
	ColorFrom string
	ColorTo   string
}

func newUpdateCmd() *cobra.Command {
	flags := &updateFlags{}

	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "[experimental] Update a team",
		Long:  `Update the display settings of a team (name, color).` + internal.CONFIG_HINT,
		Example: `  # Rename a team
  dash0 --experimental teams update <id> --name "New Name"

  # Update the team's color gradient
  dash0 --experimental teams update <id> \
      --color-from "#FF0000" --color-to "#00FF00"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runUpdate(cmd, args[0], flags)
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().StringVar(&flags.Name, "name", "", "New team name")
	cmd.Flags().StringVar(&flags.ColorFrom, "color-from", "", "Gradient start color (e.g. \"#FF0000\")")
	cmd.Flags().StringVar(&flags.ColorTo, "color-to", "", "Gradient end color (e.g. \"#00FF00\")")

	return cmd
}

func runUpdate(cmd *cobra.Command, originOrID string, flags *updateFlags) error {
	ctx := cmd.Context()

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	display := &dash0api.TeamDisplay{
		Name: flags.Name,
		Color: dash0api.Gradient{
			From: flags.ColorFrom,
			To:   flags.ColorTo,
		},
	}

	err = apiClient.UpdateTeamDisplay(ctx, originOrID, display)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "team",
			AssetID:   originOrID,
		})
	}

	fmt.Printf("Team %q updated\n", originOrID)
	return nil
}
