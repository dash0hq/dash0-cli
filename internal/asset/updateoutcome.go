package asset

import (
	"fmt"
	"io"
	"time"
)

// UpdateOutcome is an asset before and after an `update` command, or before
// and after the update that `update --dry-run` previews.
type UpdateOutcome struct {
	DisplayKind     string
	Name            string
	ID              string
	Before          any
	After           any
	BeforeDeletedAt *time.Time
	AfterDeletedAt  *time.Time
	DryRun          bool
}

// PrintUpdateOutcome writes the outcome of an update to w. For a deleted
// asset it prints `restored`, or `would be restored` on a dry run, followed by
// a diff of any other changes. For any other asset it prints a diff. After a
// real update it returns a StillDeletedError when the API response still marks
// the asset as deleted.
func PrintUpdateOutcome(w io.Writer, outcome UpdateOutcome) error {
	if !outcome.DryRun {
		if err := RequireNotDeleted(outcome.DisplayKind, outcome.Name, outcome.ID, outcome.AfterDeletedAt); err != nil {
			return err
		}
	}
	if outcome.BeforeDeletedAt == nil {
		return PrintDiff(w, outcome.DisplayKind, outcome.Name, outcome.Before, outcome.After)
	}
	restored := string(ActionRestored)
	if outcome.DryRun {
		restored = "would be restored"
	}
	fmt.Fprintf(w, "%s %q %s\n", outcome.DisplayKind, outcome.Name, restored)
	return PrintChanges(w, outcome.DisplayKind, outcome.Before, outcome.After)
}
