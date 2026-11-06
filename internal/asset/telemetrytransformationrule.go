package asset

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
)

func ParseTelemetryTransformationRule(raw []byte) (*dash0api.TelemetryTransformationRuleDefinition, error) {
	var data json.RawMessage
	if err := dash0yaml.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("failed to parse telemetry transformation rule:\n  %w", err)
	}
	var rule dash0api.TelemetryTransformationRuleDefinition
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&rule); err != nil {
		return nil, fmt.Errorf("failed to parse telemetry transformation rule:\n  %w", err)
	}
	if string(rule.Kind) != "Dash0TelemetryTransformationRule" {
		return nil, fmt.Errorf("expected kind Dash0TelemetryTransformationRule, got %q", rule.Kind)
	}
	fields, err := transformationRequiredFields(data, "rule", "metadata", "spec")
	if err != nil {
		return nil, err
	}
	if _, err := transformationRequiredFields(fields["metadata"], "metadata", "name"); err != nil {
		return nil, err
	}
	spec, err := transformationRequiredFields(fields["spec"], "spec", "enabled", "signal", "context", "transformations")
	if err != nil {
		return nil, err
	}
	validContext := rule.Spec.Context == "resource" || rule.Spec.Context == "scope"
	switch rule.Spec.Signal {
	case "traces":
		validContext = validContext || rule.Spec.Context == "span" || rule.Spec.Context == "spanevent"
	case "metrics":
		validContext = validContext || rule.Spec.Context == "metric" || rule.Spec.Context == "datapoint"
	case "logs":
		validContext = validContext || rule.Spec.Context == "log"
	case "profiles":
		validContext = validContext || rule.Spec.Context == "profile"
	default:
		return nil, fmt.Errorf("invalid telemetry transformation signal %q", rule.Spec.Signal)
	}
	if !validContext {
		return nil, fmt.Errorf("context %q is not valid for signal %q", rule.Spec.Context, rule.Spec.Signal)
	}
	if p := rule.Spec.Priority; p != nil && (*p < 0 || *p > 400 || *p%100 != 0) {
		return nil, fmt.Errorf("spec.priority must be 0, 100, 200, 300, or 400")
	}
	if len(rule.Spec.Transformations) == 0 {
		return nil, fmt.Errorf("spec.transformations must contain at least one named transformation")
	}
	if err := transformationStringList(spec["conditions"], "spec.conditions"); err != nil {
		return nil, err
	}
	var transformations []json.RawMessage
	if err := json.Unmarshal(spec["transformations"], &transformations); err != nil {
		return nil, err
	}
	for i, transformation := range transformations {
		path := fmt.Sprintf("spec.transformations[%d]", i)
		fields, err := transformationRequiredFields(transformation, path, "name", "statements")
		if err != nil {
			return nil, err
		}
		if err := transformationStringList(fields["statements"], path+".statements"); err != nil {
			return nil, err
		}
	}
	if rule.Spec.Examples != nil {
		if len(*rule.Spec.Examples) > 10 {
			return nil, fmt.Errorf("spec.examples must contain at most 10 examples")
		}
		var examples []json.RawMessage
		if err := json.Unmarshal(spec["examples"], &examples); err != nil {
			return nil, err
		}
		for i, example := range examples {
			if _, err := transformationRequiredFields(example, fmt.Sprintf("spec.examples[%d]", i), "name", "otlp"); err != nil {
				return nil, err
			}
		}
	}
	return &rule, nil
}

func transformationStringList(data json.RawMessage, path string) error {
	if len(data) == 0 {
		return nil
	}
	var values []*string
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	for i, value := range values {
		if value == nil {
			return fmt.Errorf("%s[%d] must be a string", path, i)
		}
	}
	return nil
}

func transformationRequiredFields(data []byte, path string, names ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	for _, name := range names {
		if value := fields[name]; len(value) == 0 || bytes.Equal(value, []byte("null")) {
			return nil, fmt.Errorf("%s.%s is required", path, name)
		}
	}
	return fields, nil
}

