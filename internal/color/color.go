package color

import (
	"os"

	"github.com/muesli/termenv"
)

// NoColor disables all color output when set to true.
// This is set by the --color=none flag or the DASH0_COLOR=none environment variable.
var NoColor bool

// StdoutOutput returns a termenv output for stdout, respecting the NoColor flag.
func StdoutOutput() *termenv.Output {
	if NoColor {
		return termenv.NewOutput(os.Stdout, termenv.WithProfile(termenv.Ascii))
	}
	return termenv.NewOutput(os.Stdout)
}

// StderrOutput returns a termenv output for stderr, respecting the NoColor flag.
func StderrOutput() *termenv.Output {
	if NoColor {
		return termenv.NewOutput(os.Stderr, termenv.WithProfile(termenv.Ascii))
	}
	return termenv.NewOutput(os.Stderr)
}
