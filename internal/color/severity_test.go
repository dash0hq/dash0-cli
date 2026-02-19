package color

import (
	"testing"

	"github.com/fatih/color"
)

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
			got := sprintSeverityColored(tc.severity)
			// Plain text is at most max(severityWidth, len(severity)) characters.
			// ANSI codes add escape sequences that make the string longer.
			plainLen := severityWidth
			if len(tc.severity) > plainLen {
				plainLen = len(tc.severity)
			}
			hasANSI := len(got) > plainLen
			if tc.wantANSI && !hasANSI {
				t.Errorf("sprintSeverityColored(%q) = %q, expected ANSI codes", tc.severity, got)
			}
			if !tc.wantANSI && hasANSI {
				t.Errorf("sprintSeverityColored(%q) = %q, did not expect ANSI codes", tc.severity, got)
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
			got := SprintSeverity(sev)
			// Should be plain text, padded to severityWidth
			if len(got) < severityWidth {
				t.Errorf("SprintSeverity(%q) = %q (len %d), expected at least %d characters",
					sev, got, len(got), severityWidth)
			}
			// No ANSI escape codes — length should be exactly max(severityWidth, len(sev))
			expected := severityWidth
			if len(sev) > expected {
				expected = len(sev)
			}
			if len(got) != expected {
				t.Errorf("SprintSeverity(%q) = %q (len %d), expected len %d",
					sev, got, len(got), expected)
			}
		})
	}
}

func TestSprintSeverityColorMapping(t *testing.T) {
	color.NoColor = false
	defer func() { color.NoColor = false }()

	// ERROR and FATAL should produce the same color (red)
	errorOut := sprintSeverityColored("ERROR")
	fatalOut := sprintSeverityColored("FATAL")
	if len(errorOut) <= severityWidth {
		t.Error("ERROR should have ANSI codes")
	}
	if len(fatalOut) <= severityWidth {
		t.Error("FATAL should have ANSI codes")
	}

	// INFO should be colored (cyan)
	infoOut := sprintSeverityColored("INFO")
	if len(infoOut) <= severityWidth {
		t.Error("INFO should have ANSI codes")
	}

	// UNKNOWN should be colored (grey)
	unknownOut := sprintSeverityColored("UNKNOWN")
	if len(unknownOut) <= severityWidth {
		t.Error("UNKNOWN should have ANSI codes")
	}

	// DEBUG and TRACE should not be colored (fall through to default)
	debugOut := sprintSeverityColored("DEBUG")
	traceOut := sprintSeverityColored("TRACE")
	if len(debugOut) > severityWidth {
		t.Error("DEBUG should not have ANSI codes")
	}
	if len(traceOut) > severityWidth {
		t.Error("TRACE should not have ANSI codes")
	}
}
