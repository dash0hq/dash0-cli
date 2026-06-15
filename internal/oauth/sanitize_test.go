package oauth

import (
	"strings"
	"testing"
)

func TestSanitizeASText(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"plain ascii", "user denied access", "user denied access"},
		{"keeps tab", "col1\tcol2", "col1\tcol2"},
		// Newlines are dropped: a hostile AS could otherwise inject a
		// forged "Logged in…" line that renders below the real "Error:"
		// line and mislead the user.
		{"drops newline", "line1\nline2\tcol", "line1line2\tcol"},
		{"drops escape and bell", "alert \x1b[31mERROR\x1b[0m \x07 done", "alert [31mERROR[0m  done"},
		{"drops carriage return and backspace", "foo\rbar\b baz", "foobar baz"},
		// Unicode line/paragraph separators are dropped for the same
		// reason as ASCII newlines — terminals treat them as line breaks.
		{"drops NEL (U+0085)", "a\u0085b", "ab"},
		{"drops LS (U+2028)", "a\u2028b", "ab"},
		{"drops PS (U+2029)", "a\u2029b", "ab"},
		{"drops DEL", "x\x7fy", "xy"},
		{"keeps high unicode", "été ✓ ☃", "été ✓ ☃"},
		// Hostile or broken AS may send invalid UTF-8 bytes. They are
		// dropped rather than rendered as U+FFFD: the replacement char
		// carries no signal and would just be visible noise on the TTY.
		{"drops invalid utf-8", "x\xffy", "xy"},
		{"drops standalone continuation byte", "a\x80b", "ab"},
		{"keeps multi-byte unicode adjacent to invalid", "a\xff café", "a café"},
		// Trojan Source (CVE-2021-42574) class: bidirectional override and
		// zero-width controls that reorder or hide adjacent text on a
		// terminal without altering the byte stream. Use explicit \u
		// escapes so the test source reads cleanly (the characters are
		// invisible in most editors and the BOM is rejected by the Go
		// parser outside the file header).
		{"drops RLO (U+202E)", "user‮denied", "userdenied"},
		{"drops zero-width space (U+200B)", "ad​min", "admin"},
		{"drops BOM (U+FEFF)", "\uFEFFstart", "start"},
		{"drops LRI/PDI isolates (U+2066/U+2069)", "a⁦secret⁩b", "asecretb"},
		{"drops LRE/PDF pair (U+202A/U+202C)", "‪oops‬", "oops"},
		{"drops zero-width joiner (U+200D)", "ab‍cd", "abcd"},
		{"drops LRM (U+200E)", "x‎y", "xy"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SanitizeASText(c.in); got != c.want {
				t.Errorf("SanitizeASText(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestSanitizeASTextLengthCap verifies the upfront input cap so a hostile
// AS returning a multi-megabyte error_description does not cause the
// caller to allocate a multi-megabyte buffer for content the caller will
// immediately truncate to ~512 chars anyway.
func TestSanitizeASTextLengthCap(t *testing.T) {
	huge := strings.Repeat("a", maxASTextBytes*4)
	out := SanitizeASText(huge)
	if len(out) > maxASTextBytes {
		t.Fatalf("SanitizeASText output longer than cap: got %d, cap %d", len(out), maxASTextBytes)
	}
	if len(out) != maxASTextBytes {
		t.Fatalf("expected exactly cap bytes (all ASCII 'a'), got %d", len(out))
	}
}
