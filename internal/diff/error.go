package diff

import "github.com/dash0hq/dash0-cli/internal/asset"

// PendingDifferencesError signals that diff ran to completion and found one
// or more pending differences (creates, updates, or deletions) -- not a
// failure. main() maps it to exit code 1, distinct from both a clean diff
// (exit 0) and a genuine error (exit 2); the report itself is already
// printed to stdout/stderr by the time this is returned, so main() must not
// render it through the ordinary "Error:"-prefixed path.
type PendingDifferencesError struct {
	Count int
}

func (e *PendingDifferencesError) Error() string {
	return asset.Pluralize(e.Count, "difference") + " pending"
}
