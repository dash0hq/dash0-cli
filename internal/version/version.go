package version

// Version is the build version of the CLI, set from cmd/dash0/main.go.
var Version = "dev"

// UserAgent returns the User-Agent string for HTTP requests.
// For release builds it returns "dash0-cli/1.4.0".
// For dev builds it returns "dash0-cli/dev" or "dash0-cli/dev(abc1234)" when the commit is known.
func UserAgent() string {
	return "dash0-cli/" + Version
}
