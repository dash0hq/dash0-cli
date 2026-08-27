package asset

import "testing"

// TestKindDisplayName_Team is a regression test for a bug where
// KindDisplayName had no case for "team", so any caller relying on this
// function for a team asset (e.g. the teams command group, or --since's
// deletion messages once it exists) printed the raw normalized kind string
// "team" instead of a proper display name.
func TestKindDisplayName_Team(t *testing.T) {
	for _, kind := range []string{"team", "Team", "Dash0Team", "dash0-team"} {
		if got := KindDisplayName(kind); got != "Team" {
			t.Errorf("KindDisplayName(%q) = %q, want %q", kind, got, "Team")
		}
	}
}

func TestKindDisplayName_KnownKinds(t *testing.T) {
	cases := map[string]string{
		"Dashboard":                "Dashboard",
		"CheckRule":                "Check rule",
		"SyntheticCheck":           "Synthetic check",
		"View":                     "View",
		"PrometheusRule":           "PrometheusRule",
		"PersesDashboard":          "PersesDashboard",
		"Dash0NotificationChannel": "Notification channel",
		"Dash0SpamFilter":          "Spam filter",
		"Dash0Team":                "Team",
	}
	for kind, want := range cases {
		if got := KindDisplayName(kind); got != want {
			t.Errorf("KindDisplayName(%q) = %q, want %q", kind, got, want)
		}
	}
}
