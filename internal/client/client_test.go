package client

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewClient_WithEnvVars(t *testing.T) {
	// Setup test environment
	os.Setenv("DASH0_TEST_MODE", "1")
	os.Setenv("DASH0_API_URL", "https://api.test.dash0.com")
	os.Setenv("DASH0_AUTH_TOKEN", "auth_test-token-12345")
	defer func() {
		os.Unsetenv("DASH0_TEST_MODE")
		os.Unsetenv("DASH0_API_URL")
		os.Unsetenv("DASH0_AUTH_TOKEN")
	}()

	client, err := NewClient("", "")
	assert.NoError(t, err)
	assert.NotNil(t, client)
}

func TestNewClient_WithOverrides(t *testing.T) {
	os.Setenv("DASH0_TEST_MODE", "1")
	defer os.Unsetenv("DASH0_TEST_MODE")

	client, err := NewClient("https://api.override.dash0.com", "auth_override-token-12345")
	assert.NoError(t, err)
	assert.NotNil(t, client)
}

func TestNewClient_MissingConfig(t *testing.T) {
	// Ensure no environment variables are set
	os.Unsetenv("DASH0_API_URL")
	os.Unsetenv("DASH0_AUTH_TOKEN")
	os.Unsetenv("DASH0_TEST_MODE")

	// Without test mode, missing config should return an error
	client, err := NewClient("", "")
	assert.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "api-url and auth-token are required")
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
