package client

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewClientFromContext_WithEnvVars(t *testing.T) {
	// Setup test environment
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
	// Set base env vars that will be overridden by parameters
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
	// Ensure no environment variables are set
	os.Unsetenv("DASH0_API_URL")
	os.Unsetenv("DASH0_AUTH_TOKEN")

	// Use a temporary empty config directory to ensure no profile is loaded
	tempDir := t.TempDir()
	os.Setenv("DASH0_CONFIG_DIR", tempDir)
	defer os.Unsetenv("DASH0_CONFIG_DIR")

	// Missing config should return an error about no active profile
	client, err := NewClientFromContext(context.Background(), "", "")
	assert.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "no active profile configured")
}

func TestDatasetPtr(t *testing.T) {
	// Empty string should return nil
	result := DatasetPtr("")
	assert.Nil(t, result)

	// Non-empty string should return pointer
	result = DatasetPtr("my-dataset")
	assert.NotNil(t, result)
	assert.Equal(t, "my-dataset", *result)
}
