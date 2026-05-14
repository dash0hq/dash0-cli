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
		var prefix string
		switch {
		case assetType != "" && identifier != "":
			prefix = fmt.Sprintf("%s %q not found", assetType, identifier)
		case assetType != "":
			prefix = fmt.Sprintf("%s not found", assetType)
		default:
			prefix = "asset not found"
		}
		return formatAPIError(prefix, err)
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
// error is an APIError, the output uses a two-line format with the status
// metadata on the first line and the parsed server message indented on the
// second:
//
//	invalid request (status: 400, trace_id: abc123):
//	  The submitted check rule has an invalid expression.
//
// The parsed APIError.Message (extracted by the SDK from the nested
// { "error": { "message": ... } } shape) is preferred. When no message was
// parsed, the raw response body is used as a fallback. When neither is
// available, only the status line is emitted so the trace ID is still surfaced.
func formatAPIError(prefix string, err error) error {
	var apiErr *dash0api.APIError
	if !errors.As(err, &apiErr) {
		return fmt.Errorf("%s: %w", prefix, err)
	}

	detail := strings.TrimSpace(apiErr.Message)
	if detail == "" {
		detail = strings.TrimSpace(apiErr.Body)
	}

	const maxDetailLen = 500
	if len(detail) > maxDetailLen {
		detail = detail[:maxDetailLen] + "..."
	}

	statusLine := fmt.Sprintf("%s (status: %d", prefix, apiErr.StatusCode)
	if apiErr.TraceID != "" {
		statusLine += ", trace_id: " + apiErr.TraceID
	}
	statusLine += ")"

	if detail == "" {
		return fmt.Errorf("%s", statusLine)
	}
	return fmt.Errorf("%s:\n  %s", statusLine, detail)
}

