package asset

import (
	"context"
	"fmt"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
)

// SignalToMetricsRuleDisplayKind is the display name of the Dash0SignalToMetrics
// kind, per the asset kind display-name table in docs/cli-naming-conventions.md.
const SignalToMetricsRuleDisplayKind = "Signal-to-metrics rule"

// GetSignalToMetricsRuleOrigin extracts the dash0.com/origin label. The API
// client ships accessors for the id, name, and dataset labels, but not this one.
func GetSignalToMetricsRuleOrigin(rule *dash0api.SignalToMetricsDefinition) string {
	if rule == nil || rule.Metadata.Labels == nil || rule.Metadata.Labels.Dash0Comorigin == nil {
		return ""
	}
	return *rule.Metadata.Labels.Dash0Comorigin
}

// SignalToMetricsRuleKey returns the identifier ImportSignalToMetricsRule
// upserts by: the dash0.com/origin label, else the dash0.com/id label, else "".
func SignalToMetricsRuleKey(rule *dash0api.SignalToMetricsDefinition) string {
	if origin := GetSignalToMetricsRuleOrigin(rule); origin != "" {
		return origin
	}
	return dash0api.GetSignalToMetricsID(rule)
}

// SignalToMetricsRuleDeletedAt returns when the rule was deleted, or nil when
// it is not deleted.
func SignalToMetricsRuleDeletedAt(rule *dash0api.SignalToMetricsDefinition) *time.Time {
	if rule == nil || rule.Metadata.Annotations == nil {
		return nil
	}
	return rule.Metadata.Annotations.Dash0ComdeletedAt
}

// IsSignalToMetricsRuleWrongDataset reports whether err is the API's
// cross-dataset origin collision: HTTP 400 saying a rule with this origin
// exists but belongs to a different dataset.
//
// Origins are unique per organization while each rule belongs to one dataset,
// as for time series aggregations. Writing or deleting by an origin from
// another dataset therefore fails with this 400. It is not a 404: the rule
// exists, so delete --force must not report it as already gone.
func IsSignalToMetricsRuleWrongDataset(err error) bool {
	return isOriginInAnotherDataset(err)
}

// WrapSignalToMetricsRuleWrongDataset turns the cross-dataset 400 into an
// error that names the cause and the fix, with the advice behind "\nHint:" so
// agent mode lifts it into the JSON error's hint field.
func WrapSignalToMetricsRuleWrongDataset(err error, originOrID string) error {
	return fmt.Errorf(
		"signal-to-metrics rule %q already exists in another dataset:\n  %w"+
			"\nHint: origins are unique per organization while each rule belongs to one dataset. "+
			"Use a distinct origin per dataset, or pass the --dataset that owns the rule (shown by 'dash0 s2m get %s')",
		originOrID, err, originOrID,
	)
}

// SignalToMetricsRuleUsesOrigin reports whether a signal-to-metrics rule
// document carries a non-empty dash0.com/origin label.
//
// --since uses this to warn when a rule is about to be deleted by dash0.com/id
// alone. Rule ids are server-assigned, so an id-only document first applied to
// an organization without that id took ImportSignalToMetricsRule's POST
// fallback, and the live rule sits at an id the document never learned.
// Deleting by the id recorded in git history then misses it.
func SignalToMetricsRuleUsesOrigin(data []byte) (bool, error) {
	var rule dash0api.SignalToMetricsDefinition
	if err := dash0yaml.Unmarshal(data, &rule); err != nil {
		return false, fmt.Errorf("failed to decode signal-to-metrics rule: %w", err)
	}
	return GetSignalToMetricsRuleOrigin(&rule) != "", nil
}

// ImportSignalToMetricsRule creates or upserts a signal-to-metrics rule.
//
// The API's PUT /api/signal-to-metrics/{originOrId} creates a rule when nothing
// matches the path value, and records that value as the new rule's origin.
// Upsert key selection follows from that, and mirrors ImportSLO:
//
//   - With a dash0.com/origin label, the rule is PUT to that origin, which
//     creates or replaces it, with any dash0.com/id cleared from the body. A
//     preflight GET only decides whether the result is reported as created,
//     updated, or restored.
//   - With only a dash0.com/id label, a preflight GET gates the route. On a hit
//     the rule is PUT to that id. On a 404 it is POSTed with the id cleared,
//     because the server assigns ids and rejects one in a POST body. A PUT here
//     would create a rule whose origin is the foreign id. The POST fallback
//     lets a document exported from another organization apply, but it is not
//     idempotent: the next apply preflights the same unknown id and POSTs again.
//     Any other preflight error is surfaced instead of risking a duplicate.
//   - With neither label, the rule is POSTed and the server assigns both.
//
// Both labels are read before StripSignalToMetricsServerFields, which clears
// the origin. A PUT to a deleted rule brings it back, which is reported as
// ActionRestored.
func ImportSignalToMetricsRule(ctx context.Context, apiClient dash0api.Client, rule *dash0api.SignalToMetricsDefinition, dataset *string) (ImportResult, error) {
	origin := GetSignalToMetricsRuleOrigin(rule)
	id := dash0api.GetSignalToMetricsID(rule)
	dash0api.StripSignalToMetricsServerFields(rule)

	action := ActionCreated
	var before any
	var upsertKey string
	switch {
	case origin != "":
		upsertKey = origin
		// The origin in the path selects the rule, so an id in the body can
		// only disagree with it, for example the id of the same rule in
		// another organization.
		dash0api.ClearSignalToMetricsID(rule)
		existing, err := apiClient.GetSignalToMetrics(ctx, origin, dataset)
		switch {
		case err == nil:
			action, before = actionForExisting(SignalToMetricsRuleDeletedAt(existing)), existing
		case dash0api.IsNotFound(err):
			// Nothing at this origin yet; the PUT below creates it.
		default:
			return ImportResult{}, err
		}
	case id != "":
		existing, err := apiClient.GetSignalToMetrics(ctx, id, dataset)
		switch {
		case err == nil:
			upsertKey = id
			action, before = actionForExisting(SignalToMetricsRuleDeletedAt(existing)), existing
		case dash0api.IsNotFound(err):
			// Fall through to POST.
		default:
			return ImportResult{}, err
		}
	}

	var result *dash0api.SignalToMetricsDefinition
	var err error
	if upsertKey != "" {
		result, err = apiClient.UpdateSignalToMetrics(ctx, upsertKey, rule, dataset)
	} else {
		dash0api.ClearSignalToMetricsID(rule)
		result, err = apiClient.CreateSignalToMetrics(ctx, rule, dataset)
	}
	if IsSignalToMetricsRuleWrongDataset(err) {
		// The preflight GET ignores the dataset and finds the rule, so the
		// collision only surfaces here, on the write.
		return ImportResult{}, WrapSignalToMetricsRuleWrongDataset(err, upsertKey)
	}
	if err != nil {
		return ImportResult{}, err
	}
	if result == nil {
		return ImportResult{}, fmt.Errorf("the API returned no signal-to-metrics rule")
	}

	if resultID := dash0api.GetSignalToMetricsID(result); resultID != "" {
		id = resultID
	}
	name := dash0api.GetSignalToMetricsName(result)
	if err := RequireNotDeleted(SignalToMetricsRuleDisplayKind, name, id, SignalToMetricsRuleDeletedAt(result)); err != nil {
		return ImportResult{}, err
	}
	return ImportResult{Name: name, ID: id, Action: action, Before: before, After: result}, nil
}
