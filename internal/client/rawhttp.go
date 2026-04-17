package client

import (
	"context"
	"fmt"
	"net/http"

	"github.com/dash0hq/dash0-api-client-go/profiles"
	"github.com/dash0hq/dash0-cli/internal/version"
)

// RawHTTPConfig holds the resolved configuration for raw HTTP calls against
// the Dash0 API.
type RawHTTPConfig struct {
	// HTTPClient is a plain net/http client ready to perform requests.
	HTTPClient *http.Client
	// ApiUrl is the resolved API base URL.
	ApiUrl string
	// AuthToken is the resolved bearer token.
	AuthToken string
	// Dataset is the dataset resolved from environment or active profile.
	// Flag overrides are the caller's responsibility so that "explicitly
	// unset" can be distinguished from "not provided".
	Dataset string
	// UserAgent is the User-Agent string to send with requests.
	UserAgent string
}

// NewRawHTTPConfig resolves the API URL, auth token and profile dataset from
// context, environment and flag overrides, returning a ready-to-use
// configuration for raw HTTP requests against the Dash0 API.
//
// This sits beside NewClientFromContext rather than wrapping the generated
// typed client: the typed client does not expose its underlying transport,
// and raw mode needs to emit arbitrary requests without the constraints of
// the generated surface.
func NewRawHTTPConfig(ctx context.Context, apiUrl, authToken string) (*RawHTTPConfig, error) {
	resolvedApiUrl := apiUrl
	resolvedAuthToken := authToken
	var resolvedDataset string

	if cfg := profiles.FromContext(ctx); cfg != nil {
		if resolvedApiUrl == "" {
			resolvedApiUrl = cfg.ApiUrl
		}
		if resolvedAuthToken == "" {
			resolvedAuthToken = cfg.AuthToken
		}
		resolvedDataset = cfg.Dataset
	} else {
		resolved, err := profiles.ResolveConfiguration(resolvedApiUrl, resolvedAuthToken)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve configuration: %w", err)
		}
		resolvedApiUrl = resolved.ApiUrl
		resolvedAuthToken = resolved.AuthToken
		resolvedDataset = resolved.Dataset
	}

	if resolvedApiUrl == "" {
		return nil, fmt.Errorf("api-url is required; provide it as a flag, environment variable, or configure a profile")
	}
	if resolvedAuthToken == "" {
		return nil, fmt.Errorf("auth-token is required; provide it as a flag, environment variable, or configure a profile")
	}

	return &RawHTTPConfig{
		HTTPClient: &http.Client{},
		ApiUrl:     resolvedApiUrl,
		AuthToken:  resolvedAuthToken,
		Dataset:    resolvedDataset,
		UserAgent:  version.UserAgent(),
	}, nil
}
