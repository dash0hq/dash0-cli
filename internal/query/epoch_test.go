package query

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveToEpochSeconds(t *testing.T) {
	now := time.Now().Unix()

	t.Run("now", func(t *testing.T) {
		result, err := ResolveToEpochSeconds("now")
		require.NoError(t, err)
		epoch, err := strconv.ParseInt(result, 10, 64)
		require.NoError(t, err)
		assert.InDelta(t, now, epoch, 2)
	})

	t.Run("now-1h", func(t *testing.T) {
		result, err := ResolveToEpochSeconds("now-1h")
		require.NoError(t, err)
		epoch, err := strconv.ParseInt(result, 10, 64)
		require.NoError(t, err)
		assert.InDelta(t, now-3600, epoch, 2)
	})

	t.Run("now-30m", func(t *testing.T) {
		result, err := ResolveToEpochSeconds("now-30m")
		require.NoError(t, err)
		epoch, err := strconv.ParseInt(result, 10, 64)
		require.NoError(t, err)
		assert.InDelta(t, now-1800, epoch, 2)
	})

	t.Run("now-7d", func(t *testing.T) {
		result, err := ResolveToEpochSeconds("now-7d")
		require.NoError(t, err)
		epoch, err := strconv.ParseInt(result, 10, 64)
		require.NoError(t, err)
		assert.InDelta(t, now-7*86400, epoch, 2)
	})

	t.Run("absolute timestamp with timezone", func(t *testing.T) {
		result, err := ResolveToEpochSeconds("2024-01-25T10:00:00Z")
		require.NoError(t, err)
		expected := time.Date(2024, 1, 25, 10, 0, 0, 0, time.UTC).Unix()
		assert.Equal(t, strconv.FormatInt(expected, 10), result)
	})

	t.Run("absolute timestamp with millis", func(t *testing.T) {
		result, err := ResolveToEpochSeconds("2024-01-25T10:00:00.000Z")
		require.NoError(t, err)
		expected := time.Date(2024, 1, 25, 10, 0, 0, 0, time.UTC).Unix()
		assert.Equal(t, strconv.FormatInt(expected, 10), result)
	})

	t.Run("absolute date only", func(t *testing.T) {
		result, err := ResolveToEpochSeconds("2024-01-25")
		require.NoError(t, err)
		expected := time.Date(2024, 1, 25, 0, 0, 0, 0, time.UTC).Unix()
		assert.Equal(t, strconv.FormatInt(expected, 10), result)
	})

	t.Run("absolute timestamp with offset", func(t *testing.T) {
		result, err := ResolveToEpochSeconds("2024-01-25T12:00:00+02:00")
		require.NoError(t, err)
		expected := time.Date(2024, 1, 25, 10, 0, 0, 0, time.UTC).Unix()
		assert.Equal(t, strconv.FormatInt(expected, 10), result)
	})

	t.Run("invalid expression", func(t *testing.T) {
		_, err := ResolveToEpochSeconds("garbage")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot parse time expression")
	})

	t.Run("invalid relative duration", func(t *testing.T) {
		_, err := ResolveToEpochSeconds("now-abc")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid relative time")
	})

	t.Run("invalid day count", func(t *testing.T) {
		_, err := ResolveToEpochSeconds("now-abcd")
		assert.Error(t, err)
	})
}
