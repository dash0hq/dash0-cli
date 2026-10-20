package asset

import (
	"fmt"
	"time"
)

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
	Before any // asset state before update or restore (nil for creates)
	After  any // asset state after update/create
}

// actionForExisting returns the action for a write to an asset that already
// exists. The API keeps deleted assets and brings them back on PUT, so a write
// to a deleted asset is a restore.
func actionForExisting(deletedAt *time.Time) ImportAction {
	if deletedAt != nil {
		return ActionRestored
	}
	return ActionUpdated
}

// RequireNotDeleted returns an error when the API response for a create or
// update still marks the asset as deleted.
func RequireNotDeleted(kind, name string, deletedAt *time.Time) error {
	if deletedAt == nil {
		return nil
	}
	return fmt.Errorf("%s %q is still deleted after the update", kind, name)
}
