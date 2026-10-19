package asset

import "time"

// ImportAction indicates whether an asset was created, updated, or restored.
type ImportAction string

const (
	ActionCreated  ImportAction = "created"
	ActionUpdated  ImportAction = "updated"
	ActionRestored ImportAction = "restored"
)

// ImportResult holds the outcome of importing an asset.
type ImportResult struct {
	Name   string
	ID     string
	Action ImportAction
	Before any // asset state before update (nil for creates and restores)
	After  any // asset state after update/create
}

// actionForExisting returns the action and diff base for a write to an asset
// that already exists. The API keeps deleted assets and brings them back on
// PUT, so a deleted asset is restored, and has no diff base.
func actionForExisting(existing any, deletedAt *time.Time) (ImportAction, any) {
	if deletedAt != nil {
		return ActionRestored, nil
	}
	return ActionUpdated, existing
}
