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
	// CreatesAtID is set for kinds that keep the id in the file, as check
	// rules: an id that is not found is created at that id.
	CreatesAtID bool
}

// CreateOrigin is the origin the asset is created by.
func (i Identity) CreateOrigin() string {
	if i.CreatesAtID && i.ID != "" {
		return i.ID
	}
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
			if err := checkStoredOrigin(identity, storedOrigin(stored)); err != nil {
				return Route[T]{}, err
			}
			return Route[T]{Key: identity.ID, Stored: stored}, nil
		}
		if !dash0api.IsNotFound(err) {
			return Route[T]{}, err
		}
		if identity.CreatesAtID {
			return Route[T]{Key: identity.ID, Create: true}, nil
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
// stores with another origin. The CLI never changes a stored origin.
type OriginMismatchError struct {
	ID       string
	Stored   string
	Document string
}

func checkStoredOrigin(identity Identity, stored string) error {
	if identity.Origin != "" && stored != "" && stored != identity.Origin {
		return &OriginMismatchError{ID: identity.ID, Stored: stored, Document: identity.Origin}
	}
	return nil
}

func (e *OriginMismatchError) Error() string {
	return fmt.Sprintf("the asset with id %q has origin %q in Dash0, but the document sets origin %q, and the CLI never changes a stored origin"+
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
// named name. Teams, notification channels, and Agent0 skills are
// organization-level, so theirs has no dataset.
func InferKindOrigin(kind, name string, dataset *string) string {
	kind = InferredOriginKind(kind)
	switch kind {
	case "team", "notificationchannel", "skill":
		return InferOrigin(kind, name, "")
	}
	return InferOrigin(kind, name, DatasetName(dataset))
}

// InferredOriginKind returns the normalized kind an inferred origin hashes,
// so documents of two kinds that write the same origin compare equal.
func InferredOriginKind(kind string) string {
	kind = NormalizeKind(kind)
	if kind == "persesdashboard" {
		// apply converts a PersesDashboard to a Dashboard before it routes it.
		return "dashboard"
	}
	return kind
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

// CheckInferredOriginCollisions fails documents without an id that would
// write the same origin: two that get the same inferred origin, or one whose
// explicit origin is another's inferred origin, because the second would
// replace the first. A document with an id writes by the inferred origin only
// when the id is not found, which only the run knows; InferredOriginWrites
// checks that.
func CheckInferredOriginCollisions(documents []Document, dataset *string) []string {
	type use struct {
		location string
		explicit bool
	}
	var keys []string
	uses := make(map[string][]use)
	add := func(key string, u use) {
		if _, seen := uses[key]; !seen {
			keys = append(keys, key)
		}
		uses[key] = append(uses[key], u)
	}
	for _, doc := range documents {
		identity, ok := documentIdentity(doc, dataset)
		if !ok || identity.ID != "" {
			continue
		}
		group := writeGroup(doc.Kind)
		if identity.Origin == "" {
			add(group+"\x00"+identity.Inferred, use{location: doc.Location()})
		} else if group != "prometheusrule" {
			// A PrometheusRule writes each alert at an origin derived from its own.
			add(group+"\x00"+identity.Origin, use{location: doc.Location(), explicit: true})
		}
	}

	var collisions []string
	for _, key := range keys {
		list := uses[key]
		origin := key[strings.IndexByte(key, 0)+1:]
		var locations []string
		inferred, explicit := 0, 0
		for _, u := range list {
			locations = append(locations, u.location)
			if u.explicit {
				explicit++
			} else {
				inferred++
			}
		}
		switch {
		case inferred == 0 || len(list) < 2:
			// Only check rules check equal explicit origins (checkRuleIDCollisions).
		case explicit == 0:
			collisions = append(collisions, fmt.Sprintf(
				"%d documents have no dash0.com/origin and the same kind, metadata.name, and dataset, so any of them created by inferred origin %q would replace the others; set dash0.com/origin on each:\n    %s",
				len(list), origin, strings.Join(locations, "\n    ")))
		default:
			collisions = append(collisions, fmt.Sprintf(
				"origin %q is set as dash0.com/origin and is also the origin the CLI infers for a document without one, so one would replace the other's asset; set another dash0.com/origin:\n    %s",
				origin, strings.Join(locations, "\n    ")))
		}
	}
	return collisions
}

// writeGroup returns the kind whose endpoint a document writes to, so
// origins compare only between documents that write the same assets.
func writeGroup(kind string) string {
	switch kind = NormalizeKind(kind); kind {
	case "dashboard", "ephemeraldashboard", "persesdashboard":
		return "dashboard"
	}
	return kind
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
	identity, _, routed, err := parseRoutedIdentity(doc.Kind, doc.Raw, dataset)
	return identity, routed && err == nil
}

type routedParser func(raw []byte, dataset *string) (identity Identity, name string, err error)

// routedParsers holds every kind RouteDocument serves. A kind not in it is not
// routed yet.
var routedParsers = map[string]routedParser{
	"dashboard":              parseRoutedDashboard,
	"ephemeraldashboard":     parseRoutedDashboard,
	"persesdashboard":        parseRoutedDashboard,
	"team":                   parseRoutedTeam,
	"notificationchannel":    parseRoutedNotificationChannel,
	"timeseriesaggregation":  parseRoutedTimeSeriesAggregation,
	"spamfilter":             parseRoutedSpamFilter,
	"slo":                    parseRoutedSLO,
	"generativeaievaluation": parseRoutedGenerativeAiEvaluation,
	"signaltometrics":        parseRoutedSignalToMetricsRule,
	"sampling":               parseRoutedSamplingRule,
	"skill":                  parseRoutedCustomSkill,
	"view":                   parseRoutedView,
	"syntheticcheck":         parseRoutedSyntheticCheck,
	"checkrule":              parseRoutedCheckRule,
	"prometheusrule":         parseRoutedPrometheusRule,
}

// IsRoutedKind reports whether RouteDocument serves kind.
func IsRoutedKind(kind string) bool {
	_, routed := routedParsers[NormalizeKind(kind)]
	return routed
}

// DocumentCreateOrigin returns the origin apply creates the document by, or ""
// for kinds the router does not serve.
func DocumentCreateOrigin(doc Document, dataset *string) string {
	identity, ok := documentIdentity(doc, dataset)
	if !ok {
		return ""
	}
	return identity.CreateOrigin()
}

// parseRoutedIdentity returns the identity apply routes the document by, the
// name its inferred origin comes from, and false for kinds the router does
// not serve yet.
func parseRoutedIdentity(kind string, raw []byte, dataset *string) (identity Identity, name string, routed bool, err error) {
	parse, routed := routedParsers[NormalizeKind(kind)]
	if !routed {
		return Identity{}, "", false, nil
	}
	identity, name, err = parse(raw, dataset)
	return identity, name, true, err
}

func parseRoutedDashboard(raw []byte, dataset *string) (Identity, string, error) {
	dashboard, inferredName, err := ParseDashboard(raw)
	if err != nil {
		return Identity{}, "", err
	}
	return DashboardIdentity(dashboard, inferredName, dataset), inferredName, nil
}

func parseRoutedTeam(raw []byte, _ *string) (Identity, string, error) {
	var team dash0api.TeamDefinitionV1Alpha1
	if err := dash0yaml.Unmarshal(raw, &team); err != nil {
		return Identity{}, "", err
	}
	return TeamIdentity(&team), team.Metadata.Name, nil
}

func parseRoutedNotificationChannel(raw []byte, _ *string) (Identity, string, error) {
	var channel dash0api.NotificationChannelDefinition
	if err := dash0yaml.Unmarshal(raw, &channel); err != nil {
		return Identity{}, "", err
	}
	return NotificationChannelIdentity(&channel), channel.Metadata.Name, nil
}

func parseRoutedTimeSeriesAggregation(raw []byte, dataset *string) (Identity, string, error) {
	var aggregation dash0api.TimeSeriesAggregationDefinition
	if err := dash0yaml.Unmarshal(raw, &aggregation); err != nil {
		return Identity{}, "", err
	}
	return TimeSeriesAggregationIdentity(&aggregation, dataset), aggregation.Metadata.Name, nil
}

func parseRoutedSpamFilter(raw []byte, dataset *string) (Identity, string, error) {
	// Both apiVersions share the metadata shape.
	var filter dash0api.SpamFilter
	if err := dash0yaml.Unmarshal(raw, &filter); err != nil {
		return Identity{}, "", err
	}
	return SpamFilterIdentity(&filter, dataset), filter.GetName(), nil
}

func parseRoutedSLO(raw []byte, dataset *string) (Identity, string, error) {
	var slo dash0api.SloDefinition
	if err := dash0yaml.Unmarshal(raw, &slo); err != nil {
		return Identity{}, "", err
	}
	return SLOIdentity(&slo, dataset), slo.Metadata.Name, nil
}

func parseRoutedGenerativeAiEvaluation(raw []byte, dataset *string) (Identity, string, error) {
	var evaluation dash0api.GenerativeAiEvaluationDefinition
	if err := dash0yaml.Unmarshal(raw, &evaluation); err != nil {
		return Identity{}, "", err
	}
	return GenerativeAiEvaluationIdentity(&evaluation, dataset), evaluation.Metadata.Name, nil
}

func parseRoutedSignalToMetricsRule(raw []byte, dataset *string) (Identity, string, error) {
	var rule dash0api.SignalToMetricsDefinition
	if err := dash0yaml.Unmarshal(raw, &rule); err != nil {
		return Identity{}, "", err
	}
	return SignalToMetricsRuleIdentity(&rule, dataset), rule.Metadata.Name, nil
}

func parseRoutedSamplingRule(raw []byte, dataset *string) (Identity, string, error) {
	var rule dash0api.SamplingDefinition
	if err := dash0yaml.Unmarshal(raw, &rule); err != nil {
		return Identity{}, "", err
	}
	return SamplingRuleIdentity(&rule, dataset), rule.Metadata.Name, nil
}

func parseRoutedCustomSkill(raw []byte, _ *string) (Identity, string, error) {
	var skill dash0api.SkillDefinition
	if err := dash0yaml.Unmarshal(raw, &skill); err != nil {
		return Identity{}, "", err
	}
	return CustomSkillIdentity(&skill), CustomSkillName(&skill), nil
}

func parseRoutedView(raw []byte, dataset *string) (Identity, string, error) {
	var view dash0api.ViewDefinition
	if err := dash0yaml.Unmarshal(raw, &view); err != nil {
		return Identity{}, "", err
	}
	return ViewIdentity(&view, dataset), view.Metadata.Name, nil
}

func parseRoutedSyntheticCheck(raw []byte, dataset *string) (Identity, string, error) {
	var check dash0api.SyntheticCheckDefinition
	if err := dash0yaml.Unmarshal(raw, &check); err != nil {
		return Identity{}, "", err
	}
	return SyntheticCheckIdentity(&check, dataset), check.Metadata.Name, nil
}

func parseRoutedCheckRule(raw []byte, dataset *string) (Identity, string, error) {
	var rule dash0api.PrometheusAlertRule
	if err := dash0yaml.Unmarshal(raw, &rule); err != nil {
		return Identity{}, "", err
	}
	return CheckRule{PrometheusAlertRule: &rule, Origin: rule.GetOrigin(), InferredKind: "checkrule", InferredName: rule.Name}.Identity(dataset), rule.Name, nil
}

func parseRoutedPrometheusRule(raw []byte, dataset *string) (Identity, string, error) {
	// The CRD's own labels. Each alert and the recording rules route by
	// keys derived from them, which deletePrometheusRuleCRD recomputes.
	crd, err := ParsePrometheusRuleCRD(raw)
	if err != nil {
		return Identity{}, "", err
	}
	return identityOf("prometheusrule", crd.Metadata.Name, dataset, crd), crd.Metadata.Name, nil
}

// optionalString returns nil for "", so an empty label is left out of the body.
func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
