package query

import (
	"strings"
	"time"
)

// NormalizeTimestamp normalizes an absolute timestamp to the format required by the
// Dash0 API (RFC 3339 with millisecond precision). Relative expressions like "now"
// and "now-1h" are returned as-is.
func NormalizeTimestamp(s string) string {
	// Relative expressions start with "now"
	if strings.HasPrefix(s, "now") {
		return s
	}

	// Try parsing common ISO 8601 variants and re-format with millisecond precision.
	formats := []string{
		"2006-01-02T15:04:05.000Z07:00", // already has millis
		"2006-01-02T15:04:05Z07:00",     // missing fractional seconds
		"2006-01-02T15:04:05",           // no timezone
		"2006-01-02",                    // date only (midnight UTC)
	}
	for _, layout := range formats {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format("2006-01-02T15:04:05.000Z")
		}
	}

	// Could not parse — return as-is and let the API validate.
	return s
}
