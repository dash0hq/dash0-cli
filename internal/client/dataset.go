package client

import (
	"context"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-api-client-go/profiles"
)

// ResolveDataset returns the dataset to use for API calls, checking (in order):
// 1. The CLI flag value (flagDataset)
// 2. The configuration from context (profile + DASH0_DATASET env var)
// Returns nil when no dataset is configured or when the dataset is "default",
// since the API uses "default" implicitly when no dataset parameter is sent.
func ResolveDataset(ctx context.Context, flagDataset string) *string {
	if flagDataset != "" {
		return dash0api.DatasetPtr(flagDataset)
	}
	if cfg := profiles.FromContext(ctx); cfg != nil && cfg.Dataset != "" {
		return dash0api.DatasetPtr(cfg.Dataset)
	}
	return nil
}
