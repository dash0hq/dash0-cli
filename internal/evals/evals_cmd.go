package evals

import (
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/spf13/cobra"
)

const assetType = asset.GenerativeAiEvaluationDisplayName

// NewEvalsCmd creates the evals parent command.
func NewEvalsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "evals",
		Aliases: []string{"generative-ai-evaluations"},
		Short:   "[experimental] Manage Dash0 generative AI evaluations (evals)",
		Long: `Create, list, get, update, and delete generative AI evaluations (evals) in Dash0.

An eval uses a judge model to score sampled generative AI spans. Eval documents use kind: Dash0GenerativeAiEvaluation.`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}
