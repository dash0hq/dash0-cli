package asset

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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

func TestNormalizeKind(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Dashboard", "dashboard"},
		{"dashboard", "dashboard"},
		{"DASHBOARD", "dashboard"},
		{"CheckRule", "checkrule"},
		{"check-rule", "checkrule"},
		{"check_rule", "checkrule"},
		{"Dash0Dashboard", "dashboard"},
		{"Dash0CheckRule", "checkrule"},
		{"PrometheusRule", "prometheusrule"},
		{"SyntheticCheck", "syntheticcheck"},
		{"synthetic-check", "syntheticcheck"},
		{"View", "view"},
		{"Dash0View", "view"},
		{"PersesDashboard", "persesdashboard"},
		{"persesdashboard", "persesdashboard"},
		{"Dash0NotificationChannel", "notificationchannel"},
		{"Dash0SpamFilter", "spamfilter"},
		{"notification-channel", "notificationchannel"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := NormalizeKind(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsValidKind(t *testing.T) {
	validKinds := []string{
		"Dashboard",
		"dashboard",
		"CheckRule",
		"checkrule",
		"check-rule",
		"PrometheusRule",
		"prometheusrule",
		"SyntheticCheck",
		"syntheticcheck",
		"synthetic-check",
		"View",
		"view",
		"Dash0Dashboard",
		"Dash0View",
		"PersesDashboard",
		"persesdashboard",
		"Dash0SpamFilter",
		"Dash0NotificationChannel",
		"notification-channel",
	}

	for _, kind := range validKinds {
		t.Run("valid_"+kind, func(t *testing.T) {
			assert.True(t, IsValidKind(kind), "expected %q to be valid", kind)
		})
	}

	invalidKinds := []string{
		"Unknown",
		"Pod",
		"Deployment",
		"ConfigMap",
		"",
		"   ",
	}

	for _, kind := range invalidKinds {
		t.Run("invalid_"+kind, func(t *testing.T) {
			assert.False(t, IsValidKind(kind), "expected %q to be invalid", kind)
		})
	}
}
