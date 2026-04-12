package agent0

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatTagsService(t *testing.T) {
	input := "the <Service name='api' namespace='dash0'></Service> service"
	result := formatTags(input)
	assert.Equal(t, "the `api` (dash0) service", result)
}

func TestFormatTagsServiceNoNamespace(t *testing.T) {
	input := "<Service name='frontend'></Service>"
	result := formatTags(input)
	assert.Equal(t, "`frontend`", result)
}

func TestFormatTagsTimeRangeSameDay(t *testing.T) {
	input := "<TimeRange from='2026-04-12T17:00:00Z' to='2026-04-12T18:00:00Z'></TimeRange>"
	result := formatTags(input)
	// Should show just times since same day
	assert.Contains(t, result, "–")
	assert.NotContains(t, result, "2026")
}

func TestFormatTagsTimeRangeCrossDay(t *testing.T) {
	// Use dates far enough apart that they're different days in any timezone
	input := "<TimeRange from='2026-04-10T10:00:00Z' to='2026-04-12T10:00:00Z'></TimeRange>"
	result := formatTags(input)
	assert.Contains(t, result, "–")
	assert.Contains(t, result, "Apr")
}

func TestFormatTagsMetric(t *testing.T) {
	input := "<Metric name='http_requests_total' type='counter'></Metric>"
	result := formatTags(input)
	assert.Equal(t, "`http_requests_total` (counter)", result)
}

func TestFormatTagsPromql(t *testing.T) {
	input := "<Promql expression='sum(rate(http_requests_total[5m]))'></Promql>"
	result := formatTags(input)
	assert.Equal(t, "`sum(rate(http_requests_total[5m]))`", result)
}

func TestFormatTagsDocument(t *testing.T) {
	input := "<Document url='https://docs.dash0.com/api' title='API Docs'></Document>"
	result := formatTags(input)
	assert.Equal(t, "[API Docs](https://docs.dash0.com/api)", result)
}

func TestFormatTagsLinearIssue(t *testing.T) {
	input := "<LinearIssue identifier='AI-863'></LinearIssue>"
	result := formatTags(input)
	assert.Equal(t, "AI-863", result)
}

func TestFormatTagsGithub(t *testing.T) {
	input := "<Github url='https://github.com/dash0hq/dash0/pull/123' entityType='pull_request'></Github>"
	result := formatTags(input)
	assert.Equal(t, "github.com/dash0hq/dash0/pull/123", result)
}

func TestFormatTagsMultipleTags(t *testing.T) {
	input := "Errors in <Service name='api' namespace='dash0'></Service> during <TimeRange from='2026-04-12T17:00:00Z' to='2026-04-12T18:00:00Z'></TimeRange>"
	result := formatTags(input)
	assert.Contains(t, result, "`api` (dash0)")
	assert.Contains(t, result, "–")
	assert.NotContains(t, result, "<Service")
	assert.NotContains(t, result, "<TimeRange")
}

func TestFormatTagsNoTags(t *testing.T) {
	input := "plain text with no tags"
	assert.Equal(t, input, formatTags(input))
}

func TestFormatTagsRealAgent0Response(t *testing.T) {
	// Based on the actual agent0 response format from the debug log
	input := "I searched for errors in the <Service name='agents' namespace='dash0'></Service> component across three time periods.\n\n**Last Hour** (<TimeRange from='2026-04-12T17:23:11.651294219Z' to='2026-04-12T18:23:11.651294219Z'></TimeRange>)\n- **0 errors found**"
	result := formatTags(input)

	assert.Contains(t, result, "`agents` (dash0)")
	assert.NotContains(t, result, "<Service")
	assert.NotContains(t, result, "<TimeRange")
	assert.Contains(t, result, "–")
	assert.Contains(t, result, "0 errors found")
}
