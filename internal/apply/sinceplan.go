package apply

import (
	"context"

	gitutil "github.com/dash0hq/dash0-cli/internal/git"
)

// computeDeletionPlan computes flags.Since's deletion plan. The logic lives in
// internal/git so diff --since previews exactly what apply --since deletes.
func computeDeletionPlan(ctx context.Context, flags *applyFlags) (*gitutil.SincePlan, error) {
	return gitutil.ComputeSincePlan(ctx, flags.File, flags.Since)
}
