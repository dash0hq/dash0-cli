package asset

import "fmt"

// OriginLabel is the document path of the dash0.com/origin label, for error
// messages that tell the user where to set it. Spam filters, teams,
// notification channels, and time series aggregations all upsert by it.
const OriginLabel = `metadata.labels["dash0.com/origin"]`

// UpdateKey describes the identifiers an `update -f <file>` invocation has to
// choose between: the optional positional argument, and whatever the document
// itself carries.
//
// Every asset kind's update command faces the same decision, and each one used
// to answer it inline. That produced a different error message per kind for the
// same mistake, so `dashboards update` and `spam-filters update` disagreed on
// how to phrase a mismatched argument. ResolveUpdateKey is the single answer.
type UpdateKey struct {
	// Noun names the asset in error messages, lowercase and unabbreviated:
	// "dashboard", "spam filter", "time series aggregation".
	Noun string

	// UsesOrigin marks kinds whose dash0.com/origin label is an upsert key
	// (spam filters, teams, time series aggregations). It is deliberately
	// separate from Origin being non-empty: an origin-keyed kind whose
	// document omits the label still needs errors that mention origin, while
	// a kind with no origin key must never mention one it does not have.
	UsesOrigin bool

	// Origin is the document's dash0.com/origin label, empty when absent or
	// when UsesOrigin is false.
	Origin string

	// ID is the document's identifier, empty when absent. Where it lives
	// varies by kind — see the identifier table in docs/commands.md.
	ID string
}

// ResolveUpdateKey returns the value to send as the update's originOrId path
// parameter.
//
// Precedence is the positional argument, then the document's origin, then its
// ID — origin before ID because that is the order the Import helpers upsert by,
// so `update` addresses whatever `apply` would have written.
//
// A positional argument that matches neither identifier in the document is a
// user error rather than an override. Honoring the argument would PUT the
// document's contents onto a different asset than the document names, which is
// unrecoverable; failing here costs one retry.
func ResolveUpdateKey(args []string, k UpdateKey) (string, error) {
	if len(args) > 0 {
		arg := args[0]
		switch {
		case k.Origin != "" && k.ID != "":
			if arg != k.Origin && arg != k.ID {
				return "", fmt.Errorf(
					"argument %q matches neither the file's origin (%q) nor its id (%q)",
					arg, k.Origin, k.ID,
				)
			}
		case k.Origin != "":
			if arg != k.Origin {
				return "", fmt.Errorf("argument %q does not match the file's origin (%q)", arg, k.Origin)
			}
		case k.ID != "":
			if arg != k.ID {
				return "", fmt.Errorf("argument %q does not match the file's id (%q)", arg, k.ID)
			}
		}
		// The document names no identifier at all, so there is nothing for the
		// argument to contradict.
		return arg, nil
	}

	if k.Origin != "" {
		return k.Origin, nil
	}
	if k.ID != "" {
		return k.ID, nil
	}

	if k.UsesOrigin {
		return "", fmt.Errorf(
			"no %s origin or id given: pass one as an argument, or set %s in the file",
			k.Noun, OriginLabel,
		)
	}
	return "", fmt.Errorf("no %s id given: pass one as an argument, or set the id in the file", k.Noun)
}
