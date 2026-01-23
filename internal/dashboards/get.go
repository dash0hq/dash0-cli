package dashboards

import (
	"context"
	"fmt"
	"os"

	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/output"
	"github.com/dash0hq/dash0-cli/internal/resource"
	"github.com/spf13/cobra"
)

func newGetCmd() *cobra.Command {
	var flags resource.GetFlags

	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get a dashboard by ID",
		Long:  `Retrieve a dashboard definition by its ID`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGet(cmd.Context(), args[0], &flags)
		},
	}

	resource.RegisterGetFlags(cmd, &flags)
	return cmd
}

func runGet(ctx context.Context, id string, flags *resource.GetFlags) error {
	apiClient, err := client.NewClient(flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dashboard, err := apiClient.GetDashboard(ctx, id, client.DatasetPtr(flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err)
	}

	// Format output
	format, err := output.ParseFormat(flags.Output)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(format, os.Stdout)

	switch format {
	case output.FormatJSON, output.FormatYAML:
		return formatter.Print(dashboard)
	default:
		// For table format, print key details
		fmt.Printf("Kind: %s\n", dashboard.Kind)
		fmt.Printf("Name: %s\n", dashboard.Metadata.Name)
		if dashboard.Metadata.CreatedAt != nil {
			fmt.Printf("Created: %s\n", dashboard.Metadata.CreatedAt.Format("2006-01-02 15:04:05"))
		}
		if dashboard.Metadata.UpdatedAt != nil {
			fmt.Printf("Updated: %s\n", dashboard.Metadata.UpdatedAt.Format("2006-01-02 15:04:05"))
		}
		return nil
	}
}
