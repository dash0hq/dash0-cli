//go:build windows

package otlp

import (
	"strings"
	"testing"
)

func TestPortInUseHint_Windows_UsesNetstat(t *testing.T) {
	got := portInUseHint(4318)
	if !strings.HasPrefix(got, "netstat ") {
		t.Errorf("Windows hint should start with 'netstat '; got %q", got)
	}
	if !strings.Contains(got, "4318") {
		t.Errorf("hint should reference the port; got %q", got)
	}
}
