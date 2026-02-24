package asset

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeeplinkURL(t *testing.T) {
	tests := []struct {
		name      string
		apiUrl    string
		assetType string
		assetID   string
		expected  string
	}{
		{
			name:      "dashboard",
			apiUrl:    "https://api.us-west-2.aws.dash0.com",
			assetType: "dashboard",
			assetID:   "a1b2c3d4-5678-90ab-cdef-1234567890ab",
			expected:  "https://app.dash0.com/goto/dashboards?dashboard_id=a1b2c3d4-5678-90ab-cdef-1234567890ab",
		},
		{
			name:      "check rule",
			apiUrl:    "https://api.eu-west-1.aws.dash0.com",
			assetType: "check rule",
			assetID:   "b2c3d4e5-6789-01bc-def0-234567890abc",
			expected:  "https://app.dash0.com/goto/alerting/check-rules?check_rule_id=b2c3d4e5-6789-01bc-def0-234567890abc",
		},
		{
			name:      "synthetic check",
			apiUrl:    "https://api.us-west-2.aws.dash0.com",
			assetType: "synthetic check",
			assetID:   "c3d4e5f6-7890-12cd-ef01-34567890abcd",
			expected:  "https://app.dash0.com/goto/alerting/synthetics?check_id=c3d4e5f6-7890-12cd-ef01-34567890abcd",
		},
		{
			name:      "view",
			apiUrl:    "https://api.us-west-2.aws.dash0.com",
			assetType: "view",
			assetID:   "d4e5f6a7-8901-23de-f012-456789abcdef",
			expected:  "https://app.dash0.com/goto/logs?view_id=d4e5f6a7-8901-23de-f012-456789abcdef",
		},
		{
			name:      "custom deployment domain",
			apiUrl:    "https://api.custom.example.io",
			assetType: "dashboard",
			assetID:   "abc123",
			expected:  "https://app.example.io/goto/dashboards?dashboard_id=abc123",
		},
		{
			name:      "empty API URL",
			apiUrl:    "",
			assetType: "dashboard",
			assetID:   "abc123",
			expected:  "",
		},
		{
			name:      "unknown asset type",
			apiUrl:    "https://api.us-west-2.aws.dash0.com",
			assetType: "unknown",
			assetID:   "abc123",
			expected:  "",
		},
		{
			name:      "checkrule single word",
			apiUrl:    "https://api.us-west-2.aws.dash0.com",
			assetType: "checkrule",
			assetID:   "abc123",
			expected:  "https://app.dash0.com/goto/alerting/check-rules?check_rule_id=abc123",
		},
		{
			name:      "prometheusrule maps to check rule",
			apiUrl:    "https://api.us-west-2.aws.dash0.com",
			assetType: "prometheusrule",
			assetID:   "abc123",
			expected:  "https://app.dash0.com/goto/alerting/check-rules?check_rule_id=abc123",
		},
		{
			name:      "team",
			apiUrl:    "https://api.us-west-2.aws.dash0.com",
			assetType: "team",
			assetID:   "a1b2c3d4-5678-90ab-cdef-1234567890ab",
			expected:  "https://app.dash0.com/goto/settings/teams?team_id=a1b2c3d4-5678-90ab-cdef-1234567890ab",
		},
		{
			name:      "member",
			apiUrl:    "https://api.us-west-2.aws.dash0.com",
			assetType: "member",
			assetID:   "user_2TOneA9gdtUmokQ1QBbl0JPrml5",
			expected:  "https://app.dash0.com/goto/settings/members?member_id=user_2TOneA9gdtUmokQ1QBbl0JPrml5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DeeplinkURL(tt.apiUrl, tt.assetType, tt.assetID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDomainSuffix(t *testing.T) {
	tests := []struct {
		hostname string
		expected string
	}{
		{"api.us-west-2.aws.dash0.com", "dash0.com"},
		{"api.eu-west-1.aws.dash0.com", "dash0.com"},
		{"api.custom.example.io", "example.io"},
		{"localhost", ""},
		{"dash0.com", "dash0.com"},
	}

	for _, tt := range tests {
		t.Run(tt.hostname, func(t *testing.T) {
			assert.Equal(t, tt.expected, domainSuffix(tt.hostname))
		})
	}
}

func TestDeeplinkBaseURL(t *testing.T) {
	tests := []struct {
		apiUrl   string
		expected string
	}{
		{"https://api.us-west-2.aws.dash0.com", "https://app.dash0.com"},
		{"https://api.eu-west-1.aws.dash0.com", "https://app.dash0.com"},
		{"https://api.custom.example.io", "https://app.example.io"},
		{"", ""},
		{"not-a-url", ""},
	}

	for _, tt := range tests {
		t.Run(tt.apiUrl, func(t *testing.T) {
			assert.Equal(t, tt.expected, deeplinkBaseURL(tt.apiUrl))
		})
	}
}
