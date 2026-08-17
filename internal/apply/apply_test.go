package apply

import (
	"strings"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			result := normalizeKind(tt.input)
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
			assert.True(t, isValidKind(kind), "expected %q to be valid", kind)
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
			assert.False(t, isValidKind(kind), "expected %q to be invalid", kind)
		})
	}
}

func TestApplyAction_String(t *testing.T) {
	assert.Equal(t, "created", string(actionCreated))
	assert.Equal(t, "updated", string(actionUpdated))
}

func TestValidateDocuments_NotificationChannelAssetsWarning(t *testing.T) {
	yaml := `kind: Dash0NotificationChannel
metadata:
  name: Channel With Assets
spec:
  type: webhook
  config:
    url: https://example.com/webhook
  routing:
    filters: []
    assets:
      - kind: check_rule
        id: 00000000-0000-0000-0000-000000000001
        name: some rule
`
	docs, err := asset.ReadMultiDocumentYAML("-", strings.NewReader(yaml))
	require.NoError(t, err)

	validationErrors, validationWarnings := validateDocuments(docs)
	assert.Empty(t, validationErrors, "the API-managed assets warning must not become a validation error")
	require.Len(t, validationWarnings, 1)
	assert.Contains(t, validationWarnings[0], "spec.routing.assets is API-managed and ignored on write")
}
