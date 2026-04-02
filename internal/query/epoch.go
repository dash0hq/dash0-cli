package query

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ResolveToEpochSeconds converts a time expression to a Unix epoch seconds string.
// It handles:
//   - "now" → current timestamp
//   - "now-1h", "now-30m", "now-7d" → relative to now (supports d/h/m/s suffixes)
//   - ISO 8601 absolute timestamps → parsed and converted
func ResolveToEpochSeconds(s string) (string, error) {
	now := time.Now()

	if s == "now" {
		return strconv.FormatInt(now.Unix(), 10), nil
	}

	if strings.HasPrefix(s, "now-") {
		durationStr := s[4:]
		d, err := parseRelativeDuration(durationStr)
		if err != nil {
			return "", fmt.Errorf("invalid relative time %q: %w", s, err)
		}
		return strconv.FormatInt(now.Add(-d).Unix(), 10), nil
	}

	// Try absolute timestamp formats
	formats := []string{
		"2006-01-02T15:04:05.000Z07:00",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, layout := range formats {
		if t, err := time.Parse(layout, s); err == nil {
			return strconv.FormatInt(t.Unix(), 10), nil
		}
	}

	return "", fmt.Errorf("cannot parse time expression %q", s)
}

// parseRelativeDuration parses a duration string that may include "d" for days,
// which time.ParseDuration does not support.
func parseRelativeDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		daysStr := strings.TrimSuffix(s, "d")
		days, err := strconv.Atoi(daysStr)
		if err != nil {
			return 0, fmt.Errorf("invalid day count %q: %w", daysStr, err)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
