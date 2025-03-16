package tools

import (
	"os"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
)

func TestAddQueryInstantMetricTool(t *testing.T) {
	// Simply test that we can register the tool without error
	os.Setenv("DASH0_TEST_MODE", "1")
	defer os.Unsetenv("DASH0_TEST_MODE")

	// Create a server
	s := server.NewMCPServer("Dash0", "test", server.WithResourceCapabilities(true, true))
	
	// Register the tool - this should not panic
	assert.NotPanics(t, func() {
		addQueryInstantMetricTool(s)
	}, "Should be able to register the tool")
	
	// We can't easily test the actual execution of the tool without mocking too many components,
	// so we'll stop here and rely on manual/integration testing for the full functionality
	
	// For a real integration test, we would:
	// 1. Create a test HTTP server that responds with mock metric data
	// 2. Create an MCP server and register the tool 
	// 3. Build a proper request with test parameters
	// 4. Invoke the tool through the server
	// 5. Verify the results
	
	// The actual execution happens in the normal CLI flow:
	// 1. User starts the dash0 mcp server
	// 2. AI uses the queryInstantMetric tool
	// 3. The MCP server calls our handler with the tool parameters
	// 4. We make the HTTP request to the Dash0 API
	// 5. We return the formatted results to the AI
}