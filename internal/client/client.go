package client

import (
	"context"
	"fmt"

	dash0 "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/config"
)

// NewClientFromContext creates a new Dash0 API client using configuration from context.
// Flag overrides (apiUrl, authToken) are applied on top of the context configuration.
func NewClientFromContext(ctx context.Context, apiUrl, authToken string) (dash0.Client, error) {
	cfg := config.FromContext(ctx)
	if cfg == nil {
		// Fallback to ResolveConfiguration if not in context
		return NewClient(apiUrl, authToken)
	}

	// Apply flag overrides
	finalApiUrl := cfg.ApiUrl
	finalAuthToken := cfg.AuthToken
	if apiUrl != "" {
		finalApiUrl = apiUrl
	}
	if authToken != "" {
		finalAuthToken = authToken
	}

	client, err := dash0.NewClient(
		dash0.WithApiUrl(finalApiUrl),
		dash0.WithAuthToken(finalAuthToken),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %w", err)
	}

	return client, nil
}

// NewClient creates a new Dash0 API client from the given configuration overrides.
// It uses config.ResolveConfiguration to handle the configuration resolution hierarchy:
// environment variables > command-line flags > active profile
// Deprecated: Use NewClientFromContext for commands that run within the CLI context.
func NewClient(apiUrl, authToken string) (dash0.Client, error) {
	cfg, err := config.ResolveConfiguration(apiUrl, authToken)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve configuration: %w", err)
	}

	client, err := dash0.NewClient(
		dash0.WithApiUrl(cfg.ApiUrl),
		dash0.WithAuthToken(cfg.AuthToken),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %w", err)
	}

	return client, nil
}

// ErrorContext provides context about the asset involved in an error.
// This context is used to generate more specific and actionable error messages.
type ErrorContext struct {
	AssetType string // e.g., "dashboard", "check rule"
	AssetID   string // e.g., "a1b2c3d4-..." (optional, empty for create/list)
	AssetName string // e.g., "Production Overview" (optional, for user-friendly messages)
}

// HandleAPIError provides user-friendly error messages for API errors.
// It checks for common error types and returns descriptive messages.
// Optional context can be provided to include asset details in error messages.
func HandleAPIError(err error, ctx ...ErrorContext) error {
	if err == nil {
		return nil
	}

	// Helper to get the best identifier (prefer name over ID)
	getIdentifier := func() string {
		if len(ctx) > 0 {
			if ctx[0].AssetName != "" {
				return ctx[0].AssetName
			}
			return ctx[0].AssetID
		}
		return ""
	}

	// Helper to get asset type
	getAssetType := func() string {
		if len(ctx) > 0 {
			return ctx[0].AssetType
		}
		return ""
	}

	if dash0.IsNotFound(err) {
		assetType := getAssetType()
		identifier := getIdentifier()
		if assetType != "" {
			if identifier != "" {
				return fmt.Errorf("%s %q not found", assetType, identifier)
			}
			return fmt.Errorf("%s not found", assetType)
		}
		return fmt.Errorf("asset not found: %w", err)
	}
	if dash0.IsUnauthorized(err) {
		return fmt.Errorf("authentication failed; check your auth token: %w", err)
	}
	if dash0.IsForbidden(err) {
		return fmt.Errorf("access denied; check your permissions: %w", err)
	}
	if dash0.IsBadRequest(err) {
		return fmt.Errorf("invalid request: %w", err)
	}
	if dash0.IsConflict(err) {
		assetType := getAssetType()
		identifier := getIdentifier()
		if assetType != "" {
			if identifier != "" {
				return fmt.Errorf("%s %q already exists or conflicts with existing asset: %w", assetType, identifier, err)
			}
			return fmt.Errorf("%s already exists or conflicts with existing asset: %w", assetType, err)
		}
		return fmt.Errorf("asset conflict: %w", err)
	}
	if dash0.IsRateLimited(err) {
		return fmt.Errorf("rate limited; please try again later: %w", err)
	}
	if dash0.IsServerError(err) {
		return fmt.Errorf("server error; please try again later: %w", err)
	}

	return err
}

// DatasetPtr converts a dataset string to a pointer, returning nil for empty strings.
// This is a helper for API calls that accept optional dataset parameters.
func DatasetPtr(dataset string) *string {
	if dataset == "" {
		return nil
	}
	return &dataset
}
