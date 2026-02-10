package logs

import (
	"testing"

	"github.com/dash0hq/dash0-cli/internal/version"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseKeyValuePairs(t *testing.T) {
	t.Run("valid pairs", func(t *testing.T) {
		result, err := parseKeyValuePairs([]string{"key1=value1", "key2=value2"})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"key1": "value1", "key2": "value2"}, result)
	})

	t.Run("empty value", func(t *testing.T) {
		result, err := parseKeyValuePairs([]string{"key="})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"key": ""}, result)
	})

	t.Run("value with equals sign", func(t *testing.T) {
		result, err := parseKeyValuePairs([]string{"key=val=ue"})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"key": "val=ue"}, result)
	})

	t.Run("nil input", func(t *testing.T) {
		result, err := parseKeyValuePairs(nil)
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("missing equals", func(t *testing.T) {
		_, err := parseKeyValuePairs([]string{"noequalssign"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "expected key=value format")
	})

	t.Run("empty key", func(t *testing.T) {
		_, err := parseKeyValuePairs([]string{"=value"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty key")
	})
}

func TestParseTraceID(t *testing.T) {
	t.Run("valid trace ID", func(t *testing.T) {
		tid, err := parseTraceID("0af7651916cd43dd8448eb211c80319c")
		require.NoError(t, err)
		assert.Equal(t, [16]byte{0x0a, 0xf7, 0x65, 0x19, 0x16, 0xcd, 0x43, 0xdd, 0x84, 0x48, 0xeb, 0x21, 0x1c, 0x80, 0x31, 0x9c}, [16]byte(tid))
	})

	t.Run("too short", func(t *testing.T) {
		_, err := parseTraceID("0af765")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "32 hex characters")
	})

	t.Run("too long", func(t *testing.T) {
		_, err := parseTraceID("0af7651916cd43dd8448eb211c80319c00")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "32 hex characters")
	})

	t.Run("invalid hex", func(t *testing.T) {
		_, err := parseTraceID("0af7651916cd43dd8448eb211c80319z")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "valid hex")
	})
}

func TestTraceAndSpanIDMustBeSpecifiedTogether(t *testing.T) {
	t.Run("only trace-id", func(t *testing.T) {
		cmd := newCreateCmd()
		cmd.SetArgs([]string{"test body", "--trace-id", "0af7651916cd43dd8448eb211c80319c"})
		err := cmd.Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "both --trace-id and --span-id must be specified together")
	})

	t.Run("only span-id", func(t *testing.T) {
		cmd := newCreateCmd()
		cmd.SetArgs([]string{"test body", "--span-id", "00f067aa0ba902b7"})
		err := cmd.Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "both --trace-id and --span-id must be specified together")
	})
}

func TestTimestampParsing(t *testing.T) {
	t.Run("without fractional seconds", func(t *testing.T) {
		cmd := newCreateCmd()
		cmd.SetArgs([]string{"test", "--time", "2024-03-15T10:30:00Z"})
		// Will fail on missing OTLP client, but timestamp parsing happens first
		err := cmd.Execute()
		assert.NotContains(t, err.Error(), "invalid time format")
	})

	t.Run("with nanosecond precision", func(t *testing.T) {
		cmd := newCreateCmd()
		cmd.SetArgs([]string{"test", "--time", "2024-03-15T10:30:00.123456789Z"})
		err := cmd.Execute()
		assert.NotContains(t, err.Error(), "invalid time format")
	})

	t.Run("with millisecond precision", func(t *testing.T) {
		cmd := newCreateCmd()
		cmd.SetArgs([]string{"test", "--time", "2024-03-15T10:30:00.123Z"})
		err := cmd.Execute()
		assert.NotContains(t, err.Error(), "invalid time format")
	})

	t.Run("with timezone offset and nanoseconds", func(t *testing.T) {
		cmd := newCreateCmd()
		cmd.SetArgs([]string{"test", "--time", "2024-03-15T10:30:00.000000001+01:00"})
		err := cmd.Execute()
		assert.NotContains(t, err.Error(), "invalid time format")
	})

	t.Run("invalid timestamp", func(t *testing.T) {
		cmd := newCreateCmd()
		cmd.SetArgs([]string{"test", "--time", "not-a-timestamp"})
		err := cmd.Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid time format")
	})

	t.Run("observed-time with nanoseconds", func(t *testing.T) {
		cmd := newCreateCmd()
		cmd.SetArgs([]string{"test", "--observed-time", "2024-03-15T10:30:00.999999999Z"})
		err := cmd.Execute()
		assert.NotContains(t, err.Error(), "invalid observed-time format")
	})

	t.Run("invalid observed-time", func(t *testing.T) {
		cmd := newCreateCmd()
		cmd.SetArgs([]string{"test", "--observed-time", "not-a-timestamp"})
		err := cmd.Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid observed-time format")
	})
}

