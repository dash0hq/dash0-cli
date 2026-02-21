package query

import (
	"strconv"
	"time"
)

// FormatTimestamp converts a nanosecond Unix timestamp string to a human-readable format.
func FormatTimestamp(nanoStr string) string {
	nanos, err := strconv.ParseInt(nanoStr, 10, 64)
	if err != nil {
		return nanoStr
	}
	t := time.Unix(0, nanos).UTC()
	return t.Format("2006-01-02T15:04:05.000Z")
}
