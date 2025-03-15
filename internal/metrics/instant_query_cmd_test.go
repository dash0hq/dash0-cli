package metrics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQueryInstantCmd(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check the request
		assert.Equal(t, "/api/prometheus/api/v1/query", r.URL.Path)
		assert.Equal(t, "test_query", r.URL.Query().Get("query"))
		assert.NotEmpty(t, r.URL.Query().Get("time"))
		assert.Equal(t, "Bearer test_token", r.Header.Get("Authorization"))

		// Return a sample response
		response := QueryInstantResponse{
			Status: "success",
			Data: struct {
				ResultType string `json:"resultType"`
				Result     []struct {
					Metric map[string]string `json:"metric"`
					Value  []interface{}     `json:"value"`
				} `json:"result"`
			}{
				ResultType: "vector",
				Result: []struct {
					Metric map[string]string `json:"metric"`
					Value  []interface{}     `json:"value"`
				}{
					{
						Metric: map[string]string{"instance": "localhost:8080", "job": "test"},
						Value:  []interface{}{float64(1609459200), "1"},
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Set up environment for test
	os.Setenv("DASH0_TEST_MODE", "1")
	defer os.Unsetenv("DASH0_TEST_MODE")

	// Create and execute the command
	cmd := newInstantQueryCmd()
	cmd.SetArgs([]string{"--query", "test_query", "--base-url", server.URL, "--auth-token", "test_token"})

	// Execute should succeed
	err := cmd.Execute()
	assert.NoError(t, err)
}

func TestQueryInstantCmdError(t *testing.T) {
	// Create a test server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return an error response
		response := QueryInstantResponse{
			Status:    "error",
			Error:     "Query execution error",
			ErrorType: "execution",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Set up environment for test
	os.Setenv("DASH0_TEST_MODE", "1")
	defer os.Unsetenv("DASH0_TEST_MODE")

	// Create and execute the command
	cmd := newInstantQueryCmd()
	cmd.SetArgs([]string{"--query", "test_query", "--base-url", server.URL, "--auth-token", "test_token"})

	// Execute should fail with the expected error
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Query execution error")
}
