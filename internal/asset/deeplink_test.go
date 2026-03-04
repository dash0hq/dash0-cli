package asset

import (
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestViewDeeplinkURL(t *testing.T) {
	apiUrl := "https://api.us-west-2.aws.dash0.com"
	viewID := "d4e5f6a7-8901-23de-f012-456789abcdef"

	tests := []struct {
		name     string
		viewType string
		expected string
	}{
		{
			name:     "logs view",
			viewType: "logs",
			expected: "https://app.dash0.com/goto/logs?view_id=" + viewID,
		},
		{
			name:     "spans view",
			viewType: "spans",
			expected: "https://app.dash0.com/goto/traces/explorer?view_id=" + viewID,
		},
		{
			name:     "metrics view",
			viewType: "metrics",
			expected: "https://app.dash0.com/goto/metrics/explorer?view_id=" + viewID,
		},
		{
			name:     "services view",
			viewType: "services",
			expected: "https://app.dash0.com/goto/services/map?view_id=" + viewID,
		},
		{
			name:     "resources view",
			viewType: "resources",
			expected: "https://app.dash0.com/goto/resources/table?view_id=" + viewID,
		},
		{
			name:     "failed_checks view",
			viewType: "failed_checks",
			expected: "https://app.dash0.com/goto/alerting/failed-checks?view_id=" + viewID,
		},
		{
			name:     "web_events view",
			viewType: "web_events",
			expected: "https://app.dash0.com/goto/web-events/explorer?view_id=" + viewID,
		},
		{
			name:     "unknown view type",
			viewType: "unknown",
			expected: "",
		},
		{
			name:     "empty view type",
			viewType: "",
			expected: "",
		},
		{
			name:     "empty API URL",
			viewType: "logs",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testApiUrl := apiUrl
			if tt.name == "empty API URL" {
				testApiUrl = ""
			}
			result := ViewDeeplinkURL(testApiUrl, tt.viewType, viewID)
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

func TestLogsExplorerURL(t *testing.T) {
	apiUrl := "https://api.us-west-2.aws.dash0.com"

	t.Run("with filters and time range", func(t *testing.T) {
		filters := []DeeplinkFilter{
			{Key: "service.name", Operator: "is", Value: "my-service"},
		}
		result := LogsExplorerURL(apiUrl, filters, "now-1h", "now", nil)
		assert.Contains(t, result, "https://app.dash0.com/goto/logs?")
		assert.Contains(t, result, "from=now-1h")
		assert.Contains(t, result, "to=now")
		assert.Contains(t, result, "filter=")
	})

	t.Run("without filters", func(t *testing.T) {
		result := LogsExplorerURL(apiUrl, nil, "now-15m", "now", nil)
		assert.Contains(t, result, "https://app.dash0.com/goto/logs?")
		assert.Contains(t, result, "from=now-15m")
		assert.Contains(t, result, "to=now")
		assert.NotContains(t, result, "filter=")
	})

	t.Run("empty API URL", func(t *testing.T) {
		result := LogsExplorerURL("", nil, "now-15m", "now", nil)
		assert.Equal(t, "", result)
	})

	t.Run("with dataset", func(t *testing.T) {
		ds := "my-dataset"
		result := LogsExplorerURL(apiUrl, nil, "now-15m", "now", &ds)
		assert.Contains(t, result, "dataset=my-dataset")
	})

	t.Run("nil dataset", func(t *testing.T) {
		result := LogsExplorerURL(apiUrl, nil, "now-15m", "now", nil)
		assert.NotContains(t, result, "dataset=")
	})
}

func TestSpansExplorerURL(t *testing.T) {
	apiUrl := "https://api.us-west-2.aws.dash0.com"

	t.Run("with filters and time range", func(t *testing.T) {
		filters := []DeeplinkFilter{
			{Key: "service.name", Operator: "is", Value: "my-service"},
		}
		result := SpansExplorerURL(apiUrl, filters, "now-1h", "now", nil)
		assert.Contains(t, result, "https://app.dash0.com/goto/traces/explorer?")
		assert.Contains(t, result, "filter=")
		assert.Contains(t, result, "from=now-1h")
		assert.Contains(t, result, "to=now")
		assert.NotContains(t, result, "trace_id=")
	})

	t.Run("without filters", func(t *testing.T) {
		result := SpansExplorerURL(apiUrl, nil, "now-15m", "now", nil)
		assert.Contains(t, result, "https://app.dash0.com/goto/traces/explorer?")
		assert.NotContains(t, result, "filter=")
	})

	t.Run("empty API URL", func(t *testing.T) {
		result := SpansExplorerURL("", nil, "now-1h", "now", nil)
		assert.Equal(t, "", result)
	})
}

func TestTracesExplorerURL(t *testing.T) {
	apiUrl := "https://api.us-west-2.aws.dash0.com"

	t.Run("with trace ID", func(t *testing.T) {
		result := TracesExplorerURL(apiUrl, "0af7651916cd43dd8448eb211c80319c", nil)
		assert.Contains(t, result, "https://app.dash0.com/goto/traces/explorer?")
		assert.Contains(t, result, "trace_id=0af7651916cd43dd8448eb211c80319c")
		assert.NotContains(t, result, "from=")
		assert.NotContains(t, result, "to=")
		assert.NotContains(t, result, "filter=")
	})

	t.Run("with dataset", func(t *testing.T) {
		ds := "my-dataset"
		result := TracesExplorerURL(apiUrl, "abc123", &ds)
		assert.Contains(t, result, "trace_id=abc123")
		assert.Contains(t, result, "dataset=my-dataset")
	})

	t.Run("empty API URL", func(t *testing.T) {
		result := TracesExplorerURL("", "abc123", nil)
		assert.Equal(t, "", result)
	})
}

func TestFiltersToDeeplinkFilters(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		result := FiltersToDeeplinkFilters(nil)
		assert.Nil(t, result)
	})

	t.Run("single-value filter", func(t *testing.T) {
		var val dash0api.AttributeFilter_Value
		require.NoError(t, val.FromAttributeFilterStringValue("my-service"))
		filters := dash0api.FilterCriteria{
			{Key: "service.name", Operator: dash0api.AttributeFilterOperatorIs, Value: &val},
		}
		result := FiltersToDeeplinkFilters(&filters)
		require.Len(t, result, 1)
		assert.Equal(t, "service.name", result[0].Key)
		assert.Equal(t, "is", result[0].Operator)
		assert.Equal(t, "my-service", result[0].Value)
	})

	t.Run("multi-value filter", func(t *testing.T) {
		var item1, item2 dash0api.AttributeFilter_Values_Item
		require.NoError(t, item1.FromAttributeFilterStringValue("ERROR"))
		require.NoError(t, item2.FromAttributeFilterStringValue("WARN"))
		items := []dash0api.AttributeFilter_Values_Item{item1, item2}
		filters := dash0api.FilterCriteria{
			{Key: "otel.log.severity.range", Operator: dash0api.AttributeFilterOperatorIsOneOf, Values: &items},
		}
		result := FiltersToDeeplinkFilters(&filters)
		require.Len(t, result, 1)
		assert.Equal(t, "otel.log.severity.range", result[0].Key)
		assert.Equal(t, "is_one_of", result[0].Operator)
		assert.Equal(t, "ERROR WARN", result[0].Value)
	})

	t.Run("no-value filter", func(t *testing.T) {
		filters := dash0api.FilterCriteria{
			{Key: "error.message", Operator: dash0api.AttributeFilterOperatorIsSet},
		}
		result := FiltersToDeeplinkFilters(&filters)
		require.Len(t, result, 1)
		assert.Equal(t, "error.message", result[0].Key)
		assert.Equal(t, "is_set", result[0].Operator)
		assert.Equal(t, "", result[0].Value)
	})
}
