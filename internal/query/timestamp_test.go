package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeTimestamp(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"relative now", "now", "now"},
		{"relative now-1h", "now-1h", "now-1h"},
		{"relative now-15m", "now-15m", "now-15m"},
		{"full with millis", "2024-01-25T10:00:00.000Z", "2024-01-25T10:00:00.000Z"},
		{"without millis", "2024-01-25T10:00:00Z", "2024-01-25T10:00:00.000Z"},
		{"date only", "2024-01-25", "2024-01-25T00:00:00.000Z"},
		{"no timezone", "2024-01-25T10:00:00", "2024-01-25T10:00:00.000Z"},
		{"with timezone offset", "2024-01-25T12:00:00+02:00", "2024-01-25T10:00:00.000Z"},
		{"with millis and offset", "2024-01-25T12:00:00.500+02:00", "2024-01-25T10:00:00.500Z"},
		{"unparseable returns as-is", "garbage", "garbage"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, NormalizeTimestamp(tt.input))
		})
	}
}
