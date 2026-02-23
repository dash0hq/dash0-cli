package otlp

import (
	"encoding/hex"
	"strconv"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// FindAttribute looks up key in attrs and returns its string representation.
// Returns "" if the key is not found or the value is empty.
func FindAttribute(attrs []dash0api.KeyValue, key string) string {
	for _, kv := range attrs {
		if kv.Key == key {
			return AnyValueToString(&kv.Value)
		}
	}
	return ""
}

// AnyValueToString converts an AnyValue to its string representation.
func AnyValueToString(v *dash0api.AnyValue) string {
	if v == nil {
		return ""
	}
	if v.StringValue != nil {
		return *v.StringValue
	}
	if v.IntValue != nil {
		return *v.IntValue
	}
	if v.DoubleValue != nil {
		return strconv.FormatFloat(*v.DoubleValue, 'f', -1, 64)
	}
	if v.BoolValue != nil {
		return strconv.FormatBool(*v.BoolValue)
	}
	return ""
}

// MergeAttributes merges multiple attribute slices into a single slice.
// Later slices take precedence over earlier ones for duplicate keys.
func MergeAttributes(attrSlices ...[]dash0api.KeyValue) []dash0api.KeyValue {
	seen := make(map[string]int)
	var result []dash0api.KeyValue
	for _, attrs := range attrSlices {
		for _, kv := range attrs {
			if idx, ok := seen[kv.Key]; ok {
				result[idx] = kv
			} else {
				seen[kv.Key] = len(result)
				result = append(result, kv)
			}
		}
	}
	return result
}

// DerefString returns the value of a *string, or "" if nil.
func DerefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// DerefHexBytes returns the hex encoding of a *[]byte, or "" if nil.
func DerefHexBytes(b *[]byte) string {
	if b == nil {
		return ""
	}
	return hex.EncodeToString(*b)
}

// DerefInt64 returns the string representation of a *int64, or "" if nil.
func DerefInt64(i *int64) string {
	if i == nil {
		return ""
	}
	return strconv.FormatInt(*i, 10)
}
