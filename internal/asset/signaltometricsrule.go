package asset

import (
	"context"
	"errors"
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
//
// It also reports a SignalToMetricsRuleDatasetMismatchError, the same collision
// found by the CLI before the write.
func IsSignalToMetricsRuleWrongDataset(err error) bool {
	var mismatch *SignalToMetricsRuleDatasetMismatchError
	return errors.As(err, &mismatch) || isOriginInAnotherDataset(err)
}

// SignalToMetricsRuleDatasetMismatchError reports a stored rule that belongs to
// another dataset than the one a write targets.
type SignalToMetricsRuleDatasetMismatchError struct {
	OriginOrID string
	Stored     string
	Target     string
}

func (e *SignalToMetricsRuleDatasetMismatchError) Error() string {
	return fmt.Sprintf(
		"signal-to-metrics rule %q belongs to dataset %q, not %q"+
			"\nHint: each rule belongs to one dataset, and its origin and ID are unique per organization. "+
			"Pass --dataset %s to change this rule, or use a different %s for a rule in %q",
		e.OriginOrID, e.Stored, e.Target, e.Stored, OriginLabel, e.Target,
	)
}

// CheckSignalToMetricsRuleDataset returns a
// SignalToMetricsRuleDatasetMismatchError when the stored rule belongs to
// another dataset than dataset, where nil means "default".
//
// The API's GET by origin or ID ignores the dataset parameter, so it returns a
// rule from any dataset. A write through another dataset then fails with a 400,
// and a preview that compared against that rule would report no changes for a
// write that cannot succeed. A rule without a dataset label is not checked.
func CheckSignalToMetricsRuleDataset(existing *dash0api.SignalToMetricsDefinition, dataset *string, originOrID string) error {
	stored := dash0api.GetSignalToMetricsDataset(existing)
	if stored == "" {
		return nil
	}
	target := "default"
	if dataset != nil && *dataset != "" {
		target = *dataset
	}
	if stored == target {
		return nil
	}
	return &SignalToMetricsRuleDatasetMismatchError{OriginOrID: originOrID, Stored: stored, Target: target}
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
	id := dash0api.GetSignalToMetricsID(rule)
	plan, err := PlanSignalToMetricsRule(ctx, apiClient, rule, dataset)
	if err != nil {
		return ImportResult{}, err
	}

	var result *dash0api.SignalToMetricsDefinition
	if plan.Key != "" {
		result, err = apiClient.UpdateSignalToMetrics(ctx, plan.Key, rule, dataset)
	} else {
		result, err = apiClient.CreateSignalToMetrics(ctx, rule, dataset)
	}
	if IsSignalToMetricsRuleWrongDataset(err) {
		// The plan catches a stored rule from another dataset by its dataset
		// label. A rule without one still collides here, on the write.
		return ImportResult{}, WrapSignalToMetricsRuleWrongDataset(err, plan.Key)
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
	return ImportResult{Name: name, ID: id, Action: plan.Action, Before: plan.Before, After: result}, nil
}

// PlanSignalToMetricsRule reads the stored rule that ImportSignalToMetricsRule
// would write, and prepares rule as the request body. An empty Key means a
// POST: the document has no origin, and its id is unknown or missing.
//
// Only a PUT to an id keeps the id in the body. A PUT to an origin selects the
// rule by the path, so an id in the body can only disagree with it, for example
// the id of the same rule in another organization. A POST must not carry one,
// because the server assigns ids.
func PlanSignalToMetricsRule(ctx context.Context, apiClient dash0api.Client, rule *dash0api.SignalToMetricsDefinition, dataset *string) (ImportPlan, error) {
	origin := GetSignalToMetricsRuleOrigin(rule)
	id := dash0api.GetSignalToMetricsID(rule)
	dash0api.StripSignalToMetricsServerFields(rule)

	plan := ImportPlan{Action: ActionCreated, Proposed: rule}
	switch {
	case origin != "":
		// PUT to an unknown origin creates the rule, so a 404 keeps the origin
		// as the key.
		plan.Key = origin
		existing, err := apiClient.GetSignalToMetrics(ctx, origin, dataset)
		switch {
		case err == nil:
			if err := CheckSignalToMetricsRuleDataset(existing, dataset, origin); err != nil {
				return plan, err
			}
			plan.Action, plan.Before = actionForExisting(SignalToMetricsRuleDeletedAt(existing)), existing
		case dash0api.IsNotFound(err):
		default:
			// Keep the plan, so a caller that drops a denied read still names
			// the rule by its origin.
			return plan, err
		}
	case id != "":
		existing, err := apiClient.GetSignalToMetrics(ctx, id, dataset)
		switch {
		case err == nil:
			if err := CheckSignalToMetricsRuleDataset(existing, dataset, id); err != nil {
				return plan, err
			}
			plan.Key = id
			plan.Action, plan.Before = actionForExisting(SignalToMetricsRuleDeletedAt(existing)), existing
		case dash0api.IsNotFound(err):
			// The id belongs to another organization: POST creates the rule.
		default:
			return plan, err
		}
	}
	if plan.Key != id {
		dash0api.ClearSignalToMetricsID(rule)
	}
	return plan, nil
}
