package otlp

import (
	"testing"

	"github.com/dash0hq/dash0-cli/internal/version"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseKeyValuePairs(t *testing.T) {
	t.Run("valid pairs", func(t *testing.T) {
		result, err := ParseKeyValuePairs([]string{"key1=value1", "key2=value2"})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"key1": "value1", "key2": "value2"}, result)
	})

	t.Run("empty value", func(t *testing.T) {
		result, err := ParseKeyValuePairs([]string{"key="})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"key": ""}, result)
	})

	t.Run("value with equals sign", func(t *testing.T) {
		result, err := ParseKeyValuePairs([]string{"key=val=ue"})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"key": "val=ue"}, result)
	})

	t.Run("nil input", func(t *testing.T) {
		result, err := ParseKeyValuePairs(nil)
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("missing equals", func(t *testing.T) {
		_, err := ParseKeyValuePairs([]string{"noequalssign"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "expected key=value format")
	})

	t.Run("empty key", func(t *testing.T) {
		_, err := ParseKeyValuePairs([]string{"=value"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty key")
	})
}

func TestParseTraceID(t *testing.T) {
	t.Run("valid trace ID", func(t *testing.T) {
		tid, err := ParseTraceID("0af7651916cd43dd8448eb211c80319c")
		require.NoError(t, err)
		assert.Equal(t, [16]byte{0x0a, 0xf7, 0x65, 0x19, 0x16, 0xcd, 0x43, 0xdd, 0x84, 0x48, 0xeb, 0x21, 0x1c, 0x80, 0x31, 0x9c}, [16]byte(tid))
	})

	t.Run("too short", func(t *testing.T) {
		_, err := ParseTraceID("0af765")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "32 hex characters")
	})

	t.Run("too long", func(t *testing.T) {
		_, err := ParseTraceID("0af7651916cd43dd8448eb211c80319c00")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "32 hex characters")
	})

	t.Run("invalid hex", func(t *testing.T) {
		_, err := ParseTraceID("0af7651916cd43dd8448eb211c80319z")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "valid hex")
	})
}

func TestParseSpanID(t *testing.T) {
	t.Run("valid span ID", func(t *testing.T) {
		sid, err := ParseSpanID("00f067aa0ba902b7")
		require.NoError(t, err)
		assert.Equal(t, [8]byte{0x00, 0xf0, 0x67, 0xaa, 0x0b, 0xa9, 0x02, 0xb7}, [8]byte(sid))
	})

	t.Run("too short", func(t *testing.T) {
		_, err := ParseSpanID("00f067")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "16 hex characters")
	})

	t.Run("too long", func(t *testing.T) {
		_, err := ParseSpanID("00f067aa0ba902b700")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "16 hex characters")
	})

	t.Run("invalid hex", func(t *testing.T) {
		_, err := ParseSpanID("00f067aa0ba902bz")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "valid hex")
	})
}

func TestResolveScopeDefaults(t *testing.T) {
	setupScopeCmd := func(args ...string) (*cobra.Command, string, string) {
		scopeName := DefaultScopeName
		scopeVersion := version.Version
		cmd := &cobra.Command{Use: "test", RunE: func(cmd *cobra.Command, args []string) error { return nil }}
		cmd.Flags().StringVar(&scopeName, "scope-name", DefaultScopeName, "")
		cmd.Flags().StringVar(&scopeVersion, "scope-version", version.Version, "")
		cmd.SetArgs(args)
		err := cmd.Execute()
		require.NoError(t, err)
		ResolveScopeDefaults(cmd, &scopeName, &scopeVersion)
		return cmd, scopeName, scopeVersion
	}

	t.Run("neither flag set uses dash0-cli defaults", func(t *testing.T) {
		_, name, ver := setupScopeCmd()
		assert.Equal(t, "dash0-cli", name)
		assert.Equal(t, version.Version, ver)
	})

	t.Run("only scope-name set clears scope-version default", func(t *testing.T) {
		_, name, ver := setupScopeCmd("--scope-name", "my-app")
		assert.Equal(t, "my-app", name)
		assert.Equal(t, "", ver)
	})

	t.Run("only scope-version set clears scope-name default", func(t *testing.T) {
		_, name, ver := setupScopeCmd("--scope-version", "2.0.0")
		assert.Equal(t, "", name)
		assert.Equal(t, "2.0.0", ver)
	})

	t.Run("both set uses provided values", func(t *testing.T) {
		_, name, ver := setupScopeCmd("--scope-name", "my-app", "--scope-version", "2.0.0")
		assert.Equal(t, "my-app", name)
		assert.Equal(t, "2.0.0", ver)
	})
}
