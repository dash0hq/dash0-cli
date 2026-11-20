package asset

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
)

// InferredOriginPrefix starts every origin the CLI infers, so Dash0 shows the
// asset as managed by the CLI.
const InferredOriginPrefix = "dash0-cli_"

// Identity is what a document names its asset by.
type Identity struct {
	// ID is the document's id, or "".
	ID string
	// Origin is the document's dash0.com/origin, or "".
	Origin string
	// Inferred is the origin a document without one is created by.
	Inferred string
}

// CreateOrigin is the origin the asset is created by.
func (i Identity) CreateOrigin() string {
	if i.Origin != "" {
		return i.Origin
	}
	return i.Inferred
}

// Route is where a write goes.
type Route[T any] struct {
	// Key is the id or origin the PUT targets.
	Key string
	// Create is true when nothing is stored at Key, so the write sends the
	// origin. An update never sends it, so a stored origin stays as created.
	Create bool
	// Stored is the stored asset; the zero value when Create is true.
	Stored T
}

// RouteDocument is the one routing rule for every kind: id first, then origin,
// never POST, so a repeated apply cannot create a second asset. A failed id
// read leaves Key empty, because the write has no target; a failed origin read
// keeps Key, because the origin is the target either way.
func RouteDocument[T any](ctx context.Context, identity Identity, get func(context.Context, string) (T, error), storedOrigin func(T) string) (Route[T], error) {
	if identity.ID != "" {
		stored, err := get(ctx, identity.ID)
		if err == nil {
			if origin := storedOrigin(stored); identity.Origin != "" && origin != "" && origin != identity.Origin {
				return Route[T]{}, &OriginMismatchError{ID: identity.ID, Stored: origin, Document: identity.Origin}
			}
			return Route[T]{Key: identity.ID, Stored: stored}, nil
		}
		if !dash0api.IsNotFound(err) {
			return Route[T]{}, err
		}
	}
	route := Route[T]{Key: identity.CreateOrigin(), Create: true}
	if claim, ok := ctx.Value(inferredOriginClaimKey{}).(inferredOriginClaim); ok {
		if err := claim.writes.Claim(identity, route.Key, claim.location); err != nil {
			return Route[T]{}, err
		}
	}
	stored, err := get(ctx, route.Key)
	if err != nil {
		return route, ignoreNotFound(err)
	}
	route.Create, route.Stored = false, stored
	return route, nil
}

// OriginMismatchError reports a document whose id names an asset that Dash0
// stores with another origin. apply never changes a stored origin.
type OriginMismatchError struct {
	ID       string
	Stored   string
	Document string
}

func (e *OriginMismatchError) Error() string {
	return fmt.Sprintf("the asset with id %q has origin %q in Dash0, but the document sets origin %q, and apply never changes a stored origin"+
		"\nHint: remove dash0.com/origin from the document to update that asset, or remove dash0.com/id to create or update the asset with origin %q",
		e.ID, e.Stored, e.Document, e.Document)
}

// InferOrigin returns the origin of a document without one. It depends only on
// the kind, metadata.name, and dataset, so the same document gets the same
// origin from every checkout, and a different one in every dataset.
func InferOrigin(kind, name, dataset string) string {
	// The NUL keeps name "a" in dataset "bc" apart from name "ab" in dataset "c".
	sum := sha256.Sum256([]byte(NormalizeKind(kind) + "\x00" + name + "\x00" + dataset))
	return InferredOriginPrefix + hex.EncodeToString(sum[:16])
}

// InferKindOrigin returns the origin apply infers for a document of kind
// named name. Teams and notification channels are organization-level, so
// theirs has no dataset.
func InferKindOrigin(kind, name string, dataset *string) string {
	switch NormalizeKind(kind) {
	case "team", "notificationchannel":
		return InferOrigin(kind, name, "")
	}
	return InferOrigin(kind, name, DatasetName(dataset))
}

// DatasetName returns the dataset an asset lives in. Without --dataset the API
// uses "default", so both name the same asset and must infer the same origin.
func DatasetName(dataset *string) string {
	if dataset == nil || *dataset == "" {
		return "default"
	}
	return *dataset
}

// routedAsset is an asset whose identifiers the router reads.
type routedAsset interface {
	GetID() string
	GetOrigin() string
}

func identityOf(kind, name string, dataset *string, asset routedAsset) Identity {
	return Identity{ID: asset.GetID(), Origin: asset.GetOrigin(), Inferred: InferKindOrigin(kind, name, dataset)}
}

