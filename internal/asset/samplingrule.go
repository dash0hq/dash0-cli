package asset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
)

// SamplingRuleDisplayKind is the display name of the Dash0Sampling kind, per
// the asset kind display-name table in docs/cli-naming-conventions.md.
const SamplingRuleDisplayKind = "Sampling rule"

// samplingRuleMaxMatchingConditions is the most ottl and error conditions a
// sampling rule may hold across its whole condition tree. Probabilistic
// conditions do not count. The API accepts a rule above the limit, but Dash0
// then refuses to load it, and the rule silently stops matching.
const samplingRuleMaxMatchingConditions = 32

// GetSamplingRuleOrigin extracts the dash0.com/origin label. The API client
// ships accessors for the id, name, and dataset labels, but not this one.
func GetSamplingRuleOrigin(rule *dash0api.SamplingDefinition) string {
	if rule == nil || rule.Metadata.Labels == nil || rule.Metadata.Labels.Dash0Comorigin == nil {
		return ""
	}
	return *rule.Metadata.Labels.Dash0Comorigin
}

// SamplingRuleKey returns the identifier ImportSamplingRule upserts by: the
// dash0.com/origin label, else the dash0.com/id label, else "".
func SamplingRuleKey(rule *dash0api.SamplingDefinition) string {
	return PreferredKey(SamplingRuleKeys(rule))
}

// SamplingRuleKeys lets the shared key rules resolve a sampling rule's
// identifiers: the client gives SamplingDefinition no UpsertKeys. Sampling
// rules upsert by origin, with the id as the fallback. Read it before
// StripSamplingRuleServerFields, which clears both labels.
func SamplingRuleKeys(rule *dash0api.SamplingDefinition) dash0api.AssetMetadata {
	return samplingRuleKeys{
		origin: GetSamplingRuleOrigin(rule),
		id:     dash0api.GetSamplingRuleID(rule),
		name:   dash0api.GetSamplingRuleName(rule),
	}
}

type samplingRuleKeys struct{ origin, id, name string }

func (k samplingRuleKeys) GetOrigin() string                 { return k.origin }
func (k samplingRuleKeys) GetID() string                     { return k.id }
func (k samplingRuleKeys) GetName() string                   { return k.name }
func (k samplingRuleKeys) UpsertKeys() (byOrigin, byID bool) { return true, true }

// ParseSamplingRule decodes a sampling rule document and checks what the API
// would accept but what could never work, before any API call. apply runs it
// for every document before the first write, so a multi-document apply never
// writes some rules and then fails on a problem the CLI can find by itself.
//
//   - spec.enabled must be present. The client type holds it as a plain bool,
//     so a document without it would silently create a disabled rule.
//   - spec.conditions must be present, because the API requires it.
//   - The condition tree may hold at most 32 ottl and error conditions. Dash0
//     refuses to load a rule above that limit, and the rule stops matching.
func ParseSamplingRule(data []byte) (*dash0api.SamplingDefinition, error) {
	var rule dash0api.SamplingDefinition
	if err := dash0yaml.Unmarshal(data, &rule); err != nil {
		return nil, fmt.Errorf("failed to parse sampling rule definition: %w", err)
	}
	var present struct {
		Spec struct {
			Enabled    *bool           `json:"enabled"`
			Conditions json.RawMessage `json:"conditions"`
		} `json:"spec"`
	}
	if err := dash0yaml.Unmarshal(data, &present); err != nil {
		return nil, fmt.Errorf("failed to parse sampling rule definition: %w", err)
	}
	if present.Spec.Enabled == nil {
		return nil, errors.New("sampling rule definition requires spec.enabled" +
			"\nHint: set spec.enabled to true or false; without it the rule would be created disabled")
	}
	if len(present.Spec.Conditions) == 0 || string(present.Spec.Conditions) == "null" {
		return nil, errors.New("sampling rule definition requires spec.conditions")
	}
	condition, err := parseSamplingCondition(present.Spec.Conditions)
	if err != nil {
		return nil, fmt.Errorf("invalid spec.conditions: %w", err)
	}
	if n := condition.matchingConditions(); n > samplingRuleMaxMatchingConditions {
		return nil, fmt.Errorf("sampling rule spec.conditions holds %d ottl and error conditions, more than the limit of %d"+
			"\nHint: Dash0 does not load a rule above the limit, so the rule would never keep a trace; split it into several rules",
			n, samplingRuleMaxMatchingConditions)
	}
	return &rule, nil
}

