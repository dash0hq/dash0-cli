package metrics

import (
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	attrServiceName               = "service.name"
	attrDeploymentEnvironmentName = "deployment.environment.name"
)

func stringValue(s string) *dash0api.AttributeFilter_Value {
	v := dash0api.AttributeFilter_Value{}
	_ = v.FromAttributeFilterStringValue(s)
	return &v
}

func stringValues(ss ...string) *[]dash0api.AttributeFilter_Values_Item {
	items := make([]dash0api.AttributeFilter_Values_Item, len(ss))
	for i, s := range ss {
		_ = items[i].FromAttributeFilterStringValue(s)
	}
	return &items
}

func TestFiltersToPromQL(t *testing.T) {
	tests := []struct {
		name    string
		filters dash0api.FilterCriteria
		want    string
		wantErr string
	}{
		{
			name: "is operator",
			filters: dash0api.FilterCriteria{
				{Key: attrServiceName, Operator: dash0api.AttributeFilterOperatorIs, Value: stringValue("my-service")},
			},
			want: `{service_name="my-service"}`,
		},
		{
			name: "is_not operator",
			filters: dash0api.FilterCriteria{
				{Key: "job", Operator: dash0api.AttributeFilterOperatorIsNot, Value: stringValue("test")},
			},
			want: `{job!="test"}`,
		},
		{
			name: "matches operator",
			filters: dash0api.FilterCriteria{
				{Key: "instance", Operator: dash0api.AttributeFilterOperatorMatches, Value: stringValue("api.*")},
			},
			want: `{instance=~"api.*"}`,
		},
		{
			name: "does_not_match operator",
			filters: dash0api.FilterCriteria{
				{Key: "job", Operator: dash0api.AttributeFilterOperatorDoesNotMatch, Value: stringValue("test.*")},
			},
			want: `{job!~"test.*"}`,
		},
		{
			name: "multiple filters",
			filters: dash0api.FilterCriteria{
				{Key: attrServiceName, Operator: dash0api.AttributeFilterOperatorIs, Value: stringValue("api")},
				{Key: "job", Operator: dash0api.AttributeFilterOperatorIsNot, Value: stringValue("test")},
			},
			want: `{service_name="api",job!="test"}`,
		},
		{
			name: "dot-to-underscore normalization",
			filters: dash0api.FilterCriteria{
				{Key: attrDeploymentEnvironmentName, Operator: dash0api.AttributeFilterOperatorIs, Value: stringValue("prod")},
			},
			want: `{deployment_environment_name="prod"}`,
		},
		{
			name: "contains desugars to regex",
			filters: dash0api.FilterCriteria{
				{Key: "job", Operator: dash0api.AttributeFilterOperatorContains, Value: stringValue("api")},
			},
			want: `{job=~".*api.*"}`,
		},
		{
			name: "does_not_contain desugars to negative regex",
			filters: dash0api.FilterCriteria{
				{Key: "job", Operator: dash0api.AttributeFilterOperatorDoesNotContain, Value: stringValue("test")},
			},
			want: `{job!~".*test.*"}`,
		},
		{
			name: "starts_with desugars to regex",
			filters: dash0api.FilterCriteria{
				{Key: "job", Operator: dash0api.AttributeFilterOperatorStartsWith, Value: stringValue("api")},
			},
			want: `{job=~"^api.*"}`,
		},
		{
			name: "does_not_start_with desugars to negative regex",
			filters: dash0api.FilterCriteria{
				{Key: "job", Operator: dash0api.AttributeFilterOperatorDoesNotStartWith, Value: stringValue("test")},
			},
			want: `{job!~"^test.*"}`,
		},
		{
			name: "ends_with desugars to regex",
			filters: dash0api.FilterCriteria{
				{Key: "job", Operator: dash0api.AttributeFilterOperatorEndsWith, Value: stringValue("-prod")},
			},
			want: `{job=~".*-prod$"}`,
		},
		{
			name: "does_not_end_with desugars to negative regex",
			filters: dash0api.FilterCriteria{
				{Key: "job", Operator: dash0api.AttributeFilterOperatorDoesNotEndWith, Value: stringValue("-test")},
			},
			want: `{job!~".*-test$"}`,
		},
		{
			name: "is_set desugars to not-empty",
			filters: dash0api.FilterCriteria{
				{Key: "job", Operator: dash0api.AttributeFilterOperatorIsSet},
			},
			want: `{job!=""}`,
		},
		{
			name: "is_not_set desugars to empty",
			filters: dash0api.FilterCriteria{
				{Key: "job", Operator: dash0api.AttributeFilterOperatorIsNotSet},
			},
			want: `{job=""}`,
		},
		{
			name: "is_one_of desugars to regex alternation",
			filters: dash0api.FilterCriteria{
				{Key: "job", Operator: dash0api.AttributeFilterOperatorIsOneOf, Values: stringValues("api", "web")},
			},
			want: `{job=~"^(api|web)$"}`,
		},
		{
			name: "is_not_one_of desugars to negative regex alternation",
			filters: dash0api.FilterCriteria{
				{Key: "job", Operator: dash0api.AttributeFilterOperatorIsNotOneOf, Values: stringValues("test", "debug")},
			},
			want: `{job!~"^(test|debug)$"}`,
		},
		{
			name: "gt operator",
			filters: dash0api.FilterCriteria{
				{Key: "http.status_code", Operator: dash0api.AttributeFilterOperatorGt, Value: stringValue("400")},
			},
			want: `{http_status_code>"400"}`,
		},
		{
			name: "value with special characters is escaped",
			filters: dash0api.FilterCriteria{
				{Key: "path", Operator: dash0api.AttributeFilterOperatorIs, Value: stringValue(`/api/"test"\path`)},
			},
			want: `{path="/api/\"test\"\\path"}`,
		},
		{
			name: "contains with regex metacharacters escapes them",
			filters: dash0api.FilterCriteria{
				{Key: "path", Operator: dash0api.AttributeFilterOperatorContains, Value: stringValue("foo.bar")},
			},
			want: `{path=~".*foo\.bar.*"}`,
		},
		{
			name: "is_one_of with regex metacharacters escapes them",
			filters: dash0api.FilterCriteria{
				{Key: "path", Operator: dash0api.AttributeFilterOperatorIsOneOf, Values: stringValues("a.b", "c+d")},
			},
			want: `{path=~"^(a\.b|c\+d)$"}`,
		},
		{
			name: "unsupported operator returns error",
			filters: dash0api.FilterCriteria{
				{Key: "job", Operator: dash0api.AttributeFilterOperatorIsAny},
			},
			wantErr: "not supported for metrics queries",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := filtersToPromQL(&tt.filters)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestNormalizeKey(t *testing.T) {
	assert.Equal(t, "service_name", normalizeKey(attrServiceName))
	assert.Equal(t, "job", normalizeKey("job"))
	assert.Equal(t, "deployment_environment_name", normalizeKey(attrDeploymentEnvironmentName))
}

func TestEscapePromQLString(t *testing.T) {
	assert.Equal(t, `hello`, escapePromQLString(`hello`))
	assert.Equal(t, `say \"hi\"`, escapePromQLString(`say "hi"`))
	assert.Equal(t, `back\\slash`, escapePromQLString(`back\slash`))
}
