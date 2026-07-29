package client

import (
	"context"
	"os"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-api-client-go/profiles"
)

// ResolveDataset returns the dataset to use for API calls, checking (in order):
// 1. The CLI flag value (flagDataset)
// 2. The configuration from context (profile + DASH0_DATASET env var)
// 3. The DASH0_DATASET env var, then a freshly resolved configuration, when
// the context carries no configuration at all
// Returns nil when no dataset is configured or when the dataset is "default",
// since the API uses "default" implicitly when no dataset parameter is sent.
//
// Step 3 exists because [NewClientFromContext] falls back to
// [profiles.ResolveConfiguration] when the context configuration is missing.
// Without the same fallback here, a command whose config failed to load would
// still reach the API with credentials recovered from flags or env vars, but
// silently without a dataset — so the request would hit the default dataset
// instead of the configured one.
func ResolveDataset(ctx context.Context, flagDataset string) *string {
	if flagDataset != "" {
		return dash0api.DatasetPtr(flagDataset)
	}
	if cfg := profiles.FromContext(ctx); cfg != nil {
		if cfg.Dataset != "" {
			return dash0api.DatasetPtr(cfg.Dataset)
		}
		return nil
	}
	if envDataset := os.Getenv(profiles.EnvDataset); envDataset != "" {
		return dash0api.DatasetPtr(envDataset)
	}
	if cfg, err := profiles.ResolveConfigurationContext(ctx, "", ""); err == nil && cfg.Dataset != "" {
		return dash0api.DatasetPtr(cfg.Dataset)
	}
	return nil
}