// samplingCondition is a sampling condition decoded generically, so the CLI
// can walk the tree without the client's union accessors.
type samplingCondition struct {
	Kind string `json:"kind"`
	Spec struct {
		Conditions []samplingCondition `json:"conditions"`
		Ottl       string              `json:"ottl"`
		Rate       *float64            `json:"rate"`
	} `json:"spec"`
}

func parseSamplingCondition(raw []byte) (samplingCondition, error) {
	var c samplingCondition
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, err
	}
	return c, nil
}

// matchingConditions counts the ottl and error conditions in the tree, which
// is what the per-rule limit applies to.
func (c samplingCondition) matchingConditions() int {
	switch c.Kind {
	case "ottl", "error":
		return 1
	case "and":
		n := 0
		for _, child := range c.Spec.Conditions {
			n += child.matchingConditions()
		}
		return n
	default:
		return 0
	}
}

// describe renders the condition tree on one line, for example
// "and(error, probabilistic 0.25)". OTTL expressions are left out, because
// they can be long; `get -o yaml` shows them.
func (c samplingCondition) describe() string {
	switch c.Kind {
	case "probabilistic":
		if c.Spec.Rate == nil {
			return "probabilistic"
		}
		return "probabilistic " + strconv.FormatFloat(*c.Spec.Rate, 'g', -1, 64)
	case "and":
		parts := make([]string, len(c.Spec.Conditions))
		for i, child := range c.Spec.Conditions {
			parts[i] = child.describe()
		}
		return "and(" + strings.Join(parts, ", ") + ")"
	default:
		return c.Kind
	}
}

// DescribeSamplingRuleConditions renders a rule's condition tree on one line
// for table and summary output, or "" when it cannot be decoded.
func DescribeSamplingRuleConditions(rule *dash0api.SamplingDefinition) string {
	if rule == nil {
		return ""
	}
	raw, err := rule.Spec.Conditions.MarshalJSON()
	if err != nil || string(raw) == "null" {
		return ""
	}
	c, err := parseSamplingCondition(raw)
	if err != nil {
		return ""
	}
	return c.describe()
}

// IsSamplingRuleWrongDataset reports whether err is the API's cross-dataset
// collision: HTTP 400 saying a rule with this origin or ID exists but belongs
// to a different dataset.
//
// Origins and IDs are unique per organization while each rule belongs to one
// dataset. GET, PUT, and DELETE through another dataset therefore fail with
// this 400. It is not a 404: the rule exists, so delete --force must not report
// it as already gone.
func IsSamplingRuleWrongDataset(err error) bool {
	return isOriginInAnotherDataset(err)
}

// WrapSamplingRuleWrongDataset turns the cross-dataset 400 into an error that
// names the cause and the fix, with the advice behind "\nHint:" so agent mode
// lifts it into the JSON error's hint field.
func WrapSamplingRuleWrongDataset(err error, originOrID string) error {
	return fmt.Errorf(
		"sampling rule %q already exists in another dataset:\n  %w"+
			"\nHint: origins and IDs are unique per organization while each rule belongs to one dataset. "+
			"Use a distinct origin per dataset, or pass the --dataset that owns the rule",
		originOrID, err,
	)
}

