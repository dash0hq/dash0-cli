package asset

import "fmt"

// idOnlyDeleteWarning is what `apply --since` needs for a kind whose live asset
// can sit at a different id than the one a document recorded: an id-only
// document applied where that id is unknown makes the server assign a new one.
type idOnlyDeleteWarning struct {
	// noun names the kind in the error for a document that fails to decode.
	noun       string
	usesOrigin func(data []byte) (bool, error)
	// format takes the asset's name and id, as asset.FormatNameAndID prints them.
	format string
}

// IdentifiedByIDOnly reports whether a document of a kind that needs the
// id-only delete warning carries no dash0.com/origin label. It is false for
// every other kind.
func IdentifiedByIDOnly(kind string, data []byte) (bool, error) {
	warning := kinds[NormalizeKind(kind)].idOnlyDelete
	if warning == nil {
		return false, nil
	}
	usesOrigin, err := warning.usesOrigin(data)
	if err != nil {
		return false, fmt.Errorf("failed to determine %s identifier source: %w", warning.noun, err)
	}
	return !usesOrigin, nil
}

// IDOnlyDeleteWarning returns the warning `apply --since` prints before it
// deletes an asset that IdentifiedByIDOnly reported. display is the asset's
// name and id.
func IDOnlyDeleteWarning(kind, display string) string {
	warning := kinds[NormalizeKind(kind)].idOnlyDelete
	if warning == nil {
		return ""
	}
	return fmt.Sprintf(warning.format, display)
}

// idOnlyDelete builds the warning for a kind. article is the noun with "a" or
// "an".
func idOnlyDelete(noun, article, plural string, usesOrigin func(data []byte) (bool, error)) *idOnlyDeleteWarning {
	return &idOnlyDeleteWarning{
		noun:       noun,
		usesOrigin: usesOrigin,
		format: "warning: " + noun + " %s was identified by dash0.com/id alone; --since deletes it at that id, or at its inferred origin when the id is not found, but " +
			article + " that an earlier dash0 version created by POST sits at a server-assigned id, and this delete misses it; set dash0.com/origin on " + plural + " you manage with --since",
	}
}
