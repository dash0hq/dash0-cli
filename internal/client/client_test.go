package client

import (
	"context"
	"fmt"
	"os"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-api-client-go/profiles"
	"github.com/stretchr/testify/assert"
)

func TestNewClientFromContext_WithEnvVars(t *testing.T) {
	os.Setenv("DASH0_API_URL", "https://api.test.dash0.com")
	os.Setenv("DASH0_AUTH_TOKEN", "auth_test-token-12345")
	defer func() {
		os.Unsetenv("DASH0_API_URL")
		os.Unsetenv("DASH0_AUTH_TOKEN")
	}()

	client, err := NewClientFromContext(context.Background(), "", "")
	assert.NoError(t, err)
	assert.NotNil(t, client)
}

func TestNewClientFromContext_WithOverrides(t *testing.T) {
	os.Setenv("DASH0_API_URL", "https://api.base.dash0.com")
	os.Setenv("DASH0_AUTH_TOKEN", "auth_base-token-12345")
	defer func() {
		os.Unsetenv("DASH0_API_URL")
		os.Unsetenv("DASH0_AUTH_TOKEN")
	}()

	client, err := NewClientFromContext(context.Background(), "https://api.override.dash0.com", "auth_override-token-12345")
	assert.NoError(t, err)
	assert.NotNil(t, client)
}

func TestNewClientFromContext_MissingConfig(t *testing.T) {
	os.Unsetenv("DASH0_API_URL")
	os.Unsetenv("DASH0_AUTH_TOKEN")

	tempDir := t.TempDir()
	os.Setenv("DASH0_CONFIG_DIR", tempDir)
	defer os.Unsetenv("DASH0_CONFIG_DIR")

	client, err := NewClientFromContext(context.Background(), "", "")
	assert.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "no active profile configured")
}

func TestResolveDataset_FlagTakesPrecedence(t *testing.T) {
	cfg := &profiles.Configuration{
		ApiUrl:    "https://api.test.dash0.com",
		AuthToken: "auth_test-token-12345",
		Dataset:   "config-dataset",
	}
	ctx := profiles.WithConfiguration(context.Background(), cfg)

	result := ResolveDataset(ctx, "flag-dataset")
	assert.NotNil(t, result)
	assert.Equal(t, "flag-dataset", *result)
}

func TestResolveDataset_FallsBackToConfig(t *testing.T) {
	cfg := &profiles.Configuration{
		ApiUrl:    "https://api.test.dash0.com",
		AuthToken: "auth_test-token-12345",
		Dataset:   "config-dataset",
	}
	ctx := profiles.WithConfiguration(context.Background(), cfg)

	result := ResolveDataset(ctx, "")
	assert.NotNil(t, result)
	assert.Equal(t, "config-dataset", *result)
}

func TestResolveDataset_NilWhenEmpty(t *testing.T) {
	cfg := &profiles.Configuration{
		ApiUrl:    "https://api.test.dash0.com",
		AuthToken: "auth_test-token-12345",
	}
	ctx := profiles.WithConfiguration(context.Background(), cfg)

	result := ResolveDataset(ctx, "")
	assert.Nil(t, result)
}

func TestResolveDataset_NilForDefaultInConfig(t *testing.T) {
	cfg := &profiles.Configuration{
		ApiUrl:    "https://api.test.dash0.com",
		AuthToken: "auth_test-token-12345",
		Dataset:   "default",
	}
	ctx := profiles.WithConfiguration(context.Background(), cfg)

	result := ResolveDataset(ctx, "")
	assert.Nil(t, result)
}

func TestResolveDataset_NilForDefaultFlag(t *testing.T) {
	result := ResolveDataset(context.Background(), "default")
	assert.Nil(t, result)
}

func TestResolveDataset_DefaultFlagOverridesConfig(t *testing.T) {
	cfg := &profiles.Configuration{
		ApiUrl:    "https://api.test.dash0.com",
		AuthToken: "auth_test-token-12345",
		Dataset:   "config-dataset",
	}
	ctx := profiles.WithConfiguration(context.Background(), cfg)

	result := ResolveDataset(ctx, "default")
	assert.Nil(t, result)
}

func TestResolveDataset_NilWithoutContext(t *testing.T) {
	result := ResolveDataset(context.Background(), "")
	assert.Nil(t, result)
}

func TestNewRawHTTPConfig_DatasetFromEnvWithoutContext(t *testing.T) {
	t.Setenv("DASH0_API_URL", "https://api.test.dash0.com")
	t.Setenv("DASH0_AUTH_TOKEN", "auth_test-token-12345")
	t.Setenv("DASH0_DATASET", "env-dataset")
	t.Setenv("DASH0_CONFIG_DIR", t.TempDir())

	cfg, err := NewRawHTTPConfig(context.Background(), "", "")
	assert.NoError(t, err)
	assert.Equal(t, "https://api.test.dash0.com", cfg.ApiUrl)
	assert.Equal(t, "auth_test-token-12345", cfg.AuthToken)
	assert.Equal(t, "env-dataset", cfg.Dataset)
}

