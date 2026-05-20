package client

import (
	"context"
	"fmt"
	"os"
	"strings"
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

func TestResolveMaxRetries_Default(t *testing.T) {
	t.Setenv("DASH0_MAX_RETRIES", "")
	os.Unsetenv("DASH0_MAX_RETRIES")

	n, err := resolveMaxRetries(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, DefaultMaxRetries, n)
}

func TestResolveMaxRetries_FromEnv(t *testing.T) {
	t.Setenv("DASH0_MAX_RETRIES", "2")

	n, err := resolveMaxRetries(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 2, n)
}

func TestResolveMaxRetries_Zero(t *testing.T) {
	t.Setenv("DASH0_MAX_RETRIES", "0")

	n, err := resolveMaxRetries(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestResolveMaxRetries_InvalidNonInteger(t *testing.T) {
	t.Setenv("DASH0_MAX_RETRIES", "abc")

	_, err := resolveMaxRetries(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be an integer")
}

func TestResolveMaxRetries_Negative(t *testing.T) {
	t.Setenv("DASH0_MAX_RETRIES", "-1")

	_, err := resolveMaxRetries(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must not be negative")
}

func TestResolveMaxRetries_ExceedsMax(t *testing.T) {
	t.Setenv("DASH0_MAX_RETRIES", "10")

	_, err := resolveMaxRetries(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must not exceed")
}

func TestResolveMaxRetries_FlagTakesPrecedence(t *testing.T) {
	t.Setenv("DASH0_MAX_RETRIES", "4")

	flagValue := 1
	ctx := WithMaxRetries(context.Background(), &flagValue)
	n, err := resolveMaxRetries(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 1, n)
}

func TestResolveMaxRetries_FlagZeroOverridesEnv(t *testing.T) {
	t.Setenv("DASH0_MAX_RETRIES", "3")

	flagValue := 0
	ctx := WithMaxRetries(context.Background(), &flagValue)
	n, err := resolveMaxRetries(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestResolveMaxRetries_FlagExceedsMax(t *testing.T) {
	flagValue := 10
	ctx := WithMaxRetries(context.Background(), &flagValue)
	_, err := resolveMaxRetries(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must not exceed")
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

func TestFormatAPIError_PrefersParsedMessage(t *testing.T) {
	apiErr := &dash0api.APIError{
		StatusCode: 400,
		Status:     "400 Bad Request",
		Message:    "Bad Request: origin mismatch",
		Body:       `{"error":{"code":400,"message":"Bad Request: origin mismatch","traceId":"abc123"}}`,
		TraceID:    "abc123",
	}

	result := formatAPIError("invalid request", apiErr)
	expected := "invalid request (status: 400, trace_id: abc123):\n" +
		"  Bad Request: origin mismatch"
	assert.Equal(t, expected, result.Error())
}

func TestFormatAPIError_FallsBackToBodyWhenNoMessage(t *testing.T) {
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

func TestFormatAPIError_EmptyMessageAndBody(t *testing.T) {
	apiErr := &dash0api.APIError{
		StatusCode: 400,
		Status:     "400 Bad Request",
		TraceID:    "abc123",
	}

	result := formatAPIError("invalid request", apiErr)
	assert.Equal(t, "invalid request (status: 400, trace_id: abc123)", result.Error())
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
		Message:    "missing required field",
		Body:       `{"error":{"message":"missing required field"}}`,
		TraceID:    "def456",
	}
	wrapped := fmt.Errorf("update dashboard failed: %w", apiErr)

	result := formatAPIError("invalid request", wrapped)
	assert.Equal(t,
		"invalid request (status: 400, trace_id: def456):\n  missing required field",
		result.Error())
}

func TestFormatAPIError_LongDetailTruncated(t *testing.T) {
	longMessage := strings.Repeat("x", 600)
	apiErr := &dash0api.APIError{
		StatusCode: 400,
		Status:     "400 Bad Request",
		Message:    longMessage,
	}

	result := formatAPIError("invalid request", apiErr)
	assert.Contains(t, result.Error(), "...")
	// status line + ":\n  " + 500 chars + "..."
	assert.LessOrEqual(t, len(result.Error()), 600)
}

func TestHandleAPIError_NotFoundIncludesTraceID(t *testing.T) {
	apiErr := &dash0api.APIError{
		StatusCode: 404,
		Status:     "404 Not Found",
		Message:    "Not Found: The requested dashboard does not exist or is inaccessible to you.",
		Body:       `{"error":{"code":404,"message":"Not Found: ...","traceId":"trace-404"}}`,
		TraceID:    "trace-404",
	}

	result := HandleAPIError(apiErr, ErrorContext{AssetType: "dashboard", AssetID: "abc"})
	assert.Equal(t,
		"dashboard \"abc\" not found (status: 404, trace_id: trace-404):\n"+
			"  Not Found: The requested dashboard does not exist or is inaccessible to you.",
		result.Error())
}

func TestHandleAPIError_NotFoundWithoutContext(t *testing.T) {
	apiErr := &dash0api.APIError{
		StatusCode: 404,
		Status:     "404 Not Found",
		TraceID:    "trace-404-bare",
	}

	result := HandleAPIError(apiErr)
	assert.Equal(t,
		"asset not found (status: 404, trace_id: trace-404-bare)",
		result.Error())
}
