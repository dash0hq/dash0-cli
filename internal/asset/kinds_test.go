package asset

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A kind that apply accepts in validation but lacks a behavior would fail
// only when a user runs the command that needs it.
func TestKinds_EveryValidKindHasEveryBehavior(t *testing.T) {
	for _, kind := range ValidKinds {
		behavior, ok := kinds[kind]
		require.Truef(t, ok, "kind %q is in ValidKinds but not in kinds", kind)
		assert.NotEmptyf(t, behavior.displayName, "kind %q has no display name", kind)
		assert.NotNilf(t, behavior.decodeHeader, "kind %q has no header decoder", kind)
		assert.NotNilf(t, behavior.apply, "kind %q has no apply", kind)
	}
}

// The document does not parse, so no kind reaches the API.
func TestApplyDocument_AppliesEveryValidKind(t *testing.T) {
	for _, kind := range ValidKinds {
		t.Run(kind, func(t *testing.T) {
			_, err := ApplyDocument(context.Background(), nil, Document{Kind: kind, Raw: []byte("{")}, nil)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "unsupported kind")
		})
	}
}
