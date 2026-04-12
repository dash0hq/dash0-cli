package agent0

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDiffSnapshotsFirstSnapshot(t *testing.T) {
	curr := &InvokeResponse{
		Messages: []Message{
			{Role: RoleHuman, Hash: "h1", Content: "Hello"},
			{Role: RoleAssistant, Hash: "a1", Content: "Hi there"},
		},
	}

	deltas := DiffSnapshots(nil, curr)

	assert.Len(t, deltas, 2)
	assert.True(t, deltas[0].IsNew)
	assert.Equal(t, RoleHuman, deltas[0].Role)
	assert.Equal(t, "Hello", deltas[0].Content)
	assert.True(t, deltas[1].IsNew)
	assert.Equal(t, RoleAssistant, deltas[1].Role)
}

func TestDiffSnapshotsNewMessage(t *testing.T) {
	prev := &InvokeResponse{
		Messages: []Message{
			{Role: RoleHuman, Hash: "h1", Content: "Hello"},
		},
	}
	curr := &InvokeResponse{
		Messages: []Message{
			{Role: RoleHuman, Hash: "h1", Content: "Hello"},
			{Role: RoleAssistant, Hash: "a1", Content: "Hi there"},
		},
	}

	deltas := DiffSnapshots(prev, curr)

	assert.Len(t, deltas, 1)
	assert.True(t, deltas[0].IsNew)
	assert.Equal(t, RoleAssistant, deltas[0].Role)
	assert.Equal(t, "Hi there", deltas[0].Content)
}

func TestDiffSnapshotsUpdatedMessage(t *testing.T) {
	prev := &InvokeResponse{
		Messages: []Message{
			{Role: RoleHuman, Hash: "h1", Content: "Hello"},
			{Role: RoleAssistant, Hash: "a1", Content: "Hi"},
		},
	}
	curr := &InvokeResponse{
		Messages: []Message{
			{Role: RoleHuman, Hash: "h1", Content: "Hello"},
			{Role: RoleAssistant, Hash: "a1", Content: "Hi there, how can I help?"},
		},
	}

	deltas := DiffSnapshots(prev, curr)

	assert.Len(t, deltas, 1)
	assert.False(t, deltas[0].IsNew)
	assert.Equal(t, RoleAssistant, deltas[0].Role)
	assert.Equal(t, "Hi there, how can I help?", deltas[0].Content)
}

func TestDiffSnapshotsNoChanges(t *testing.T) {
	snapshot := &InvokeResponse{
		Messages: []Message{
			{Role: RoleHuman, Hash: "h1", Content: "Hello"},
			{Role: RoleAssistant, Hash: "a1", Content: "Hi there"},
		},
	}

	deltas := DiffSnapshots(snapshot, snapshot)

	assert.Empty(t, deltas)
}

func TestDiffSnapshotsNilCurrent(t *testing.T) {
	prev := &InvokeResponse{
		Messages: []Message{
			{Role: RoleHuman, Hash: "h1", Content: "Hello"},
		},
	}

	deltas := DiffSnapshots(prev, nil)

	assert.Nil(t, deltas)
}

func TestDiffSnapshotsMessageWithoutHash(t *testing.T) {
	prev := &InvokeResponse{
		Messages: []Message{
			{Role: RoleHuman, Hash: "h1", Content: "Hello"},
		},
	}
	curr := &InvokeResponse{
		Messages: []Message{
			{Role: RoleHuman, Hash: "h1", Content: "Hello"},
			{Role: RoleTool, Hash: "", Content: "tool output"},
		},
	}

	deltas := DiffSnapshots(prev, curr)

	assert.Len(t, deltas, 1)
	assert.True(t, deltas[0].IsNew)
	assert.Equal(t, RoleTool, deltas[0].Role)
}