// SamplingRuleUsesOrigin reports whether a sampling rule document carries a
// non-empty dash0.com/origin label.
//
// --since uses this to warn when a rule is about to be deleted by dash0.com/id
// alone. An id-only document first applied to an organization without that id
// took ImportSamplingRule's POST fallback, and the live rule sits at an id the
// document never learned. Deleting by the id recorded in git history then
// misses it.
func SamplingRuleUsesOrigin(data []byte) (bool, error) {
	var rule dash0api.SamplingDefinition
	if err := dash0yaml.Unmarshal(data, &rule); err != nil {
		return false, fmt.Errorf("failed to decode sampling rule: %w", err)
	}
	return GetSamplingRuleOrigin(&rule) != "", nil
}

// ImportSamplingRule creates or upserts a sampling rule.
//
// The upsert keys follow ImportSignalToMetricsRule:
//
//   - With a dash0.com/origin label, the rule is PUT to that origin, which
//     creates or replaces it, with any dash0.com/id cleared from the body. The
//     preflight GET only decides whether the result is reported as created or
//     updated.
//   - With only a dash0.com/id label, a preflight GET gates the route. On a hit
//     the rule is PUT to that id. On a 404 it is POSTed with the id cleared,
//     because the server rejects an id in a POST body, and a PUT to an id that
//     another organization owns fails: ids are unique across organizations.
//     The POST fallback lets a document exported from another organization
//     apply, but it is not idempotent: the next apply preflights the same
//     unknown id and POSTs again. Any other preflight error is surfaced.
//   - With neither label, the rule is POSTed and the server assigns both.
//
// Both labels are read before StripSamplingRuleServerFields, which clears the
// origin and the dataset label. The dataset label must not be sent: on writes
// it currently wins over the dataset query parameter, so an exported rule
// would otherwise land in the dataset it was exported from.
//
// The API deletes sampling rules for good, so a write never restores one.
func ImportSamplingRule(ctx context.Context, apiClient dash0api.Client, rule *dash0api.SamplingDefinition, dataset *string) (ImportResult, error) {
	id := dash0api.GetSamplingRuleID(rule)
	plan, err := PlanSamplingRule(ctx, apiClient, rule, dataset)
	if err != nil {
		return ImportResult{}, err
	}

	var result *dash0api.SamplingDefinition
	if plan.Key != "" {
		result, err = apiClient.UpdateSamplingRule(ctx, plan.Key, rule, dataset)
	} else {
		result, err = apiClient.CreateSamplingRule(ctx, rule, dataset)
	}
	if IsSamplingRuleWrongDataset(err) {
		return ImportResult{}, WrapSamplingRuleWrongDataset(err, plan.Key)
	}
	if err != nil {
		return ImportResult{}, err
	}
	if result == nil {
		return ImportResult{}, fmt.Errorf("the API returned no sampling rule")
	}

	if resultID := dash0api.GetSamplingRuleID(result); resultID != "" {
		id = resultID
	}
	return ImportResult{
		Name:     dash0api.GetSamplingRuleName(result),
		ID:       id,
		Action:   plan.Action,
		Before:   plan.Before,
		After:    result,
		Warnings: plan.Warnings,
	}, nil
}

