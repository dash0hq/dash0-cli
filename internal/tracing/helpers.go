package tracing

import (
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// SpanKindString maps a numeric span kind to its string representation.
func SpanKindString(kind int32) string {
	switch kind {
	case 0:
		return "UNSPECIFIED"
	case 1:
		return "INTERNAL"
	case 2:
		return "SERVER"
	case 3:
		return "CLIENT"
	case 4:
		return "PRODUCER"
	case 5:
		return "CONSUMER"
	default:
		return fmt.Sprintf("SPAN_KIND_%d", kind)
	}
}

// ParseSpanKind parses a span kind string (case-insensitive) into its numeric value.
// UNSPECIFIED is not accepted because it is not a valid kind for user-created spans.
func ParseSpanKind(s string) (int32, error) {
	switch strings.ToUpper(s) {
	case "INTERNAL":
		return 1, nil
	case "SERVER":
		return 2, nil
	case "CLIENT":
		return 3, nil
	case "PRODUCER":
		return 4, nil
	case "CONSUMER":
		return 5, nil
	default:
		return 0, fmt.Errorf("unknown span kind %q (valid values: INTERNAL, SERVER, CLIENT, PRODUCER, CONSUMER)", s)
	}
}

// SpanStatusString maps a numeric status code to its string representation.
func SpanStatusString(code int32) string {
	switch code {
	case 0:
		return "UNSET"
	case 1:
		return "OK"
	case 2:
		return "ERROR"
	default:
		return fmt.Sprintf("STATUS_%d", code)
	}
}

// ParseSpanStatusCode parses a status code string (case-insensitive) into its numeric value.
func ParseSpanStatusCode(s string) (int32, error) {
	switch strings.ToUpper(s) {
	case "UNSET":
		return 0, nil
	case "OK":
		return 1, nil
	case "ERROR":
		return 2, nil
	default:
		return 0, fmt.Errorf("unknown status code %q (valid values: UNSET, OK, ERROR)", s)
	}
}

// FormatDuration computes the duration from two nanosecond Unix timestamps
// and returns a human-readable string.
func FormatDuration(startNano, endNano string) string {
	start, err := strconv.ParseInt(startNano, 10, 64)
	if err != nil {
		return "?"
	}
	end, err := strconv.ParseInt(endNano, 10, 64)
	if err != nil {
		return "?"
	}
	d := time.Duration(end - start)
	return FormatTimeDuration(d)
}

// FormatTimeDuration formats a time.Duration as a human-readable string.
func FormatTimeDuration(d time.Duration) string {
	if d <= 0 {
		return "0ms"
	}
	if d < time.Millisecond {
		us := float64(d.Nanoseconds()) / 1000.0
		return fmt.Sprintf("%.1fus", us)
	}
	if d < time.Second {
		ms := float64(d.Nanoseconds()) / float64(time.Millisecond)
		if ms == math.Trunc(ms) {
			return fmt.Sprintf("%dms", int64(ms))
		}
		return fmt.Sprintf("%.1fms", ms)
	}
	if d < time.Minute {
		s := d.Seconds()
		return fmt.Sprintf("%.2fs", s)
	}
	m := int(d.Minutes())
	s := d.Seconds() - float64(m*60)
	return fmt.Sprintf("%dm %.1fs", m, s)
}

// FormatSpanLinks formats outgoing span links as a semicolon-separated
// string of "traceId:spanId" pairs. Dash0ForwardLinks (incoming links from
// other spans) are intentionally excluded.
func FormatSpanLinks(links []dash0api.SpanLink) string {
	var parts []string
	for _, link := range links {
		parts = append(parts, hex.EncodeToString(link.TraceId)+":"+hex.EncodeToString(link.SpanId))
	}
	return strings.Join(parts, ";")
}

// ParseDuration parses a human-readable duration string like "100ms", "1.5s", "2m".
func ParseDuration(s string) (time.Duration, error) {
	// Go's time.ParseDuration handles "100ms", "1.5s", "2m", "1h", etc.
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q (examples: \"100ms\", \"1.5s\", \"2m\"): %w", s, err)
	}
	if d < 0 {
		return 0, fmt.Errorf("duration must be positive, got %q", s)
	}
	return d, nil
}



