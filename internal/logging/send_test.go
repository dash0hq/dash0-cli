package logging

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTraceAndSpanIDMustBeSpecifiedTogether(t *testing.T) {
	t.Run("only trace-id", func(t *testing.T) {
		cmd := newSendCmd()
		cmd.SetArgs([]string{"test body", "--trace-id", "0af7651916cd43dd8448eb211c80319c"})
		err := cmd.Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "both --trace-id and --span-id must be specified together")
	})

	t.Run("only span-id", func(t *testing.T) {
		cmd := newSendCmd()
		cmd.SetArgs([]string{"test body", "--span-id", "00f067aa0ba902b7"})
		err := cmd.Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "both --trace-id and --span-id must be specified together")
	})
}

func TestTimestampParsing(t *testing.T) {
	t.Run("without fractional seconds", func(t *testing.T) {
		cmd := newSendCmd()
		cmd.SetArgs([]string{"test", "--time", "2024-03-15T10:30:00Z"})
		// Will fail on missing OTLP client, but timestamp parsing happens first
		err := cmd.Execute()
		assert.NotContains(t, err.Error(), "invalid time format")
	})

	t.Run("with nanosecond precision", func(t *testing.T) {
		cmd := newSendCmd()
		cmd.SetArgs([]string{"test", "--time", "2024-03-15T10:30:00.123456789Z"})
		err := cmd.Execute()
		assert.NotContains(t, err.Error(), "invalid time format")
	})

	t.Run("with millisecond precision", func(t *testing.T) {
		cmd := newSendCmd()
		cmd.SetArgs([]string{"test", "--time", "2024-03-15T10:30:00.123Z"})
		err := cmd.Execute()
		assert.NotContains(t, err.Error(), "invalid time format")
	})

	t.Run("with timezone offset and nanoseconds", func(t *testing.T) {
		cmd := newSendCmd()
		cmd.SetArgs([]string{"test", "--time", "2024-03-15T10:30:00.000000001+01:00"})
		err := cmd.Execute()
		assert.NotContains(t, err.Error(), "invalid time format")
	})

	t.Run("invalid timestamp", func(t *testing.T) {
		cmd := newSendCmd()
		cmd.SetArgs([]string{"test", "--time", "not-a-timestamp"})
		err := cmd.Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid time format")
	})

	t.Run("observed-time with nanoseconds", func(t *testing.T) {
		cmd := newSendCmd()
		cmd.SetArgs([]string{"test", "--observed-time", "2024-03-15T10:30:00.999999999Z"})
		err := cmd.Execute()
		assert.NotContains(t, err.Error(), "invalid observed-time format")
	})

	t.Run("invalid observed-time", func(t *testing.T) {
		cmd := newSendCmd()
		cmd.SetArgs([]string{"test", "--observed-time", "not-a-timestamp"})
		err := cmd.Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid observed-time format")
	})
}

func TestScopeAttributeValidation(t *testing.T) {
	t.Run("valid scope attributes pass validation", func(t *testing.T) {
		cmd := newSendCmd()
		cmd.SetArgs([]string{"test", "--scope-attribute", "lib.name=mylib", "--scope-attribute", "lib.version=1.0"})
		err := cmd.Execute()
		// Will fail on OTLP client, not on scope attribute parsing
		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "invalid scope attribute")
	})

	t.Run("missing equals sign", func(t *testing.T) {
		cmd := newSendCmd()
		cmd.SetArgs([]string{"test", "--scope-attribute", "noequalssign"})
		err := cmd.Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid scope attribute")
		assert.Contains(t, err.Error(), "expected key=value format")
	})

	t.Run("empty key", func(t *testing.T) {
		cmd := newSendCmd()
		cmd.SetArgs([]string{"test", "--scope-attribute", "=value"})
		err := cmd.Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid scope attribute")
		assert.Contains(t, err.Error(), "empty key")
	})
}

func TestDroppedAttributesCount(t *testing.T) {
	t.Run("resource-dropped-attributes-count is accepted", func(t *testing.T) {
		cmd := newSendCmd()
		cmd.SetArgs([]string{"test", "--resource-dropped-attributes-count", "5"})
		err := cmd.Execute()
		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "dropped")
	})

	t.Run("scope-dropped-attributes-count is accepted", func(t *testing.T) {
		cmd := newSendCmd()
		cmd.SetArgs([]string{"test", "--scope-dropped-attributes-count", "3"})
		err := cmd.Execute()
		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "dropped")
	})

	t.Run("log-dropped-attributes-count is accepted", func(t *testing.T) {
		cmd := newSendCmd()
		cmd.SetArgs([]string{"test", "--log-dropped-attributes-count", "7"})
		err := cmd.Execute()
		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "dropped")
	})

	t.Run("all three dropped-attributes-count flags together", func(t *testing.T) {
		cmd := newSendCmd()
		cmd.SetArgs([]string{"test",
			"--resource-dropped-attributes-count", "1",
			"--scope-dropped-attributes-count", "2",
			"--log-dropped-attributes-count", "3",
		})
		err := cmd.Execute()
		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "dropped")
	})

	t.Run("negative value is rejected", func(t *testing.T) {
		cmd := newSendCmd()
		cmd.SetArgs([]string{"test", "--resource-dropped-attributes-count", "-1"})
		err := cmd.Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid argument")
	})

	t.Run("non-numeric value is rejected", func(t *testing.T) {
		cmd := newSendCmd()
		cmd.SetArgs([]string{"test", "--log-dropped-attributes-count", "abc"})
		err := cmd.Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid argument")
	})
}
