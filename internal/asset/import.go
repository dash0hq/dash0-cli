package asset

// ImportAction indicates whether an asset was created or updated.
type ImportAction string

const (
	ActionCreated ImportAction = "created"
	ActionUpdated ImportAction = "updated"
)

// ImportResult holds the outcome of importing an asset.
type ImportResult struct {
	Name   string
	ID     string
	Action ImportAction
	Before any // asset state before update (nil for creates)
	After  any // asset state after update/create
}
