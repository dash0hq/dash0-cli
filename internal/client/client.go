package client

import (
	"fmt"

	dash0 "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/config"
)

// NewClient creates a new Dash0 API client from the given configuration overrides.
// It uses config.ResolveConfiguration to handle the configuration resolution hierarchy:
// environment variables > command-line flags > active profile
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

// HandleAPIError provides user-friendly error messages for API errors.
// It checks for common error types and returns descriptive messages.
func HandleAPIError(err error) error {
	if err == nil {
		return nil
	}

	if dash0.IsNotFound(err) {
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
