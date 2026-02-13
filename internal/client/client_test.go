package client

import (
	"context"
	"os"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/config"
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

func TestDatasetPtr(t *testing.T) {
	result := DatasetPtr("")
	assert.Nil(t, result)

	result = DatasetPtr("default")
	assert.Nil(t, result)

	result = DatasetPtr("my-dataset")
	assert.NotNil(t, result)
	assert.Equal(t, "my-dataset", *result)
}

func TestResolveDataset_FlagTakesPrecedence(t *testing.T) {
	cfg := &config.Configuration{
		ApiUrl:    "https://api.test.dash0.com",
		AuthToken: "auth_test-token-12345",
		Dataset:   "config-dataset",
	}
	ctx := config.WithConfiguration(context.Background(), cfg)

	result := ResolveDataset(ctx, "flag-dataset")
	assert.NotNil(t, result)
	assert.Equal(t, "flag-dataset", *result)
}

func TestResolveDataset_FallsBackToConfig(t *testing.T) {
	cfg := &config.Configuration{
		ApiUrl:    "https://api.test.dash0.com",
		AuthToken: "auth_test-token-12345",
		Dataset:   "config-dataset",
	}
	ctx := config.WithConfiguration(context.Background(), cfg)

	result := ResolveDataset(ctx, "")
	assert.NotNil(t, result)
	assert.Equal(t, "config-dataset", *result)
}

func TestResolveDataset_NilWhenEmpty(t *testing.T) {
	cfg := &config.Configuration{
		ApiUrl:    "https://api.test.dash0.com",
		AuthToken: "auth_test-token-12345",
	}
	ctx := config.WithConfiguration(context.Background(), cfg)

	result := ResolveDataset(ctx, "")
	assert.Nil(t, result)
}

func TestResolveDataset_NilForDefaultInConfig(t *testing.T) {
	cfg := &config.Configuration{
		ApiUrl:    "https://api.test.dash0.com",
		AuthToken: "auth_test-token-12345",
		Dataset:   "default",
	}
	ctx := config.WithConfiguration(context.Background(), cfg)

	result := ResolveDataset(ctx, "")
	assert.Nil(t, result)
}

func TestResolveDataset_NilForDefaultFlag(t *testing.T) {
	result := ResolveDataset(context.Background(), "default")
	assert.Nil(t, result)
}

func TestResolveDataset_DefaultFlagOverridesConfig(t *testing.T) {
	cfg := &config.Configuration{
		ApiUrl:    "https://api.test.dash0.com",
		AuthToken: "auth_test-token-12345",
		Dataset:   "config-dataset",
	}
	ctx := config.WithConfiguration(context.Background(), cfg)

	result := ResolveDataset(ctx, "default")
	assert.Nil(t, result)
}

func TestResolveDataset_NilWithoutContext(t *testing.T) {
	result := ResolveDataset(context.Background(), "")
	assert.Nil(t, result)
}
