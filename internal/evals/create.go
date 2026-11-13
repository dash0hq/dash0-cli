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
	"github.com/spf13/cobra"
)

func newCreateCmd() *cobra.Command {
	var flags asset.FileInputFlags

	cmd := &cobra.Command{
		Use:     "create -f <file>",
		Aliases: []string{"add"},
		Short:   "[experimental] Create an eval from a file",
		Long: `Create a generative AI evaluation (eval) from a YAML or JSON definition file. Use '-f -' to read from stdin.

When the file sets metadata.labels["dash0.com/origin"], the eval is created or replaced at that origin, so running the command again does not create a duplicate.` + internal.CONFIG_HINT,
		Example: `  # Create from a YAML file
  dash0 --experimental evals create -f eval.yaml

  # Create from stdin
  cat eval.yaml | dash0 --experimental evals create -f -

  # Validate without creating
  dash0 --experimental evals create -f eval.yaml --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runCreate(cmd.Context(), &flags)
		},
	}

	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runCreate(ctx context.Context, flags *asset.FileInputFlags) error {
	data, err := asset.ReadRawInputForWrite(flags.File, os.Stdin, "generativeaievaluation")
	if err != nil {
		return fmt.Errorf("failed to read eval definition: %w", err)
	}
	evaluation, err := asset.ParseGenerativeAiEvaluation(data)
	if err != nil {
		return err
	}

	if flags.DryRun {
		fmt.Println("Dry run: eval definition is valid")
		return nil
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	name := dash0api.GetGenerativeAiEvaluationName(evaluation)
	result, err := asset.ImportGenerativeAiEvaluation(ctx, apiClient, evaluation, client.ResolveDataset(ctx, flags.Dataset))
	if err != nil {
		if explained := asset.ExplainGenerativeAiEvaluationError(err, name); explained != nil {
			return explained
		}
		return client.HandleAPIError(err, client.ErrorContext{AssetType: assetType, AssetName: name})
	}

	fmt.Printf("%s %q %s\n", assetType, result.Name, result.Action)
	return nil
}
