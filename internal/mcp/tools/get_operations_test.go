package tools

import (
	"os"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
)

func TestGetOperationsRegistration(t *testing.T) {
	// Simply test that we can register the tool without error
	os.Setenv("DASH0_TEST_MODE", "1")
	defer os.Unsetenv("DASH0_TEST_MODE")

	// Create a server
	s := server.NewMCPServer("Dash0", "test", server.WithResourceCapabilities(true, true))
	
	// Register the tool - this should not panic
	assert.NotPanics(t, func() {
		addGetOperationsTool(s)
	}, "Should be able to register the tool")
	
	// We can't easily test the actual execution of the tool without mocking too many components,
	// so we'll stop here and rely on manual/integration testing for the full functionality
}