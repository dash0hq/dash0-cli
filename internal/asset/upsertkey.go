package asset

import dash0api "github.com/dash0hq/dash0-api-client-go"

// PreferredKey returns the identifier a document is upserted by: its origin
// when the kind upserts by origin and the document has one, else its id when
// the kind upserts by id, else "". Read it before a Strip*ServerFields call,
// which can clear both labels.
func PreferredKey(definition dash0api.AssetMetadata) string {
	byOrigin, byID := definition.UpsertKeys()
	if origin := definition.GetOrigin(); byOrigin && origin != "" {
		return origin
	}
	if byID {
		return definition.GetID()
	}
	return ""
}