func TestScopeDefaults(t *testing.T) {
	setupScopeCmd := func(args ...string) (*cobra.Command, *createFlags) {
		flags := &createFlags{}
		cmd := &cobra.Command{Use: "test", RunE: func(cmd *cobra.Command, args []string) error { return nil }}
		cmd.Flags().StringVar(&flags.ScopeName, "scope-name", "dash0-cli", "")
		cmd.Flags().StringVar(&flags.ScopeVersion, "scope-version", version.Version, "")
		cmd.SetArgs(args)
		err := cmd.Execute()
		require.NoError(t, err)
		resolveScopeDefaults(cmd, flags)
		return cmd, flags
	}

	t.Run("neither flag set uses dash0-cli defaults", func(t *testing.T) {
		_, flags := setupScopeCmd()
		assert.Equal(t, "dash0-cli", flags.ScopeName)
		assert.Equal(t, version.Version, flags.ScopeVersion)
	})

	t.Run("only scope-name set clears scope-version default", func(t *testing.T) {
		_, flags := setupScopeCmd("--scope-name", "my-app")
		assert.Equal(t, "my-app", flags.ScopeName)
		assert.Equal(t, "", flags.ScopeVersion)
	})

	t.Run("only scope-version set clears scope-name default", func(t *testing.T) {
		_, flags := setupScopeCmd("--scope-version", "2.0.0")
		assert.Equal(t, "", flags.ScopeName)
		assert.Equal(t, "2.0.0", flags.ScopeVersion)
	})

	t.Run("both set uses provided values", func(t *testing.T) {
		_, flags := setupScopeCmd("--scope-name", "my-app", "--scope-version", "2.0.0")
		assert.Equal(t, "my-app", flags.ScopeName)
		assert.Equal(t, "2.0.0", flags.ScopeVersion)
	})
}

func TestScopeAttributeValidation(t *testing.T) {
	t.Run("valid scope attributes pass validation", func(t *testing.T) {
		cmd := newCreateCmd()
		cmd.SetArgs([]string{"test", "--scope-attribute", "lib.name=mylib", "--scope-attribute", "lib.version=1.0"})
		err := cmd.Execute()
		// Will fail on OTLP client, not on scope attribute parsing
		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "invalid scope attribute")
	})

	t.Run("missing equals sign", func(t *testing.T) {
		cmd := newCreateCmd()
		cmd.SetArgs([]string{"test", "--scope-attribute", "noequalssign"})
		err := cmd.Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid scope attribute")
		assert.Contains(t, err.Error(), "expected key=value format")
	})

	t.Run("empty key", func(t *testing.T) {
		cmd := newCreateCmd()
		cmd.SetArgs([]string{"test", "--scope-attribute", "=value"})
		err := cmd.Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid scope attribute")
		assert.Contains(t, err.Error(), "empty key")
	})
}

func TestDroppedAttributesCount(t *testing.T) {
	t.Run("resource-dropped-attributes-count is accepted", func(t *testing.T) {
		cmd := newCreateCmd()
		cmd.SetArgs([]string{"test", "--resource-dropped-attributes-count", "5"})
		err := cmd.Execute()
		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "dropped")
	})

	t.Run("scope-dropped-attributes-count is accepted", func(t *testing.T) {
		cmd := newCreateCmd()
		cmd.SetArgs([]string{"test", "--scope-dropped-attributes-count", "3"})
		err := cmd.Execute()
		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "dropped")
	})

	t.Run("log-dropped-attributes-count is accepted", func(t *testing.T) {
		cmd := newCreateCmd()
		cmd.SetArgs([]string{"test", "--log-dropped-attributes-count", "7"})
		err := cmd.Execute()
		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "dropped")
	})

	t.Run("all three dropped-attributes-count flags together", func(t *testing.T) {
		cmd := newCreateCmd()
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
		cmd := newCreateCmd()
		cmd.SetArgs([]string{"test", "--resource-dropped-attributes-count", "-1"})
		err := cmd.Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid argument")
	})

	t.Run("non-numeric value is rejected", func(t *testing.T) {
		cmd := newCreateCmd()
		cmd.SetArgs([]string{"test", "--log-dropped-attributes-count", "abc"})
		err := cmd.Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid argument")
	})
}

func TestParseSpanID(t *testing.T) {
	t.Run("valid span ID", func(t *testing.T) {
		sid, err := parseSpanID("00f067aa0ba902b7")
		require.NoError(t, err)
		assert.Equal(t, [8]byte{0x00, 0xf0, 0x67, 0xaa, 0x0b, 0xa9, 0x02, 0xb7}, [8]byte(sid))
	})

	t.Run("too short", func(t *testing.T) {
		_, err := parseSpanID("00f067")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "16 hex characters")
	})

	t.Run("too long", func(t *testing.T) {
		_, err := parseSpanID("00f067aa0ba902b700")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "16 hex characters")
	})

	t.Run("invalid hex", func(t *testing.T) {
		_, err := parseSpanID("00f067aa0ba902bz")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "valid hex")
	})
}
