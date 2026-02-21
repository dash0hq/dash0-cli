package otlp

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/dash0hq/dash0-cli/internal/version"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// DefaultScopeName is the default instrumentation scope name used by the CLI.
const DefaultScopeName = "dash0-cli"

// ParseKeyValuePairs parses a slice of "key=value" strings into a map.
func ParseKeyValuePairs(pairs []string) (map[string]string, error) {
	result := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		k, v, ok := strings.Cut(pair, "=")
		if !ok {
			return nil, fmt.Errorf("expected key=value format, got %q", pair)
		}
		if k == "" {
			return nil, fmt.Errorf("empty key in %q", pair)
		}
		result[k] = v
	}
	return result, nil
}

// ParseTraceID parses a 32 hex character string into a pcommon.TraceID.
func ParseTraceID(s string) (pcommon.TraceID, error) {
	if len(s) != 32 {
		return pcommon.TraceID{}, fmt.Errorf("trace-id must be 32 hex characters, got %d", len(s))
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return pcommon.TraceID{}, fmt.Errorf("trace-id must be valid hex: %w", err)
	}
	var tid pcommon.TraceID
	copy(tid[:], b)
	return tid, nil
}

// ParseSpanID parses a 16 hex character string into a pcommon.SpanID.
func ParseSpanID(s string) (pcommon.SpanID, error) {
	if len(s) != 16 {
		return pcommon.SpanID{}, fmt.Errorf("span-id must be 16 hex characters, got %d", len(s))
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return pcommon.SpanID{}, fmt.Errorf("span-id must be valid hex: %w", err)
	}
	var sid pcommon.SpanID
	copy(sid[:], b)
	return sid, nil
}

// ResolveScopeDefaults clears the default value for scope-name or scope-version
// when only the other flag is explicitly set. This avoids pairing a custom scope
// name with the dash0-cli version (or vice versa).
func ResolveScopeDefaults(cmd *cobra.Command, scopeName *string, scopeVersion *string) {
	scopeNameChanged := cmd.Flags().Changed("scope-name")
	scopeVersionChanged := cmd.Flags().Changed("scope-version")
	if scopeNameChanged && !scopeVersionChanged {
		*scopeVersion = ""
	} else if scopeVersionChanged && !scopeNameChanged {
		*scopeName = ""
	}
}

// DefaultScopeVersion returns the default scope version (the CLI version).
func DefaultScopeVersion() string {
	return version.Version
}
