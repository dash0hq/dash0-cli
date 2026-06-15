package version

import (
	"testing"

	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/stretchr/testify/assert"
)

func TestUserAgent(t *testing.T) {
	prevEnabled := agentmode.Enabled
	prevDetected := agentmode.Detected
	prevVersion := Version
	t.Cleanup(func() {
		agentmode.Enabled = prevEnabled
		agentmode.Detected = prevDetected
		Version = prevVersion
	})

	Version = "1.4.0"

	agentmode.Enabled = false
	agentmode.Detected = ""
	assert.Equal(t, "dash0-cli/1.4.0", UserAgent())

	agentmode.Enabled = true
	agentmode.Detected = "claude-code"
	assert.Equal(t, "dash0-cli/1.4.0 agent/claude-code", UserAgent())

	agentmode.Detected = "unknown"
	assert.Equal(t, "dash0-cli/1.4.0 agent/unknown", UserAgent())
}