func GetTelemetryTransformationRuleOrigin(rule *dash0api.TelemetryTransformationRuleDefinition) string {
	if rule.Metadata.Labels == nil || rule.Metadata.Labels.Dash0Comorigin == nil {
		return ""
	}
	return *rule.Metadata.Labels.Dash0Comorigin
}

func PrepareTelemetryTransformationRule(rule, stored *dash0api.TelemetryTransformationRuleDefinition, dataset *string) {
	dash0api.StripTelemetryTransformationRuleServerFields(rule)
	dash0api.ClearTelemetryTransformationRuleID(rule)
	datasetName := "default"
	if dataset != nil {
		datasetName = *dataset
	}
	dash0api.SetTelemetryTransformationRuleDataset(rule, datasetName)
	if stored != nil && stored.Metadata.Annotations != nil {
		if rule.Metadata.Annotations == nil {
			rule.Metadata.Annotations = &dash0api.TelemetryTransformationRuleAnnotations{}
		}
		// An exported token must remain stale rather than silently authorizing a newer version.
		if rule.Metadata.Annotations.Dash0ComupdatedAt == nil {
			rule.Metadata.Annotations.Dash0ComupdatedAt = stored.Metadata.Annotations.Dash0ComupdatedAt
		}
	}
}

func ImportTelemetryTransformationRule(ctx context.Context, apiClient dash0api.Client, rule *dash0api.TelemetryTransformationRuleDefinition, dataset *string) (ImportResult, error) {
	origin := GetTelemetryTransformationRuleOrigin(rule)
	key := origin
	if key == "" {
		key = dash0api.GetTelemetryTransformationRuleID(rule)
	}
	var before *dash0api.TelemetryTransformationRuleDefinition
	action := ActionCreated
	if key != "" {
		var err error
		before, err = apiClient.GetTelemetryTransformationRule(ctx, key, dataset)
		switch {
		case err == nil:
			action = ActionUpdated
		case !dash0api.IsNotFound(err):
			return ImportResult{}, err
		case origin == "":
			return ImportResult{}, fmt.Errorf("telemetry transformation rule with id %q not found:\n  %w\nHint: set %s to create a rule and manage it as code", key, err, OriginLabel)
		}
	}
	PrepareTelemetryTransformationRule(rule, before, dataset)
	var result *dash0api.TelemetryTransformationRuleDefinition
	var err error
	if before != nil {
		result, err = apiClient.UpdateTelemetryTransformationRule(ctx, key, rule, dataset)
	} else {
		result, err = apiClient.CreateTelemetryTransformationRule(ctx, rule, dataset)
		if origin != "" && dash0api.IsConflict(err) {
			// GET hides tombstones, but their origins stay reserved; only an explicit conflict permits recovery.
			before, err = apiClient.GetTelemetryTransformationRule(ctx, origin, dataset)
			switch {
			case err == nil:
				action = ActionUpdated
				PrepareTelemetryTransformationRule(rule, before, dataset)
			case dash0api.IsNotFound(err):
				action = ActionRestored
			default:
				return ImportResult{}, err
			}
			result, err = apiClient.UpdateTelemetryTransformationRule(ctx, origin, rule, dataset)
		}
	}
	if err != nil {
		return ImportResult{}, err
	}
	if result.Metadata.Annotations != nil {
		if err := RequireNotDeleted("Telemetry transformation rule", dash0api.GetTelemetryTransformationRuleName(result), dash0api.GetTelemetryTransformationRuleID(result), result.Metadata.Annotations.Dash0ComdeletedAt); err != nil {
			return ImportResult{}, err
		}
	}
	var beforeAny any
	if before != nil {
		beforeAny = before
	}
	return ImportResult{Name: dash0api.GetTelemetryTransformationRuleName(result), ID: dash0api.GetTelemetryTransformationRuleID(result), Action: action, Before: beforeAny, After: result}, nil
}
