package tracing

import (
	"testing"
	"time"
)

func TestSpanKindString(t *testing.T) {
	tests := []struct {
		kind int32
		want string
	}{
		{0, "UNSPECIFIED"},
		{1, "INTERNAL"},
		{2, "SERVER"},
		{3, "CLIENT"},
		{4, "PRODUCER"},
		{5, "CONSUMER"},
		{99, "SPAN_KIND_99"},
	}
	for _, tc := range tests {
		got := SpanKindString(tc.kind)
		if got != tc.want {
			t.Errorf("SpanKindString(%d) = %q, want %q", tc.kind, got, tc.want)
		}
	}
}

func TestParseSpanKind(t *testing.T) {
	tests := []struct {
		input   string
		want    int32
		wantErr bool
	}{
		{"INTERNAL", 1, false},
		{"server", 2, false},
		{"Client", 3, false},
		{"PRODUCER", 4, false},
		{"CONSUMER", 5, false},
		{"UNSPECIFIED", 0, true},
		{"invalid", 0, true},
	}
	for _, tc := range tests {
		got, err := ParseSpanKind(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseSpanKind(%q) expected error", tc.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseSpanKind(%q) unexpected error: %v", tc.input, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseSpanKind(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestSpanStatusString(t *testing.T) {
	tests := []struct {
		code int32
		want string
	}{
		{0, "UNSET"},
		{1, "OK"},
		{2, "ERROR"},
		{99, "STATUS_99"},
	}
	for _, tc := range tests {
		got := SpanStatusString(tc.code)
		if got != tc.want {
			t.Errorf("SpanStatusString(%d) = %q, want %q", tc.code, got, tc.want)
		}
	}
}

func TestParseSpanStatusCode(t *testing.T) {
	tests := []struct {
		input   string
		want    int32
		wantErr bool
	}{
		{"UNSET", 0, false},
		{"ok", 1, false},
		{"ERROR", 2, false},
		{"invalid", 0, true},
	}
	for _, tc := range tests {
		got, err := ParseSpanStatusCode(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseSpanStatusCode(%q) expected error", tc.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseSpanStatusCode(%q) unexpected error: %v", tc.input, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseSpanStatusCode(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		start, end string
		want       string
	}{
		// 100ms
		{"1000000000000000000", "1000000000100000000", "100ms"},
		// 1.5s
		{"1000000000000000000", "1000000001500000000", "1.50s"},
		// 2m 15.3s
		{"1000000000000000000", "1000000135300000000", "2m 15.3s"},
		// invalid start
		{"abc", "1000000000100000000", "?"},
		// 0ms (same timestamps)
		{"1000000000000000000", "1000000000000000000", "0ms"},
	}
	for _, tc := range tests {
		got := FormatDuration(tc.start, tc.end)
		if got != tc.want {
			t.Errorf("FormatDuration(%q, %q) = %q, want %q", tc.start, tc.end, got, tc.want)
		}
	}
}

func TestFormatTimeDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{100 * time.Millisecond, "100ms"},
		{1500 * time.Millisecond, "1.50s"},
		{500 * time.Microsecond, "500.0us"},
		{0, "0ms"},
		{-1 * time.Second, "0ms"},
	}
	for _, tc := range tests {
		got := FormatTimeDuration(tc.d)
		if got != tc.want {
			t.Errorf("FormatTimeDuration(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		{"100ms", 100 * time.Millisecond, false},
		{"1.5s", 1500 * time.Millisecond, false},
		{"2m", 2 * time.Minute, false},
		{"1h", time.Hour, false},
		{"-1s", 0, true},
		{"invalid", 0, true},
	}
	for _, tc := range tests {
		got, err := ParseDuration(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseDuration(%q) expected error", tc.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseDuration(%q) unexpected error: %v", tc.input, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseDuration(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}
