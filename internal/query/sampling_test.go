package query

import (
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePrecision(t *testing.T) {
	timeRange := dash0api.TimeReferenceRange{From: "now-5m", To: "now"}

	t.Run("empty value returns nil so the server applies its default", func(t *testing.T) {
		s, err := ParsePrecision("", timeRange)
		require.NoError(t, err)
		assert.Nil(t, s)
	})

	t.Run("whitespace-only value is treated as empty", func(t *testing.T) {
		s, err := ParsePrecision("   ", timeRange)
		require.NoError(t, err)
		assert.Nil(t, s)
	})

	t.Run("adaptive sets mode explicitly with the request time range", func(t *testing.T) {
		s, err := ParsePrecision("adaptive", timeRange)
		require.NoError(t, err)
		require.NotNil(t, s)
		assert.Equal(t, dash0api.Adaptive, s.Mode)
		assert.Equal(t, timeRange, s.TimeRange)
	})

	t.Run("disabled sets mode to disabled with the request time range", func(t *testing.T) {
		s, err := ParsePrecision("disabled", timeRange)
		require.NoError(t, err)
		require.NotNil(t, s)
		assert.Equal(t, dash0api.Disabled, s.Mode)
		assert.Equal(t, timeRange, s.TimeRange)
	})

	t.Run("mode value is case-insensitive", func(t *testing.T) {
		s, err := ParsePrecision("DISABLED", timeRange)
		require.NoError(t, err)
		require.NotNil(t, s)
		assert.Equal(t, dash0api.Disabled, s.Mode)
	})

	t.Run("invalid value returns an error", func(t *testing.T) {
		s, err := ParsePrecision("precise", timeRange)
		require.Error(t, err)
		assert.Nil(t, s)
		assert.Contains(t, err.Error(), `invalid --precision value "precise"`)
	})
}
