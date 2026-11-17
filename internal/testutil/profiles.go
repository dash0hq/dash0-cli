package testutil

import (
	"context"
	"testing"

	"github.com/dash0hq/dash0-api-client-go/profiles"
	"github.com/stretchr/testify/require"
)

// ResolvedConfiguration stores configuration as a profile in a temporary
// directory and reads it back: only a profiles.Store can build a
// profiles.ResolvedConfiguration. Environment variables still apply on top.
func ResolvedConfiguration(t *testing.T, profileName string, configuration profiles.Configuration) *profiles.ResolvedConfiguration {
	t.Helper()
	store, err := profiles.NewStore(profiles.WithConfigDir(t.TempDir()))
	require.NoError(t, err)
	require.NoError(t, store.AddProfile(profiles.Profile{Name: profileName, Configuration: configuration}))
	resolved, err := store.GetConfigurationForProfile(context.Background(), profileName)
	require.NoError(t, err)
	return resolved
}
