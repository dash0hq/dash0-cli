package oauth

import (
	"strings"
	"unicode/utf8"
)

// maxASTextBytes bounds the input SanitizeASText is willing to process.
// A hostile or buggy AS could return a multi-megabyte error_description;
// callers always truncate the output to a small display length anyway, so
// cap the input up front to avoid allocating large buffers for content
// that will be discarded.
const maxASTextBytes = 4096

// SanitizeASText strips characters that would mislead or attack a terminal
// reader from a string supplied by the OAuth authorization server. Three
// classes are dropped:
//
//   - ASCII control characters (0x00-0x1F, 0x7F) except tab (0x09).
//     ESC sequences are the obvious vector — a hostile AS could embed
//     them in `error_description` to redraw the user's terminal line.
//     Line-breaking characters (\n, \r) are also dropped because the
//     caller renders AS-supplied text inline with CLI-controlled text:
//     a bare newline lets a hostile AS continue on a new terminal line
//     that the user reads as CLI output (e.g. a forged "Logged in"
//     line after a real "Error:" line). Tabs are kept because they do
//     not introduce a new logical line.
//   - Invalid UTF-8 byte sequences. Go's default behavior renders these
//     as U+FFFD, which is visible noise carrying no signal — drop them.
//   - Bidirectional override and zero-width control characters
//     (U+200B-U+200F, U+202A-U+202E, U+2066-U+2069, U+FEFF). These were
//     the vector behind CVE-2021-42574 (Trojan Source) — they reorder or
//     hide adjacent text without altering its bytes, which can make a
//     malicious error_description appear innocuous.
//   - Unicode line-/paragraph-separator code points (U+0085, U+2028,
//     U+2029). Terminals treat these as line breaks; dropping them
//     closes the same forged-line-injection vector ASCII \n covers.
//
// Input longer than maxASTextBytes is truncated up front to avoid
// allocating multi-megabyte buffers for content the caller will discard.
// All other valid runes — including non-ASCII (UTF-8) — pass through.
func SanitizeASText(s string) string {
	if s == "" {
		return ""
	}
	if len(s) > maxASTextBytes {
		s = s[:maxASTextBytes]
	}
	var b strings.Builder
	b.Grow(len(s))
	// Walk by byte so invalid UTF-8 is detectable. utf8.DecodeRuneInString
	// returns (RuneError, 1) for an invalid leading byte; drop that byte and
	// keep scanning rather than emitting U+FFFD.
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			// Invalid UTF-8 byte — drop.
			i++
			continue
		}
		switch {
		case r == '\t':
			b.WriteRune(r)
		case r < 0x20 || r == 0x7F:
			// ASCII control char (including \n and \r) — drop.
		case r == 0x85 || r == 0x2028 || r == 0x2029:
			// Unicode line/paragraph separators — drop.
		case isUnicodeFormatHazard(r):
			// Bidi-control / zero-width / BOM — drop.
		default:
			b.WriteRune(r)
		}
		i += size
	}
	return b.String()
}

// isUnicodeFormatHazard reports whether r is a Unicode format-control
// character that can mislead or reorder terminal output (Trojan Source
// CVE-2021-42574 class). The set is deliberately small and explicit
// rather than relying on `unicode.Cf`, which also covers benign joiners
// like the Arabic letter mark; the listed code points are the ones that
// have documented impact on terminal text rendering.
func isUnicodeFormatHazard(r rune) bool {
	switch {
	case r >= 0x200B && r <= 0x200F:
		// Zero-width space, zero-width non-joiner, zero-width joiner,
		// LRM, RLM.
		return true
	case r >= 0x202A && r <= 0x202E:
		// LRE, RLE, PDF, LRO, RLO — the classic Trojan Source set.
		return true
	case r >= 0x2066 && r <= 0x2069:
		// LRI, RLI, FSI, PDI — the modern isolate-based override set.
		return true
	case r == 0xFEFF:
		// Byte order mark / zero-width no-break space.
		return true
	}
	return false
}
