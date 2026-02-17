package query

import (
	"fmt"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// knownOperators is the set of recognized filter operators, including symbolic aliases.
var knownOperators = map[string]dash0api.AttributeFilterOperator{
	// Canonical names
	"is":                  dash0api.AttributeFilterOperatorIs,
	"is_not":              dash0api.AttributeFilterOperatorIsNot,
	"contains":            dash0api.AttributeFilterOperatorContains,
	"does_not_contain":    dash0api.AttributeFilterOperatorDoesNotContain,
	"starts_with":         dash0api.AttributeFilterOperatorStartsWith,
	"does_not_start_with": dash0api.AttributeFilterOperatorDoesNotStartWith,
	"ends_with":           dash0api.AttributeFilterOperatorEndsWith,
	"does_not_end_with":   dash0api.AttributeFilterOperatorDoesNotEndWith,
	"matches":             dash0api.AttributeFilterOperatorMatches,
	"does_not_match":      dash0api.AttributeFilterOperatorDoesNotMatch,
	"gt":                  dash0api.AttributeFilterOperatorGt,
	"gte":                 dash0api.AttributeFilterOperatorGte,
	"lt":                  dash0api.AttributeFilterOperatorLt,
	"lte":                 dash0api.AttributeFilterOperatorLte,
	"is_set":              dash0api.AttributeFilterOperatorIsSet,
	"is_not_set":          dash0api.AttributeFilterOperatorIsNotSet,
	"is_one_of":           dash0api.AttributeFilterOperatorIsOneOf,
	"is_not_one_of":       dash0api.AttributeFilterOperatorIsNotOneOf,
	"is_any":              dash0api.AttributeFilterOperatorIsAny,

	// Symbolic aliases
	"=":  dash0api.AttributeFilterOperatorIs,
	"!=": dash0api.AttributeFilterOperatorIsNot,
	">":  dash0api.AttributeFilterOperatorGt,
	">=": dash0api.AttributeFilterOperatorGte,
	"<":  dash0api.AttributeFilterOperatorLt,
	"<=": dash0api.AttributeFilterOperatorLte,
	"~":  dash0api.AttributeFilterOperatorMatches,
	"!~": dash0api.AttributeFilterOperatorDoesNotMatch,
}

// noValueOperators are operators that take no value.
var noValueOperators = map[dash0api.AttributeFilterOperator]bool{
	dash0api.AttributeFilterOperatorIsSet:    true,
	dash0api.AttributeFilterOperatorIsNotSet: true,
}

// multiValueOperators are operators that take space-separated values.
// Values containing spaces can be single-quoted, e.g., 'my value'.
var multiValueOperators = map[dash0api.AttributeFilterOperator]bool{
	dash0api.AttributeFilterOperatorIsOneOf:    true,
	dash0api.AttributeFilterOperatorIsNotOneOf: true,
}

// ParseFilters parses a list of filter strings into FilterCriteria.
func ParseFilters(filterStrings []string) (*dash0api.FilterCriteria, error) {
	if len(filterStrings) == 0 {
		return nil, nil
	}

	filters := make(dash0api.FilterCriteria, 0, len(filterStrings))
	for _, f := range filterStrings {
		filter, err := ParseFilter(f)
		if err != nil {
			return nil, fmt.Errorf("invalid filter %q: %w", f, err)
		}
		filters = append(filters, filter)
	}
	return &filters, nil
}

// ParseFilter parses a single filter string into an AttributeFilter.
//
// Format: key [operator] value
//   - If key starts with single quote, consume until closing quote
//   - Next token is checked against known operators
//   - If it matches: use that operator, rest is value
//   - If no match: implicit "is" operator, rest (from that token) is value
//   - is_set/is_not_set expect no value
//   - is_one_of/is_not_one_of split value on whitespace (single-quoted tokens may contain spaces)
func ParseFilter(s string) (dash0api.AttributeFilter, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return dash0api.AttributeFilter{}, fmt.Errorf("empty filter expression")
	}

	key, rest, err := parseFilterKey(s)
	if err != nil {
		return dash0api.AttributeFilter{}, err
	}

	if rest == "" {
		return dash0api.AttributeFilter{}, fmt.Errorf("filter requires at least a key and value (or a key and operator like is_set)")
	}

	return parseFilterOperatorAndValue(key, rest)
}

