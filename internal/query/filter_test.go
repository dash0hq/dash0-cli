package query

import (
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFilter_SimpleKeyValue(t *testing.T) {
	filter, err := ParseFilter("service.name my-service")
	require.NoError(t, err)
	assert.Equal(t, "service.name", filter.Key)
	assert.Equal(t, dash0api.AttributeFilterOperatorIs, filter.Operator)
	require.NotNil(t, filter.Value)
}

func TestParseFilter_ExplicitOperator(t *testing.T) {
	filter, err := ParseFilter("service.name contains api")
	require.NoError(t, err)
	assert.Equal(t, "service.name", filter.Key)
	assert.Equal(t, dash0api.AttributeFilterOperatorContains, filter.Operator)
	require.NotNil(t, filter.Value)
}

func TestParseFilter_SingleQuotedKey(t *testing.T) {
	filter, err := ParseFilter("'my key with spaces' is hello")
	require.NoError(t, err)
	assert.Equal(t, "my key with spaces", filter.Key)
	assert.Equal(t, dash0api.AttributeFilterOperatorIs, filter.Operator)
	require.NotNil(t, filter.Value)
}

func TestParseFilter_ValueWithSpaces(t *testing.T) {
	filter, err := ParseFilter("body contains hello world")
	require.NoError(t, err)
	assert.Equal(t, "body", filter.Key)
	assert.Equal(t, dash0api.AttributeFilterOperatorContains, filter.Operator)
	require.NotNil(t, filter.Value)
}

func TestParseFilter_ImplicitIs_ValueWithSpaces(t *testing.T) {
	filter, err := ParseFilter("service.name my cool service")
	require.NoError(t, err)
	assert.Equal(t, "service.name", filter.Key)
	assert.Equal(t, dash0api.AttributeFilterOperatorIs, filter.Operator)
	require.NotNil(t, filter.Value)
}

func TestParseFilter_IsSet(t *testing.T) {
	filter, err := ParseFilter("error.type is_set")
	require.NoError(t, err)
	assert.Equal(t, "error.type", filter.Key)
	assert.Equal(t, dash0api.AttributeFilterOperatorIsSet, filter.Operator)
	assert.Nil(t, filter.Value)
}

func TestParseFilter_IsNotSet(t *testing.T) {
	filter, err := ParseFilter("error.type is_not_set")
	require.NoError(t, err)
	assert.Equal(t, "error.type", filter.Key)
	assert.Equal(t, dash0api.AttributeFilterOperatorIsNotSet, filter.Operator)
	assert.Nil(t, filter.Value)
}

func TestParseFilter_EmptyStringIsNotSet(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		wantOp dash0api.AttributeFilterOperator
	}{
		{`is ""`, `error.type is ""`, dash0api.AttributeFilterOperatorIsNotSet},
		{`= ""`, `error.type = ""`, dash0api.AttributeFilterOperatorIsNotSet},
		{`is ''`, `error.type is ''`, dash0api.AttributeFilterOperatorIsNotSet},
		{`is_not ""`, `error.type is_not ""`, dash0api.AttributeFilterOperatorIsSet},
		{`!= ""`, `error.type != ""`, dash0api.AttributeFilterOperatorIsSet},
		{`!= ''`, `error.type != ''`, dash0api.AttributeFilterOperatorIsSet},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter, err := ParseFilter(tt.input)
			require.NoError(t, err)
			assert.Equal(t, "error.type", filter.Key)
			assert.Equal(t, tt.wantOp, filter.Operator)
			assert.Nil(t, filter.Value)
		})
	}
}

func TestParseFilter_IsOneOf(t *testing.T) {
	filter, err := ParseFilter("severity is_one_of ERROR WARN FATAL")
	require.NoError(t, err)
	assert.Equal(t, "severity", filter.Key)
	assert.Equal(t, dash0api.AttributeFilterOperatorIsOneOf, filter.Operator)
	assert.Nil(t, filter.Value)
	require.NotNil(t, filter.Values)
	assert.Len(t, *filter.Values, 3)
}

