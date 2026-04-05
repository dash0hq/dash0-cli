package client

import (
	"context"
	"errors"
	"fmt"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-api-client-go/profiles"
	"github.com/dash0hq/dash0-cli/internal/version"
)

// NewClientFromContext creates a new Dash0 API client using configuration from context.
// Flag overrides (apiUrl, authToken) are applied on top of the context configuration.
func NewClientFromContext(ctx context.Context, apiUrl, authToken string) (dash0api.Client, error) {
	cfg := profiles.FromContext(ctx)
	if cfg == nil {
		// Fallback to ResolveConfiguration if not in context
		resolved, err := profiles.ResolveConfiguration(apiUrl, authToken)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve configuration: %w", err)
		}
		apiUrl = resolved.ApiUrl
		authToken = resolved.AuthToken
	} else {
		// Apply flag overrides on top of context configuration
		if apiUrl == "" {
			apiUrl = cfg.ApiUrl
		}
		if authToken == "" {
			authToken = cfg.AuthToken
		}
	}

	client, err := dash0api.NewClient(
		dash0api.WithApiUrl(apiUrl),
		dash0api.WithAuthToken(authToken),
		dash0api.WithUserAgent(version.UserAgent()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %w", err)
	}

	return client, nil
}

// NewOtlpClientFromContext creates a new Dash0 API client configured for OTLP using configuration from context.
// Flag overrides (otlpUrl, authToken) are applied on top of the context configuration.
func NewOtlpClientFromContext(ctx context.Context, otlpUrl, authToken string) (dash0api.Client, error) {
	cfg := profiles.FromContext(ctx)

	var finalOtlpUrl, finalAuthToken string
	if cfg != nil {
		finalOtlpUrl = cfg.OtlpUrl
		finalAuthToken = cfg.AuthToken
	}

	// Apply flag overrides
	if otlpUrl != "" {
		finalOtlpUrl = otlpUrl
	}
	if authToken != "" {
		finalAuthToken = authToken
	}

	if finalOtlpUrl == "" {
		return nil, fmt.Errorf("otlp-url is required; provide it as a flag, environment variable, or configure a profile")
	}
	if finalAuthToken == "" {
		return nil, fmt.Errorf("auth-token is required; provide it as a flag, environment variable, or configure a profile")
	}

	client, err := dash0api.NewClient(
		dash0api.WithOtlpEndpoint(dash0api.OtlpEncodingJson, finalOtlpUrl),
		dash0api.WithAuthToken(finalAuthToken),
		dash0api.WithUserAgent(version.UserAgent()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP client: %w", err)
	}

	return client, nil
}

// ResolveApiUrl returns the effective API URL from context and flag overrides.
// This is useful when the resolved URL is needed outside of client creation
// (e.g. for constructing deeplink URLs).
func ResolveApiUrl(ctx context.Context, flagApiUrl string) string {
	if flagApiUrl != "" {
		return flagApiUrl
	}
	if cfg := profiles.FromContext(ctx); cfg != nil && cfg.ApiUrl != "" {
		return cfg.ApiUrl
	}
	return ""
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

	if dash0api.IsNotFound(err) {
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
	if dash0api.IsUnauthorized(err) {
		return formatAPIError("authentication failed; check your auth token", err)
	}
	if dash0api.IsForbidden(err) {
		return formatAPIError("access denied; check your permissions", err)
	}
	if dash0api.IsBadRequest(err) {
		return formatAPIError("invalid request", err)
	}
	if dash0api.IsConflict(err) {
		assetType := getAssetType()
		identifier := getIdentifier()
		if assetType != "" {
			if identifier != "" {
				return formatAPIError(fmt.Sprintf("%s %q already exists or conflicts with existing asset", assetType, identifier), err)
			}
			return formatAPIError(fmt.Sprintf("%s already exists or conflicts with existing asset", assetType), err)
		}
		return formatAPIError("asset conflict", err)
	}
	if dash0api.IsRateLimited(err) {
		return formatAPIError("rate limited; please try again later", err)
	}
	if dash0api.IsServerError(err) {
		return formatAPIError("server error; please try again later", err)
	}

	return err
}

// formatAPIError builds a user-friendly error message. When the underlying
// error is an APIError with a non-empty body, the output uses a two-line
// format with the status metadata on the first line and the response body
// indented on the second:
//
//	invalid request (status: 400, trace_id: abc123):
//	  {"error": {"message": "..."}}
//
// When the body is empty, it falls back to wrapping the original error.
func formatAPIError(prefix string, err error) error {
	var apiErr *dash0api.APIError
	if !errors.As(err, &apiErr) {
		return fmt.Errorf("%s: %w", prefix, err)
	}

	body := strings.TrimSpace(apiErr.Body)
	if body == "" {
		return fmt.Errorf("%s: %w", prefix, err)
	}

	const maxBodyLen = 500
	if len(body) > maxBodyLen {
		body = body[:maxBodyLen] + "..."
	}

	statusLine := fmt.Sprintf("%s (status: %d", prefix, apiErr.StatusCode)
	if apiErr.TraceID != "" {
		statusLine += ", trace_id: " + apiErr.TraceID
	}
	statusLine += "):\n  " + body

	return fmt.Errorf("%s", statusLine)
}

