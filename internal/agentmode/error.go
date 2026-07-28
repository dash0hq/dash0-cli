package agentmode

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// jsonError is the structured representation of a CLI error for agent
// consumption.
type jsonError struct {
	Error string `json:"error"`
	Hint  string `json:"hint,omitempty"`
}

// PrintJSONError writes err as a JSON object to w.
// If the error string contains a "\nHint:" section, it is split into the
// "error" and "hint" fields.
func PrintJSONError(w io.Writer, err error) {
	errStr := err.Error()
	je := jsonError{}

	if idx := strings.Index(errStr, "\nHint:"); idx != -1 {
		je.Error = errStr[:idx]
		je.Hint = strings.TrimSpace(errStr[idx+6:]) // skip "\nHint:"
	} else {
		je.Error = errStr
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	// The default encoder HTML-escapes `<`, `>`, and `&` in string values, which
	// disfigures agent-mode output like `dash0 skill show <topic>` into
	// `dash0 skill show <topic>`. The output is JSON meant for a
	// terminal or an AI agent, not an HTML page, so turn escaping off.
	enc.SetEscapeHTML(false)
	if encErr := enc.Encode(je); encErr != nil {
		// Fallback: if JSON encoding fails, write the raw error string.
		fmt.Fprintln(w, errStr)
	}
}