// parseFilterKey extracts the key and remaining string from a filter expression.
// Keys may be single-quoted to allow spaces.
func parseFilterKey(s string) (key, rest string, err error) {
	if s[0] == '\'' {
		end := strings.Index(s[1:], "'")
		if end == -1 {
			return "", "", fmt.Errorf("unclosed single quote in key")
		}
		key = s[1 : end+1]
		rest = strings.TrimSpace(s[end+2:])
	} else {
		parts := strings.SplitN(s, " ", 2)
		key = parts[0]
		if len(parts) > 1 {
			rest = strings.TrimSpace(parts[1])
		}
	}
	if key == "" {
		return "", "", fmt.Errorf("empty filter key")
	}
	return key, rest, nil
}

// parseFilterOperatorAndValue parses the operator and value portion of a filter.
func parseFilterOperatorAndValue(key, rest string) (dash0api.AttributeFilter, error) {
	tokens := strings.SplitN(rest, " ", 2)
	firstToken := tokens[0]
	var valueStr string
	if len(tokens) > 1 {
		valueStr = strings.TrimSpace(tokens[1])
	}

	op, isKnown := knownOperators[firstToken]
	if !isKnown {
		// No known operator — implicit "is", entire rest is the value
		return buildFilter(key, dash0api.AttributeFilterOperatorIs, rest)
	}

	if noValueOperators[op] {
		if valueStr != "" {
			return dash0api.AttributeFilter{}, fmt.Errorf("operator %q does not accept a value", firstToken)
		}
		return dash0api.AttributeFilter{
			Key:      key,
			Operator: op,
		}, nil
	}

	if valueStr == "" {
		return dash0api.AttributeFilter{}, fmt.Errorf("operator %q requires a value", firstToken)
	}

	// Treat `= ""` as is_not_set and `!= ""` as is_set.
	if valueStr == `""` || valueStr == `''` {
		if op == dash0api.AttributeFilterOperatorIs {
			return dash0api.AttributeFilter{Key: key, Operator: dash0api.AttributeFilterOperatorIsNotSet}, nil
		}
		if op == dash0api.AttributeFilterOperatorIsNot {
			return dash0api.AttributeFilter{Key: key, Operator: dash0api.AttributeFilterOperatorIsSet}, nil
		}
	}

	return buildFilter(key, op, valueStr)
}

func buildFilter(key string, op dash0api.AttributeFilterOperator, valueStr string) (dash0api.AttributeFilter, error) {
	filter := dash0api.AttributeFilter{
		Key:      key,
		Operator: op,
	}

	if multiValueOperators[op] {
		items, err := buildMultiValues(valueStr)
		if err != nil {
			return dash0api.AttributeFilter{}, err
		}
		filter.Values = &items
	} else {
		var val dash0api.AttributeFilter_Value
		if err := val.FromAttributeFilterStringValue(valueStr); err != nil {
			return dash0api.AttributeFilter{}, fmt.Errorf("failed to build filter value: %w", err)
		}
		filter.Value = &val
	}

	return filter, nil
}

func buildMultiValues(valueStr string) ([]dash0api.AttributeFilter_Values_Item, error) {
	parts, err := splitQuotedTokens(valueStr)
	if err != nil {
		return nil, err
	}
	items := make([]dash0api.AttributeFilter_Values_Item, 0, len(parts))
	for _, p := range parts {
		var item dash0api.AttributeFilter_Values_Item
		if err := item.FromAttributeFilterStringValue(p); err != nil {
			return nil, fmt.Errorf("failed to build filter value: %w", err)
		}
		items = append(items, item)
	}
	return items, nil
}

// splitQuotedTokens splits a string on whitespace, but respects single-quoted
// segments. For example: "ERROR 'my value' WARN" → ["ERROR", "my value", "WARN"].
func splitQuotedTokens(s string) ([]string, error) {
	var tokens []string
	s = strings.TrimSpace(s)
	for len(s) > 0 {
		token, rest, err := nextQuotedToken(s)
		if err != nil {
			return nil, err
		}
		if token != "" {
			tokens = append(tokens, token)
		}
		s = rest
	}
	return tokens, nil
}

// nextQuotedToken extracts the next token from s, handling single-quoted values.
// Returns the token, the remaining string, and any error.
func nextQuotedToken(s string) (token, rest string, err error) {
	if s[0] == '\'' {
		end := strings.Index(s[1:], "'")
		if end == -1 {
			return "", "", fmt.Errorf("unclosed single quote in value")
		}
		return s[1 : end+1], strings.TrimSpace(s[end+2:]), nil
	}
	parts := strings.SplitN(s, " ", 2)
	if len(parts) > 1 {
		return parts[0], strings.TrimSpace(parts[1]), nil
	}
	return parts[0], "", nil
}