// CheckInferredOriginCollisions fails documents without an id or an origin
// that would get the same inferred origin, because the second would replace
// the first. A document with an id writes by the inferred origin only when the
// id is not found, which only the run knows; InferredOriginWrites checks that.
func CheckInferredOriginCollisions(documents []Document, dataset *string) []string {
	var origins []string
	locations := make(map[string][]string)
	for _, doc := range documents {
		identity, ok := documentIdentity(doc, dataset)
		if !ok || identity.ID != "" || identity.Origin != "" {
			continue
		}
		if _, seen := locations[identity.Inferred]; !seen {
			origins = append(origins, identity.Inferred)
		}
		locations[identity.Inferred] = append(locations[identity.Inferred], doc.Location())
	}

	var collisions []string
	for _, origin := range origins {
		if uses := locations[origin]; len(uses) > 1 {
			collisions = append(collisions, fmt.Sprintf(
				"%d documents have no dash0.com/origin and the same kind, metadata.name, and dataset, so any of them created by inferred origin %q would replace the others; set dash0.com/origin on each:\n    %s",
				len(uses), origin, strings.Join(uses, "\n    ")))
		}
	}
	return collisions
}

// InferredOriginWrites maps each inferred origin a run writes to the document
// that writes it, so a second document writing there fails instead of
// replacing the first.
type InferredOriginWrites map[string]string

// Claim records that the document at location writes key. It fails when
// another document of the run already writes key as its inferred origin.
func (w InferredOriginWrites) Claim(identity Identity, key, location string) error {
	if identity.Origin != "" || key != identity.Inferred {
		return nil
	}
	if first, ok := w[key]; ok && first != location {
		return fmt.Errorf("%s also writes inferred origin %q in this run, so one document would replace the other's asset; set dash0.com/origin on both", first, key)
	}
	w[key] = location
	return nil
}

// ClaimDocument is Claim for a document whose write targets key, for callers
// that plan documents without routing them in run order.
func (w InferredOriginWrites) ClaimDocument(doc Document, dataset *string, key string) error {
	identity, ok := documentIdentity(doc, dataset)
	if !ok {
		return nil
	}
	return w.Claim(identity, key, doc.Location())
}

type inferredOriginClaimKey struct{}

type inferredOriginClaim struct {
	writes   InferredOriginWrites
	location string
}

// WithInferredOriginWrites makes RouteDocument claim the inferred origin the
// document at location writes, before the write.
func WithInferredOriginWrites(ctx context.Context, writes InferredOriginWrites, location string) context.Context {
	return context.WithValue(ctx, inferredOriginClaimKey{}, inferredOriginClaim{writes: writes, location: location})
}

// documentIdentity returns the identity apply routes the document by, and
// false for kinds the router does not serve yet. Parse errors are reported
// when the document is applied.
func documentIdentity(doc Document, dataset *string) (Identity, bool) {
	switch NormalizeKind(doc.Kind) {
	case "dashboard", "ephemeraldashboard", "persesdashboard":
		dashboard, err := dash0yaml.ParseAsDashboard(doc.Raw)
		if err != nil {
			return Identity{}, false
		}
		return DashboardIdentity(dashboard, dataset), true
	case "team":
		var team dash0api.TeamDefinitionV1Alpha1
		if err := dash0yaml.Unmarshal(doc.Raw, &team); err != nil {
			return Identity{}, false
		}
		return TeamIdentity(&team), true
	case "notificationchannel":
		var channel dash0api.NotificationChannelDefinition
		if err := dash0yaml.Unmarshal(doc.Raw, &channel); err != nil {
			return Identity{}, false
		}
		return NotificationChannelIdentity(&channel), true
	case "timeseriesaggregation":
		var aggregation dash0api.TimeSeriesAggregationDefinition
		if err := dash0yaml.Unmarshal(doc.Raw, &aggregation); err != nil {
			return Identity{}, false
		}
		return TimeSeriesAggregationIdentity(&aggregation, dataset), true
	case "spamfilter":
		// Both apiVersions share the metadata shape.
		var filter dash0api.SpamFilter
		if err := dash0yaml.Unmarshal(doc.Raw, &filter); err != nil {
			return Identity{}, false
		}
		return SpamFilterIdentity(&filter, dataset), true
	}
	return Identity{}, false
}

// optionalString returns nil for "", so an empty label is left out of the body.
func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
