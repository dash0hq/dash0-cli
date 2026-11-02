package asset

import (
	"fmt"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
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

// ImportPlan is what an import would do, decided from the stored asset before
// any write. apply writes it; diff only prints it.
type ImportPlan struct {
	// Key is the id or origin the write targets. An empty Key means a create
	// with a server-assigned id.
	Key    string
	Action ImportAction
	// Before is the stored asset, or nil when nothing is stored yet.
	Before any
	// Proposed is the asset as it would be stored after the write.
	Proposed any
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

// StillDeletedError reports an asset that the API saved but still marks as
// deleted.
type StillDeletedError struct {
	DisplayKind string
	Name        string
	ID          string
}

func (e *StillDeletedError) Error() string {
	return fmt.Sprintf("%s %q was saved, but the API still shows it as deleted", e.DisplayKind, e.Name)
}

// RequireNotDeleted returns a StillDeletedError when the API response for a
// create or update still marks the asset as deleted.
func RequireNotDeleted(displayKind, name, id string, deletedAt *time.Time) error {
	if deletedAt == nil {
		return nil
	}
	return &StillDeletedError{DisplayKind: displayKind, Name: name, ID: id}
}

// ignoreNotFound turns a "not found" preflight into success: nothing is stored
// yet, so the write creates the asset.
func ignoreNotFound(err error) error {
	if dash0api.IsNotFound(err) {
		return nil
	}
	return err
}
