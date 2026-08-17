package asset

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpamFilterUsesOrigin_WithOrigin(t *testing.T) {
	doc := []byte(`apiVersion: v1alpha2
kind: Dash0SpamFilter
metadata:
  name: Drop debug logs
  labels:
    dash0.com/id: spam-id
    dash0.com/origin: spam-origin
spec:
  context: log
  filter:
    - key: otel.log.severity.range
      operator: is
      value: DEBUG
`)
	usesOrigin, err := SpamFilterUsesOrigin(doc)
	require.NoError(t, err)
	assert.True(t, usesOrigin)
}

func TestSpamFilterUsesOrigin_IDOnly(t *testing.T) {
	doc := []byte(`apiVersion: v1alpha1
kind: Dash0SpamFilter
metadata:
  name: Drop noisy health checks
  labels:
    dash0.com/id: spam-id
spec:
  contexts:
    - log
  filter:
    - key: http.target
      operator: ends_with
      value: /healthz
`)
	usesOrigin, err := SpamFilterUsesOrigin(doc)
	require.NoError(t, err)
	assert.False(t, usesOrigin)
}

func TestSpamFilterUsesOrigin_EmptyOriginLabel(t *testing.T) {
	doc := []byte(`kind: Dash0SpamFilter
metadata:
  name: x
  labels:
    dash0.com/id: spam-id
    dash0.com/origin: ""
spec:
  contexts: [log]
`)
	usesOrigin, err := SpamFilterUsesOrigin(doc)
	require.NoError(t, err)
	assert.False(t, usesOrigin, "an empty-string origin label must not count as using origin")
}
