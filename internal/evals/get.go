package evals

import (
	"context"
	"fmt"
	"os"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/dash0hq/dash0-cli/internal/output"
	"github.com/spf13/cobra"
)

func newGetCmd() *cobra.Command {
	var flags asset.GetFlags

	cmd := &cobra.Command{
		Use:   "get <id-or-origin>",
		Short: "[experimental] Get an eval by ID or origin",
		Long: `Retrieve a generative AI evaluation (eval) by its ID or its dash0.com/origin label.
A deleted eval is still returned, and the output shows when it was deleted.` + internal.CONFIG_HINT,
		Example: `  # Show eval summary
  dash0 --experimental evals get <id>

  # Look up by the dash0.com/origin label instead
  dash0 --experimental evals get <origin>

  # Export as YAML (suitable for re-applying)
  dash0 --experimental evals get <id> -o yaml > eval.yaml

  # Export as JSON
  dash0 --experimental evals get <id> -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runGet(cmd.Context(), args[0], &flags)
		},
	}

	asset.RegisterGetFlags(cmd, &flags)
	return cmd
}

func runGet(ctx context.Context, originOrID string, flags *asset.GetFlags) error {
	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	evaluation, err := apiClient.GetGenerativeAiEvaluation(ctx, originOrID, client.ResolveDataset(ctx, flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: assetType, AssetID: originOrID})
	}

	format, err := output.ParseFormat(flags.Output)
	if err != nil {
		return err
	}

	switch format {
	case output.FormatJSON, output.FormatYAML:
		return output.NewFormatter(format, os.Stdout).Print(evaluation)
	default:
		fmt.Printf("Kind: %s\n", evaluation.Kind)
		fmt.Printf("Name: %s\n", dash0api.GetGenerativeAiEvaluationName(evaluation))
		fmt.Printf("ID: %s\n", dash0api.GetGenerativeAiEvaluationID(evaluation))
		fmt.Printf("Dataset: %s\n", dash0api.GetGenerativeAiEvaluationDataset(evaluation))
		if deletedAt := asset.GenerativeAiEvaluationDeletedAt(evaluation); deletedAt != nil {
			fmt.Printf("Deleted: %s\n", deletedAt.UTC().Format("2006-01-02 15:04:05"))
		}
		// Origin is the eval upsert key, so it is the field users script against.
		if origin := dash0api.GetGenerativeAiEvaluationOrigin(evaluation); origin != "" {
			fmt.Printf("Origin: %s\n", origin)
		}
		fmt.Printf("Enabled: %t\n", evaluation.Spec.Enabled)
		fmt.Printf("Judge model: %s\n", evaluation.Spec.Judge.Model)
		fmt.Printf("Sample rate: %g\n", evaluation.Spec.Sample.Rate)
		if description := evaluation.Spec.Display.Description; description != nil && *description != "" {
			fmt.Printf("Description: %s\n", *description)
		}
		return nil
	}
}