// PlanSamplingRule reads the stored rule that ImportSamplingRule would write,
// and prepares rule as the request body. An empty Key means a POST: the
// document has no origin, and its id is unknown or missing.
//
// Only a PUT to an id keeps the id in the body. A PUT to an origin selects the
// rule by the path, so an id in the body can only disagree with it. A POST
// must not carry one, because the server assigns ids.
//
// Unlike signal-to-metrics rules, the GET honors the dataset: a rule stored in
// another dataset fails it with the cross-dataset 400, which is returned
// wrapped, so a preview fails the same way the write would.
//
// When the write would turn sampling on for the dataset, the plan carries a
// warning: once a dataset has an enabled rule, Dash0 drops every trace that no
// enabled rule keeps.
func PlanSamplingRule(ctx context.Context, apiClient dash0api.Client, rule *dash0api.SamplingDefinition, dataset *string) (ImportPlan, error) {
	origin := GetSamplingRuleOrigin(rule)
	id := dash0api.GetSamplingRuleID(rule)
	dash0api.StripSamplingRuleServerFields(rule)

	plan := ImportPlan{Action: ActionCreated, Proposed: rule}
	var existing *dash0api.SamplingDefinition
	switch {
	case origin != "":
		// PUT to an unknown origin creates the rule, so a 404 keeps the origin
		// as the key.
		plan.Key = origin
		stored, err := apiClient.GetSamplingRule(ctx, origin, dataset)
		switch {
		case err == nil:
			existing = stored
		case dash0api.IsNotFound(err):
		case IsSamplingRuleWrongDataset(err):
			return plan, WrapSamplingRuleWrongDataset(err, origin)
		default:
			// Keep the plan, so a caller that drops a denied read still names
			// the rule by its origin.
			return plan, err
		}
	case id != "":
		stored, err := apiClient.GetSamplingRule(ctx, id, dataset)
		switch {
		case err == nil:
			existing = stored
			plan.Key = id
		case dash0api.IsNotFound(err):
			// The id belongs to no rule in this organization: POST creates one.
		case IsSamplingRuleWrongDataset(err):
			return plan, WrapSamplingRuleWrongDataset(err, id)
		default:
			return plan, err
		}
	}
	if existing != nil {
		plan.Action, plan.Before = ActionUpdated, existing
	}
	if plan.Key != id {
		dash0api.ClearSamplingRuleID(rule)
	}

	if warning := SamplingTurnOnWarning(ctx, apiClient, rule, existing, dataset); warning != "" {
		plan.Warnings = append(plan.Warnings, warning)
	}
	return plan, nil
}

// SamplingRuleListDataset returns the dataset to send when listing sampling
// rules. The CLI represents the default dataset as nil, and the other sampling
// rule endpoints treat a missing dataset as "default", but the list endpoint
// then returns the rules of every dataset. So the list always names the
// dataset.
func SamplingRuleListDataset(dataset *string) *string {
	if dataset == nil || *dataset == "" {
		defaultDataset := "default"
		return &defaultDataset
	}
	return dataset
}

// SamplingTurnOnWarning returns a warning when writing rule would turn
// sampling on for the dataset: the rule ends up enabled, it was not enabled
// before, and no other rule in the dataset is enabled. Sampling is off for a
// dataset until it has an enabled rule; from then on, Dash0 drops every trace
// that no enabled rule keeps, and dropped traces cannot be recovered.
func SamplingTurnOnWarning(ctx context.Context, apiClient dash0api.Client, rule, existing *dash0api.SamplingDefinition, dataset *string) string {
	if !rule.Spec.Enabled || (existing != nil && existing.Spec.Enabled) {
		return ""
	}
	target := "default"
	if dataset != nil && *dataset != "" {
		target = *dataset
	}
	name := dash0api.GetSamplingRuleName(rule)

	rules, err := apiClient.ListSamplingRules(ctx, SamplingRuleListDataset(dataset))
	if err != nil {
		// The check only feeds a warning, so a failed list must not block the
		// write. Say what could not be checked instead.
		return fmt.Sprintf("sampling rule %q is enabled, and the other rules in dataset %q could not be listed (%v); "+
			"if it is the first enabled rule there, it turns sampling on, and Dash0 drops every trace that no enabled rule keeps",
			name, target, err)
	}
	existingID := dash0api.GetSamplingRuleID(existing)
	for _, other := range rules {
		if other.Spec.Enabled && (existingID == "" || dash0api.GetSamplingRuleID(other) != existingID) {
			return ""
		}
	}
	return fmt.Sprintf("sampling rule %q is the first enabled sampling rule in dataset %q, so it turns sampling on: "+
		"Dash0 then drops every trace that no enabled sampling rule keeps, and dropped traces cannot be recovered",
		name, target)
}
