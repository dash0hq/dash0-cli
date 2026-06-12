//go:build !windows

package otlp

import (
	"strings"
	"testing"
)

func TestPortInUseHint_Unix_UsesLsof(t *testing.T) {
	got := portInUseHint(4318)
	if !strings.HasPrefix(got, "lsof ") {
		t.Errorf("Unix hint should start with 'lsof '; got %q", got)
	}
	if !strings.Contains(got, "4318") {
		t.Errorf("hint should reference the port; got %q", got)
	}
}
