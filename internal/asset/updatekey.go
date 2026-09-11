package asset

import "fmt"

// OriginLabel is where the dash0.com/origin label lives in a document, for
// error messages that tell the user where to set it.
const OriginLabel = `metadata.labels["dash0.com/origin"]`

// UpdateKey holds the identifiers an `update -f <file>` invocation chooses
// between: the optional positional argument, and whatever the document carries.
//
// Every asset kind faces the same decision, and each used to answer it inline.
// That gave the same mistake a different message per kind.
type UpdateKey struct {
	// Noun names the asset in error messages, lowercase and unabbreviated:
	// "dashboard", "spam filter", "time series aggregation".
	Noun string

	// UsesOrigin marks kinds whose dash0.com/origin label is an upsert key
	// (spam filters, teams, time series aggregations). It is separate from
	// Origin being set on purpose: such a kind still needs errors that mention
	// origin when the document omits the label, and a kind without an origin
	// key must never mention one.
	UsesOrigin bool

	// Origin is the document's dash0.com/origin label, empty when absent or
	// when UsesOrigin is false.
	Origin string

	// ID is the document's identifier, empty when absent. Where it lives
	// varies by kind. See the identifier table in docs/commands.md.
	ID string
}

// ResolveUpdateKey returns the value to send as the update's originOrId path
// parameter.
//
// Precedence is the positional argument, then the document's origin, then its
// ID. Origin comes first because that is the order the Import helpers upsert
// by, so `update` addresses whatever `apply` would have written.
//
// An argument matching neither identifier is a user error, not an override.
// Honoring it would PUT the document onto an asset the document does not name,
// which cannot be undone. Failing costs one retry.
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