func TestParseFilter_IsOneOf_QuotedValues(t *testing.T) {
	filter, err := ParseFilter("severity is_one_of 'ER ROR' WARN FATAL")
	require.NoError(t, err)
	assert.Equal(t, "severity", filter.Key)
	assert.Equal(t, dash0api.AttributeFilterOperatorIsOneOf, filter.Operator)
	assert.Nil(t, filter.Value)
	require.NotNil(t, filter.Values)
	require.Len(t, *filter.Values, 3)
}

func TestParseFilter_IsNotOneOf(t *testing.T) {
	filter, err := ParseFilter("service.name is_not_one_of frontend gateway")
	require.NoError(t, err)
	assert.Equal(t, "service.name", filter.Key)
	assert.Equal(t, dash0api.AttributeFilterOperatorIsNotOneOf, filter.Operator)
	assert.Nil(t, filter.Value)
	require.NotNil(t, filter.Values)
	assert.Len(t, *filter.Values, 2)
}

func TestParseFilter_IsOneOf_UnclosedQuote(t *testing.T) {
	_, err := ParseFilter("severity is_one_of 'unclosed ERROR")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unclosed single quote")
}

func TestParseFilter_ComparisonOperators(t *testing.T) {
	tests := []struct {
		input  string
		wantOp dash0api.AttributeFilterOperator
	}{
		{"latency gt 100", dash0api.AttributeFilterOperatorGt},
		{"latency gte 100", dash0api.AttributeFilterOperatorGte},
		{"latency lt 100", dash0api.AttributeFilterOperatorLt},
		{"latency lte 100", dash0api.AttributeFilterOperatorLte},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			filter, err := ParseFilter(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantOp, filter.Operator)
		})
	}
}

func TestParseFilter_SymbolicAliases(t *testing.T) {
	tests := []struct {
		input  string
		wantOp dash0api.AttributeFilterOperator
	}{
		{"service.name = my-service", dash0api.AttributeFilterOperatorIs},
		{"service.name != my-service", dash0api.AttributeFilterOperatorIsNot},
		{"latency > 100", dash0api.AttributeFilterOperatorGt},
		{"latency >= 100", dash0api.AttributeFilterOperatorGte},
		{"latency < 100", dash0api.AttributeFilterOperatorLt},
		{"latency <= 100", dash0api.AttributeFilterOperatorLte},
		{"body ~ error.*timeout", dash0api.AttributeFilterOperatorMatches},
		{"body !~ error.*timeout", dash0api.AttributeFilterOperatorDoesNotMatch},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			filter, err := ParseFilter(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantOp, filter.Operator)
			require.NotNil(t, filter.Value)
		})
	}
}

func TestParseFilter_ErrorCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", "empty filter expression"},
		{"key only", "service.name", "filter requires at least a key and value"},
		{"unclosed quote", "'my key value", "unclosed single quote"},
		{"is_set with value", "key is_set extra", "does not accept a value"},
		{"operator without value", "key contains", "requires a value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseFilter(tt.input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestSplitQuotedTokens(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{"simple", "ERROR WARN FATAL", []string{"ERROR", "WARN", "FATAL"}, false},
		{"single value", "ERROR", []string{"ERROR"}, false},
		{"quoted value", "'ER ROR' WARN", []string{"ER ROR", "WARN"}, false},
		{"multiple quoted", "'a b' 'c d'", []string{"a b", "c d"}, false},
		{"mixed", "plain 'with space' another", []string{"plain", "with space", "another"}, false},
		{"extra whitespace", "  ERROR   WARN  ", []string{"ERROR", "WARN"}, false},
		{"unclosed quote", "'unclosed", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := splitQuotedTokens(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestParseFilters_Empty(t *testing.T) {
	filters, err := ParseFilters(nil)
	require.NoError(t, err)
	assert.Nil(t, filters)
}

func TestParseFilters_Multiple(t *testing.T) {
	filters, err := ParseFilters([]string{
		"service.name is my-service",
		"severity is_one_of ERROR WARN",
	})
	require.NoError(t, err)
	require.NotNil(t, filters)
	assert.Len(t, *filters, 2)
}

func TestParseFilters_InvalidReturnsError(t *testing.T) {
	_, err := ParseFilters([]string{""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty filter expression")
}
