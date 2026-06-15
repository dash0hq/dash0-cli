package version

import "github.com/dash0hq/dash0-cli/internal/agentmode"

// Version is the build version of the CLI, set from cmd/dash0/main.go.
var Version = "dev"

// UserAgent returns the User-Agent string for HTTP requests.
// For release builds it returns "dash0-cli/1.4.0".
// For dev builds it returns "dash0-cli/dev" or "dash0-cli/dev(abc1234)" when the commit is known.
// When agent mode is active, " agent/<slug>" is appended so the Dash0 backend
// can distinguish requests driven by specific AI coding agents (e.g.
// "agent/claude-code", "agent/cursor", "agent/unknown" when the agent could
// not be identified).
func UserAgent() string {
	ua := "dash0-cli/" + Version
	if agentmode.Enabled {
		ua += " agent/" + agentmode.Detected
	}
	return ua
}
