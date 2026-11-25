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

func newUpdateCmd() *cobra.Command {
	var flags asset.FileInputFlags

	cmd := &cobra.Command{
		Use:   "update [id-or-origin] -f <file>",
		Short: "[experimental] Update an eval from a file",
		Long: `Update an existing generative AI evaluation (eval) from a YAML or JSON definition file. Use '-f -' to read from stdin.

If the argument is omitted, the file's dash0.com/id label is read first, then its dash0.com/origin label when the id is not found, as apply does.
The eval must exist; use create to make a new one.
metadata.name and spec.judge.output.kind cannot change after the eval is created.` + internal.CONFIG_HINT,
		Example: `  # Update an eval from a file
  dash0 --experimental evals update <id> -f eval.yaml

  # Update using the origin or ID from the file
  dash0 --experimental evals update -f eval.yaml

  # Preview the changes
  dash0 --experimental evals update -f eval.yaml --dry-run

  # Export, edit, and update
  dash0 --experimental evals get <id> -o yaml > eval.yaml
  # edit eval.yaml
  dash0 --experimental evals update -f eval.yaml`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runUpdate(cmd.Context(), args, &flags)
		},
	}

	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runUpdate(ctx context.Context, args []string, flags *asset.FileInputFlags) error {
	data, err := asset.ReadRawInput(flags.File, os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read eval definition: %w", err)
	}
	evaluation, err := asset.ParseGenerativeAiEvaluation(data)
	if err != nil {
		return err
	}

	origin := dash0api.GetGenerativeAiEvaluationOrigin(evaluation)
	updateKey := asset.UpdateKey{Noun: "eval", Upsert: evaluation}
	identity := asset.GenerativeAiEvaluationIdentity(evaluation, nil)
	if err := asset.CheckRouteUpdate(args, updateKey, identity); err != nil {
		return err
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)

	// PUT creates an eval for an unknown origin, so the read turns a mistyped key
	// into a not-found error. It takes the file's id first, then its origin, as
	// apply does.
	get := func(ctx context.Context, key string) (*dash0api.GenerativeAiEvaluationDefinition, error) {
		return apiClient.GetGenerativeAiEvaluation(ctx, key, dataset)
	}
	key, before, err := asset.RouteUpdate(ctx, args, updateKey, identity, get)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: assetType, AssetID: key})
	}

	if stored := dash0api.GetGenerativeAiEvaluationOrigin(before); origin != "" && origin != stored {
		return fmt.Errorf("argument %q is eval %q with origin %q, but the file's origin is %q", key, dash0api.GetGenerativeAiEvaluationID(before), stored, origin)
	}

	// The strip removes dash0.com/version, so the PUT overwrites unconditionally,
	// and dash0.com/id, so an exported document can be sent back as is.
	dash0api.StripGenerativeAiEvaluationServerFields(evaluation)
	name := dash0api.GetGenerativeAiEvaluationName(evaluation)
	id := dash0api.GetGenerativeAiEvaluationID(before)

	if flags.DryRun {
		return asset.PrintUpdateOutcome(os.Stdout, asset.UpdateOutcome{
			DisplayKind:     assetType,
			Name:            name,
			ID:              id,
			Before:          before,
			After:           evaluation,
			BeforeDeletedAt: asset.GenerativeAiEvaluationDeletedAt(before),
			DryRun:          true,
		})
	}

	// The origin is sent only on create, so an update never changes it.
	asset.ClearUpdateOrigin(evaluation)
	result, err := apiClient.UpdateGenerativeAiEvaluation(ctx, key, evaluation, dataset)
	if err != nil {
		if explained := asset.ExplainGenerativeAiEvaluationError(err, name); explained != nil {
			return explained
		}
		return client.HandleAPIError(err, client.ErrorContext{AssetType: assetType, AssetID: key, AssetName: name})
	}

	return asset.PrintUpdateOutcome(os.Stdout, asset.UpdateOutcome{
		DisplayKind:     assetType,
		Name:            dash0api.GetGenerativeAiEvaluationName(result),
		ID:              id,
		Before:          before,
		After:           result,
		BeforeDeletedAt: asset.GenerativeAiEvaluationDeletedAt(before),
		AfterDeletedAt:  asset.GenerativeAiEvaluationDeletedAt(result),
	})
}
