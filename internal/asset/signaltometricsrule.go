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
// alone: an id-only rule that an earlier CLI version created by POST sits at
// a server-assigned id the document never learned, and the delete misses it.
func SignalToMetricsRuleUsesOrigin(data []byte) (bool, error) {
	var rule dash0api.SignalToMetricsDefinition
	if err := dash0yaml.Unmarshal(data, &rule); err != nil {
		return false, fmt.Errorf("failed to decode signal-to-metrics rule: %w", err)
	}
	return GetSignalToMetricsRuleOrigin(&rule) != "", nil
}

// ImportSignalToMetricsRule creates or updates a signal-to-metrics rule
// through RouteDocument. The path carries the origin, so the body never does.
// A PUT to a deleted rule brings it back, which is reported as ActionRestored.
func ImportSignalToMetricsRule(ctx context.Context, apiClient dash0api.Client, rule *dash0api.SignalToMetricsDefinition, dataset *string) (ImportResult, error) {
	plan, err := PlanSignalToMetricsRule(ctx, apiClient, rule, dataset)
	if err != nil {
		return ImportResult{}, err
	}

	result, err := apiClient.UpdateSignalToMetrics(ctx, plan.Key, rule, dataset)
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

	id := dash0api.GetSignalToMetricsID(result)
	name := dash0api.GetSignalToMetricsName(result)
	if err := RequireNotDeleted(SignalToMetricsRuleDisplayKind, name, id, SignalToMetricsRuleDeletedAt(result)); err != nil {
		return ImportResult{}, err
	}
	return ImportResult{Name: name, ID: id, Action: plan.Action, Before: plan.Before, After: result}, nil
}

// PlanSignalToMetricsRule routes the rule with RouteDocument, reads the stored
// rule that ImportSignalToMetricsRule would write, and prepares rule as the
// request body. Any preflight error other than "not found" is returned and
// stops the write.
//
// The GET ignores the dataset, so a stored rule from another dataset fails
// here with a SignalToMetricsRuleDatasetMismatchError instead of being
// compared against. Only a PUT to the id keeps the id in the body: a PUT to an
// origin selects the rule by the path, so an id in the body can only disagree
// with it, for example the id of the same rule in another organization.
func PlanSignalToMetricsRule(ctx context.Context, apiClient dash0api.Client, rule *dash0api.SignalToMetricsDefinition, dataset *string) (ImportPlan, error) {
	identity := SignalToMetricsRuleIdentity(rule, dataset)
	dash0api.StripSignalToMetricsServerFields(rule)
	get := func(ctx context.Context, key string) (*dash0api.SignalToMetricsDefinition, error) {
		return apiClient.GetSignalToMetrics(ctx, key, dataset)
	}
	route, err := RouteDocument(ctx, identity, get, (*dash0api.SignalToMetricsDefinition).GetOrigin)
	plan := ImportPlan{Key: route.Key, Action: ActionCreated, Proposed: rule}
	if route.Key != identity.ID {
		dash0api.ClearSignalToMetricsID(rule)
	}
	if err != nil {
		return plan, err
	}
	if !route.Create {
		if err := CheckSignalToMetricsRuleDataset(route.Stored, dataset, route.Key); err != nil {
			return plan, err
		}
		plan.Action, plan.Before = actionForExisting(SignalToMetricsRuleDeletedAt(route.Stored)), route.Stored
	}
	return plan, nil
}

// SignalToMetricsRuleIdentity returns what RouteDocument routes a
// signal-to-metrics rule by.
func SignalToMetricsRuleIdentity(rule *dash0api.SignalToMetricsDefinition, dataset *string) Identity {
	return identityOf("signaltometrics", rule.Metadata.Name, dataset, rule)
}
