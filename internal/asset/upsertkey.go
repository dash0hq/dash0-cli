package asset

// UpsertKey is the identifiers a document is upserted by. A field the kind
// does not upsert by is empty.
type UpsertKey struct {
	Origin string
	ID     string
}

// Preferred returns the identifier to address first: the origin, or the id
// when there is no origin. It is "" when the document carries neither.
func (k UpsertKey) Preferred() string {
	if k.Origin != "" {
		return k.Origin
	}
	return k.ID
}

// originKinds upsert by dash0.com/origin. Notification channels and time series
// aggregations have no other key.
var originKinds = map[string]bool{
	"slo": true, "spamfilter": true, "team": true, "skill": true,
	"generativeaievaluation": true, "signaltometrics": true,
	"notificationchannel": true, "timeseriesaggregation": true,
}

// originOnlyKinds never upsert by id: the API reads only the origin from the
// path, so an id-only document would address nothing.
var originOnlyKinds = map[string]bool{
	"notificationchannel": true, "timeseriesaggregation": true,
}

// UpsertKeyFor keeps the identifiers kind upserts by and drops the rest, so a
// caller cannot address an asset by a field the kind ignores. candidate.ID is
// the kind's own id field: see the identifier table in docs/commands.md.
func UpsertKeyFor(kind string, candidate UpsertKey) UpsertKey {
	kind = NormalizeKind(kind)
	var key UpsertKey
	if originKinds[kind] {
		key.Origin = candidate.Origin
	}
	if !originOnlyKinds[kind] {
		key.ID = candidate.ID
	}
	return key
}
