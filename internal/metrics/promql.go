package metrics

import (
	"fmt"
	"regexp"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// filtersToPromQL translates parsed Dash0 filter criteria to a PromQL selector string.
// The result is a bare selector like `{service_name="foo",job=~"api.*"}`.
func filtersToPromQL(filters *dash0api.FilterCriteria) (string, error) {
	if filters == nil || len(*filters) == 0 {
		return "", fmt.Errorf("no filters provided")
	}

	matchers := make([]string, 0, len(*filters))
	for _, f := range *filters {
		m, err := filterToMatcher(f)
		if err != nil {
			return "", err
		}
		matchers = append(matchers, m)
	}

	return "{" + strings.Join(matchers, ",") + "}", nil
}

// filterToMatcher converts a single AttributeFilter to a PromQL label matcher string.
func filterToMatcher(f dash0api.AttributeFilter) (string, error) {
	key := normalizeKey(f.Key)

	// Desugar complex operators into basic ones.
	desugared, err := desugarFilter(f)
	if err != nil {
		return "", err
	}
	if desugared != nil {
		return fmt.Sprintf(`%s%s%s`, key, desugared.operator, desugared.value), nil
	}

	// Map basic operators to PromQL.
	promqlOp, ok := operatorMap[f.Operator]
	if !ok {
		return "", fmt.Errorf("filter operator %q is not supported for metrics queries; supported operators: is, is_not, matches, does_not_match, contains, starts_with, ends_with, is_one_of, is_set, is_not_set, gt, gte, lt, lte (and their aliases)", f.Operator)
	}

	value, err := extractFilterValue(f)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(`%s%s"%s"`, key, promqlOp, escapePromQLString(value)), nil
}

// operatorMap maps basic Dash0 filter operators to PromQL operators.
var operatorMap = map[dash0api.AttributeFilterOperator]string{
	dash0api.AttributeFilterOperatorIs:           `=`,
	dash0api.AttributeFilterOperatorIsNot:        `!=`,
	dash0api.AttributeFilterOperatorMatches:      `=~`,
	dash0api.AttributeFilterOperatorDoesNotMatch: `!~`,
	dash0api.AttributeFilterOperatorGt:           `>`,
	dash0api.AttributeFilterOperatorGte:          `>=`,
	dash0api.AttributeFilterOperatorLt:           `<`,
	dash0api.AttributeFilterOperatorLte:          `<=`,
}

type desugaredMatcher struct {
	operator string
	value    string // already formatted with quotes
}

// desugarFilter converts complex operators to basic PromQL matchers.
// Returns nil if the operator does not need desugaring.
func desugarFilter(f dash0api.AttributeFilter) (*desugaredMatcher, error) {
	switch f.Operator {
	case dash0api.AttributeFilterOperatorIsSet:
		return &desugaredMatcher{operator: `!=`, value: `""`}, nil
	case dash0api.AttributeFilterOperatorIsNotSet:
		return &desugaredMatcher{operator: `=`, value: `""`}, nil

	case dash0api.AttributeFilterOperatorContains:
		v, err := extractFilterValue(f)
		if err != nil {
			return nil, err
		}
		return &desugaredMatcher{
			operator: `=~`,
			value:    fmt.Sprintf(`".*%s.*"`, regexp.QuoteMeta(v)),
		}, nil

	case dash0api.AttributeFilterOperatorDoesNotContain:
		v, err := extractFilterValue(f)
		if err != nil {
			return nil, err
		}
		return &desugaredMatcher{
			operator: `!~`,
			value:    fmt.Sprintf(`".*%s.*"`, regexp.QuoteMeta(v)),
		}, nil

	case dash0api.AttributeFilterOperatorStartsWith:
		v, err := extractFilterValue(f)
		if err != nil {
			return nil, err
		}
		return &desugaredMatcher{
			operator: `=~`,
			value:    fmt.Sprintf(`"^%s.*"`, regexp.QuoteMeta(v)),
		}, nil

	case dash0api.AttributeFilterOperatorDoesNotStartWith:
		v, err := extractFilterValue(f)
		if err != nil {
			return nil, err
		}
		return &desugaredMatcher{
			operator: `!~`,
			value:    fmt.Sprintf(`"^%s.*"`, regexp.QuoteMeta(v)),
		}, nil

	case dash0api.AttributeFilterOperatorEndsWith:
		v, err := extractFilterValue(f)
		if err != nil {
			return nil, err
		}
		return &desugaredMatcher{
			operator: `=~`,
			value:    fmt.Sprintf(`".*%s$"`, regexp.QuoteMeta(v)),
		}, nil

	case dash0api.AttributeFilterOperatorDoesNotEndWith:
		v, err := extractFilterValue(f)
		if err != nil {
			return nil, err
		}
		return &desugaredMatcher{
			operator: `!~`,
			value:    fmt.Sprintf(`".*%s$"`, regexp.QuoteMeta(v)),
		}, nil

	case dash0api.AttributeFilterOperatorIsOneOf:
		values, err := extractFilterValues(f)
		if err != nil {
			return nil, err
		}
		escaped := make([]string, len(values))
		for i, v := range values {
			escaped[i] = regexp.QuoteMeta(v)
		}
		return &desugaredMatcher{
			operator: `=~`,
			value:    fmt.Sprintf(`"^(%s)$"`, strings.Join(escaped, "|")),
		}, nil

	case dash0api.AttributeFilterOperatorIsNotOneOf:
		values, err := extractFilterValues(f)
		if err != nil {
			return nil, err
		}
		escaped := make([]string, len(values))
		for i, v := range values {
			escaped[i] = regexp.QuoteMeta(v)
		}
		return &desugaredMatcher{
			operator: `!~`,
			value:    fmt.Sprintf(`"^(%s)$"`, strings.Join(escaped, "|")),
		}, nil
	}

	return nil, nil
}

// extractFilterValue extracts the string value from a single-value filter.
func extractFilterValue(f dash0api.AttributeFilter) (string, error) {
	if f.Value == nil {
		return "", fmt.Errorf("filter for key %q with operator %q requires a value", f.Key, f.Operator)
	}
	v, err := f.Value.AsAttributeFilterStringValue()
	if err != nil {
		return "", fmt.Errorf("failed to extract value for filter key %q: %w", f.Key, err)
	}
	return v, nil
}

// extractFilterValues extracts the string values from a multi-value filter.
func extractFilterValues(f dash0api.AttributeFilter) ([]string, error) {
	if f.Values == nil || len(*f.Values) == 0 {
		return nil, fmt.Errorf("filter for key %q with operator %q requires values", f.Key, f.Operator)
	}
	result := make([]string, len(*f.Values))
	for i, item := range *f.Values {
		v, err := item.AsAttributeFilterStringValue()
		if err != nil {
			return nil, fmt.Errorf("failed to extract value at index %d for filter key %q: %w", i, f.Key, err)
		}
		result[i] = v
	}
	return result, nil
}

// normalizeKey converts OTel attribute keys (dots) to Prometheus label names (underscores).
func normalizeKey(key string) string {
	return strings.ReplaceAll(key, ".", "_")
}

// escapePromQLString escapes backslashes and double quotes for use in a PromQL string literal.
func escapePromQLString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
