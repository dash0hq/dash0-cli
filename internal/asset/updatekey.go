package asset

import (
	"fmt"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// OriginLabel is where the dash0.com/origin label lives in a document, for
// error messages that tell the user where to set it.
const OriginLabel = `metadata.labels["dash0.com/origin"]`

// UpdateKey is what every `update -f <file>` chooses its key from. One shared
// rule keeps update and apply addressing the same asset for every kind.
type UpdateKey struct {
	// Noun names the asset in error messages, lowercase and unabbreviated:
	// "dashboard", "spam filter", "time series aggregation".
	Noun string

	// Upsert is the document. Read it before a Strip*ServerFields call, which
	// can clear both labels.
	Upsert dash0api.AssetMetadata
}

// ResolveUpdateKey returns the originOrId to update. Without an argument it is
// the key apply upserts by, with no fallback. An argument must match the file,
// because a PUT onto an asset the file does not name cannot be undone.
func ResolveUpdateKey(args []string, k UpdateKey) (string, error) {
	byOrigin, _ := k.Upsert.UpsertKeys()
	origin := ""
	if byOrigin {
		origin = k.Upsert.GetOrigin()
	}
	// An exported document carries the server-assigned id even for kinds that
	// upsert by origin only, and the API path accepts it, so it stays a valid
	// argument.
	id := k.Upsert.GetID()

	if len(args) > 0 {
		arg := args[0]
		switch {
		case origin != "" && id != "":
			if arg != origin && arg != id {
				return "", fmt.Errorf(
					"argument %q matches neither the file's origin (%q) nor its id (%q)",
					arg, origin, id,
				)
			}
		case origin != "":
			if arg != origin {
				return "", fmt.Errorf("argument %q does not match the file's origin (%q)", arg, origin)
			}
		case id != "":
			if arg != id {
				return "", fmt.Errorf("argument %q does not match the file's id (%q)", arg, id)
			}
		}
		// The document names no identifier at all, so there is nothing for the
		// argument to contradict.
		return arg, nil
	}

	if key := PreferredKey(k.Upsert); key != "" {
		return key, nil
	}
	if byOrigin {
		return "", fmt.Errorf(
			"no %s origin or id given: pass one as an argument, or set %s in the file",
			k.Noun, OriginLabel,
		)
	}
	return "", fmt.Errorf("no %s id given: pass one as an argument, or set the id in the file", k.Noun)
}
