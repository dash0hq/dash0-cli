package otlp

import (
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/stretchr/testify/assert"
)

func TestFindAttribute(t *testing.T) {
	strVal := "hello"
	intVal := "42"
	doubleVal := 3.14
	boolVal := true

	attrs := []dash0api.KeyValue{
		{Key: "str.key", Value: dash0api.AnyValue{StringValue: &strVal}},
		{Key: "int.key", Value: dash0api.AnyValue{IntValue: &intVal}},
		{Key: "double.key", Value: dash0api.AnyValue{DoubleValue: &doubleVal}},
		{Key: "bool.key", Value: dash0api.AnyValue{BoolValue: &boolVal}},
		{Key: "empty.key", Value: dash0api.AnyValue{}},
	}

	tests := []struct {
		name string
		key  string
		want string
	}{
		{"string value", "str.key", "hello"},
		{"int value", "int.key", "42"},
		{"double value", "double.key", "3.14"},
		{"bool value", "bool.key", "true"},
		{"empty value", "empty.key", ""},
		{"missing key", "missing.key", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, FindAttribute(attrs, tt.key))
		})
	}
}

func TestFindAttributeNilSlice(t *testing.T) {
	assert.Equal(t, "", FindAttribute(nil, "any.key"))
}

func TestMergeAttributes(t *testing.T) {
	val1 := "first"
	val2 := "second"
	val3 := "third"

	a := []dash0api.KeyValue{
		{Key: "key.a", Value: dash0api.AnyValue{StringValue: &val1}},
		{Key: "key.b", Value: dash0api.AnyValue{StringValue: &val1}},
	}
	b := []dash0api.KeyValue{
		{Key: "key.b", Value: dash0api.AnyValue{StringValue: &val2}},
		{Key: "key.c", Value: dash0api.AnyValue{StringValue: &val3}},
	}

	merged := MergeAttributes(a, b)
	assert.Len(t, merged, 3)
	assert.Equal(t, "first", FindAttribute(merged, "key.a"))
	assert.Equal(t, "second", FindAttribute(merged, "key.b"))
	assert.Equal(t, "third", FindAttribute(merged, "key.c"))
}

func TestMergeAttributesEmpty(t *testing.T) {
	merged := MergeAttributes(nil, nil)
	assert.Empty(t, merged)
}