func TestNewRawHTTPConfig_DatasetFromContext(t *testing.T) {
	profileCfg := &profiles.Configuration{
		ApiUrl:    "https://api.test.dash0.com",
		AuthToken: "auth_test-token-12345",
		Dataset:   "profile-dataset",
	}
	ctx := profiles.WithConfiguration(context.Background(), profileCfg)

	cfg, err := NewRawHTTPConfig(ctx, "", "")
	assert.NoError(t, err)
	assert.Equal(t, "profile-dataset", cfg.Dataset)
}

func TestNewRawHTTPConfig_NoDatasetWhenUnset(t *testing.T) {
	t.Setenv("DASH0_API_URL", "https://api.test.dash0.com")
	t.Setenv("DASH0_AUTH_TOKEN", "auth_test-token-12345")
	t.Setenv("DASH0_CONFIG_DIR", t.TempDir())
	os.Unsetenv("DASH0_DATASET")

	cfg, err := NewRawHTTPConfig(context.Background(), "", "")
	assert.NoError(t, err)
	assert.Empty(t, cfg.Dataset)
}

func TestFormatAPIError_BodyOnSecondLine(t *testing.T) {
	apiErr := &dash0api.APIError{
		StatusCode: 400,
		Status:     "400 Bad Request",
		Body:       `{"error":{"code":400,"message":"Bad Request: origin mismatch"}}`,
		TraceID:    "abc123",
	}

	result := formatAPIError("invalid request", apiErr)
	expected := "invalid request (status: 400, trace_id: abc123):\n" +
		`  {"error":{"code":400,"message":"Bad Request: origin mismatch"}}`
	assert.Equal(t, expected, result.Error())
}

func TestFormatAPIError_BodyWithoutTraceID(t *testing.T) {
	apiErr := &dash0api.APIError{
		StatusCode: 400,
		Status:     "400 Bad Request",
		Body:       `{"details": "field 'expression' is required"}`,
	}

	result := formatAPIError("invalid request", apiErr)
	expected := "invalid request (status: 400):\n" +
		`  {"details": "field 'expression' is required"}`
	assert.Equal(t, expected, result.Error())
}

func TestFormatAPIError_EmptyBody(t *testing.T) {
	apiErr := &dash0api.APIError{
		StatusCode: 400,
		Status:     "400 Bad Request",
		TraceID:    "abc123",
	}

	result := formatAPIError("invalid request", apiErr)
	assert.Contains(t, result.Error(), "invalid request")
	assert.Contains(t, result.Error(), "400 Bad Request")
}

func TestFormatAPIError_NonAPIError(t *testing.T) {
	err := fmt.Errorf("some other error")
	result := formatAPIError("invalid request", err)
	assert.Equal(t, "invalid request: some other error", result.Error())
}

func TestFormatAPIError_WrappedAPIError(t *testing.T) {
	apiErr := &dash0api.APIError{
		StatusCode: 400,
		Status:     "400 Bad Request",
		Body:       `{"details": "missing required field"}`,
		TraceID:    "def456",
	}
	wrapped := fmt.Errorf("update dashboard failed: %w", apiErr)

	result := formatAPIError("invalid request", wrapped)
	assert.Contains(t, result.Error(), "invalid request (status: 400, trace_id: def456):")
	assert.Contains(t, result.Error(), "missing required field")
}

func TestFormatAPIError_LongBodyTruncated(t *testing.T) {
	longBody := string(make([]byte, 600))
	apiErr := &dash0api.APIError{
		StatusCode: 400,
		Status:     "400 Bad Request",
		Body:       longBody,
	}

	result := formatAPIError("invalid request", apiErr)
	assert.Contains(t, result.Error(), "...")
	// status line + ":\n  " + 500 chars + "..."
	assert.LessOrEqual(t, len(result.Error()), 600)
}

func TestFormatAPIError_MessageInBody(t *testing.T) {
	// When the API client already extracted a Message, the body is still
	// shown because it may contain additional context.
	apiErr := &dash0api.APIError{
		StatusCode: 400,
		Status:     "400 Bad Request",
		Message:    "validation failed",
		Body:       `{"message": "validation failed", "details": "extra info"}`,
		TraceID:    "trace1",
	}

	result := formatAPIError("invalid request", apiErr)
	assert.Contains(t, result.Error(), "invalid request (status: 400, trace_id: trace1):")
	assert.Contains(t, result.Error(), "extra info")
}
