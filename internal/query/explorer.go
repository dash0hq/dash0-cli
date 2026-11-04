package query

import (
	"fmt"
	"io"
)

// ExplorerURLKey is the JSON key under which query commands report the Dash0
// explorer deep link alongside their results.
//
// The key is prefixed with "dash0" because it is a Dash0 extension to payloads
// that are otherwise OTLP/JSON: an unprefixed name risks colliding with a field
// OTLP itself may introduce later.
const ExplorerURLKey = "dash0ExplorerUrl"

// Labels introducing a deep link in human-readable output.
const (
	ExplorerLabelQuery = "Open this query in Dash0:"
	ExplorerLabelTrace = "Open this trace in Dash0:"
)

// PrintExplorerURL writes a Dash0 explorer deep link for human consumption,
// introduced by label. It is a no-op when explorerURL is empty, which is what
// the URL builders return when the API URL is unset or cannot be parsed.
//
// Callers pass os.Stderr so the link stays out of the piped result stream: the
// link is commentary about the query, not part of its results.
func PrintExplorerURL(w io.Writer, label, explorerURL string) {
	if explorerURL == "" {
		return
	}
	fmt.Fprintf(w, "\n%s\n    %s\n", label, explorerURL)
}

// AddExplorerURL sets the deep link on a JSON result wrapper under
// [ExplorerURLKey], and returns the wrapper for chaining. It leaves the wrapper
// untouched when explorerURL is empty, so a consumer can treat the key's
// presence as "a link is available" rather than having to test for "".
func AddExplorerURL(wrapper map[string]any, explorerURL string) map[string]any {
	if explorerURL != "" {
		wrapper[ExplorerURLKey] = explorerURL
	}
	return wrapper
}
