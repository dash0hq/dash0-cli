package otlp

import (
	"strings"
	"testing"
)

// withTailColorEnabled toggles the package-level color flag for the
// duration of a test and restores the previous value on cleanup. Tests
// that exercise the color path must use this helper so they don't leak
// state into other tests.
func withTailColorEnabled(t *testing.T, enabled bool) {
	t.Helper()
	prev := tailColorEnabled
	tailColorEnabled = enabled
	t.Cleanup(func() { tailColorEnabled = prev })
}

func TestColorSeverity_DisabledReturnsPlain(t *testing.T) {
	withTailColorEnabled(t, false)
	got := colorSeverity("ERROR", 0)
	if got != "ERROR" {
		t.Errorf("got %q; want plain 'ERROR' when coloring disabled", got)
	}
	// No ANSI escape sequences when disabled.
	if strings.Contains(got, "\x1b[") {
		t.Errorf("disabled output should contain no ANSI escapes; got %q", got)
	}
}

func TestColorSeverity_PadsToWidth(t *testing.T) {
	withTailColorEnabled(t, false)
	got := colorSeverity("INFO", 10)
	if got != "INFO      " {
		t.Errorf("got %q; want 'INFO      ' (right-padded to 10)", got)
	}
}

func TestColorSeverity_NoEscapesWhenNotTTY(t *testing.T) {
	// stdout under `go test` is rarely a TTY; the function should fall
	// back to plain text even when the global flag is enabled. We can't
	// reliably force a TTY in tests, so the contract we check is:
	// when stdout is not a TTY, no ANSI escapes leak regardless of flag.
	withTailColorEnabled(t, true)
	got := colorSeverity("ERROR", 0)
	if strings.Contains(got, "\x1b[") {
		t.Errorf("non-TTY stdout should produce no ANSI escapes even with color enabled; got %q", got)
	}
}

func TestColorSeverity_UnknownLevelUnchanged(t *testing.T) {
	withTailColorEnabled(t, true)
	// Even if we were on a TTY, custom/unknown severities fall through
	// to the plain padding branch. Verify by checking the result is
	// exactly the padded text.
	if got := colorSeverity("CUSTOM", 0); got != "CUSTOM" {
		t.Errorf("got %q; want plain 'CUSTOM' for non-standard severity", got)
	}
}

func TestColorSpanStatus_DisabledReturnsPlain(t *testing.T) {
	withTailColorEnabled(t, false)
	for _, s := range []string{"Ok", "Error", "Unset", "STATUS_CODE_ERROR"} {
		got := colorSpanStatus(s, 0)
		if got != s {
			t.Errorf("got %q; want plain %q when coloring disabled", got, s)
		}
		if strings.Contains(got, "\x1b[") {
			t.Errorf("disabled output should contain no ANSI escapes; got %q for %q", got, s)
		}
	}
}

func TestColorSpanStatus_PadsToWidth(t *testing.T) {
	withTailColorEnabled(t, false)
	got := colorSpanStatus("OK", 6)
	if got != "OK    " {
		t.Errorf("got %q; want 'OK    ' (right-padded to 6)", got)
	}
}

func TestColorSpanStatus_NoEscapesWhenNotTTY(t *testing.T) {
	withTailColorEnabled(t, true)
	if got := colorSpanStatus("Error", 0); strings.Contains(got, "\x1b[") {
		t.Errorf("non-TTY stdout should produce no ANSI escapes; got %q", got)
	}
}

func TestSetTailColorEnabled_TogglesGlobal(t *testing.T) {
	prev := tailColorEnabled
	t.Cleanup(func() { tailColorEnabled = prev })

	SetTailColorEnabled(true)
	if !tailColorEnabled {
		t.Error("SetTailColorEnabled(true) did not enable")
	}
	SetTailColorEnabled(false)
	if tailColorEnabled {
		t.Error("SetTailColorEnabled(false) did not disable")
	}
}
