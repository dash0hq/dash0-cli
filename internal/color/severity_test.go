package color

import (
	"testing"

	"github.com/fatih/color"
)

const testWidth = 10

func TestSprintSeverityColored(t *testing.T) {
	tests := []struct {
		severity string
		wantANSI bool
	}{
		{"ERROR", true},
		{"FATAL", true},
		{"WARN", true},
		{"INFO", true},
		{"UNKNOWN", true},
		{"DEBUG", false},
		{"TRACE", false},
		{"custom", false},
	}

	// Force color on so ANSI codes are emitted regardless of terminal.
	color.NoColor = false
	defer func() { color.NoColor = false }()

	for _, tc := range tests {
		t.Run(tc.severity, func(t *testing.T) {
			got := sprintSeverityColored(tc.severity, testWidth)
			// Plain text is at most max(testWidth, len(severity)) characters.
			// ANSI codes add escape sequences that make the string longer.
			plainLen := testWidth
			if len(tc.severity) > plainLen {
				plainLen = len(tc.severity)
			}
			hasANSI := len(got) > plainLen
			if tc.wantANSI && !hasANSI {
				t.Errorf("sprintSeverityColored(%q, %d) = %q, expected ANSI codes", tc.severity, testWidth, got)
			}
			if !tc.wantANSI && hasANSI {
				t.Errorf("sprintSeverityColored(%q, %d) = %q, did not expect ANSI codes", tc.severity, testWidth, got)
			}
		})
	}
}

func TestSprintSeverityNoColor(t *testing.T) {
	color.NoColor = true
	defer func() { color.NoColor = false }()

	tests := []string{"ERROR", "WARN", "INFO", "DEBUG", "TRACE", "FATAL", "UNKNOWN"}
	for _, sev := range tests {
		t.Run(sev, func(t *testing.T) {
			got := SprintSeverity(sev, testWidth)
			// Should be plain text, padded to testWidth
			if len(got) < testWidth {
				t.Errorf("SprintSeverity(%q, %d) = %q (len %d), expected at least %d characters",
					sev, testWidth, got, len(got), testWidth)
			}
			// No ANSI escape codes — length should be exactly max(testWidth, len(sev))
			expected := testWidth
			if len(sev) > expected {
				expected = len(sev)
			}
			if len(got) != expected {
				t.Errorf("SprintSeverity(%q, %d) = %q (len %d), expected len %d",
					sev, testWidth, got, len(got), expected)
			}
		})
	}
}

func TestSprintSeverityZeroWidth(t *testing.T) {
	color.NoColor = true
	defer func() { color.NoColor = false }()

	got := SprintSeverity("INFO", 0)
	if got != "INFO" {
		t.Errorf("SprintSeverity(%q, 0) = %q, expected %q", "INFO", got, "INFO")
	}
}

func TestSprintSeverityColorMapping(t *testing.T) {
	color.NoColor = false
	defer func() { color.NoColor = false }()

	// ERROR and FATAL should produce the same color (red)
	errorOut := sprintSeverityColored("ERROR", testWidth)
	fatalOut := sprintSeverityColored("FATAL", testWidth)
	if len(errorOut) <= testWidth {
		t.Error("ERROR should have ANSI codes")
	}
	if len(fatalOut) <= testWidth {
		t.Error("FATAL should have ANSI codes")
	}

	// INFO should be colored (cyan)
	infoOut := sprintSeverityColored("INFO", testWidth)
	if len(infoOut) <= testWidth {
		t.Error("INFO should have ANSI codes")
	}

	// UNKNOWN should be colored (grey)
	unknownOut := sprintSeverityColored("UNKNOWN", testWidth)
	if len(unknownOut) <= testWidth {
		t.Error("UNKNOWN should have ANSI codes")
	}

	// DEBUG and TRACE should not be colored (fall through to default)
	debugOut := sprintSeverityColored("DEBUG", testWidth)
	traceOut := sprintSeverityColored("TRACE", testWidth)
	if len(debugOut) > testWidth {
		t.Error("DEBUG should not have ANSI codes")
	}
	if len(traceOut) > testWidth {
		t.Error("TRACE should not have ANSI codes")
	}
}
