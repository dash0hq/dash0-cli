package client

import (
	"context"

	"github.com/dash0hq/dash0-cli/internal/config"
)

// ResolveDataset returns the dataset to use for API calls, checking (in order):
// 1. The CLI flag value (flagDataset)
// 2. The configuration from context (profile + DASH0_DATASET env var)
// Returns nil when no dataset is configured or when the dataset is "default",
// since the API uses "default" implicitly when no dataset parameter is sent.
func ResolveDataset(ctx context.Context, flagDataset string) *string {
	if flagDataset != "" {
		return DatasetPtr(flagDataset)
	}
	if cfg := config.FromContext(ctx); cfg != nil && cfg.Dataset != "" {
		return DatasetPtr(cfg.Dataset)
	}
	return nil
}

// DatasetPtr converts a dataset string to a pointer, returning nil for empty
// strings and for "default" (the API uses "default" implicitly when no dataset
// parameter is sent).
func DatasetPtr(dataset string) *string {
	if dataset == "" || dataset == "default" {
		return nil
	}
	return &dataset
}
