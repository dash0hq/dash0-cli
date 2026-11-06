package signaltometricsrules

import (
	"fmt"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
	"github.com/dash0hq/dash0-cli/internal/asset"
)

// assetType is the display name used in error messages and ErrorContext.
const assetType = "signal-to-metrics rule"

// displayKind is the display name used in success messages and diffs, per the
// asset kind display-name table in docs/cli-naming-conventions.md.
const displayKind = asset.SignalToMetricsRuleDisplayKind

// decode parses a YAML or JSON signal-to-metrics rule document.
func decode(raw []byte) (*dash0api.SignalToMetricsDefinition, error) {
	var rule dash0api.SignalToMetricsDefinition
	if err := dash0yaml.Unmarshal(raw, &rule); err != nil {
		return nil, fmt.Errorf("failed to parse signal-to-metrics rule definition: %w", err)
	}
	return &rule, nil
}

// origin aliases the asset helper so the command files read the same way they
// do for the other labels.
func origin(rule *dash0api.SignalToMetricsDefinition) string {
	return asset.GetSignalToMetricsRuleOrigin(rule)
}

// signal renders spec.match.signal ("logs" or "spans").
func signal(rule *dash0api.SignalToMetricsDefinition) string {
	if rule == nil {
		return ""
	}
	return string(rule.Spec.Match.Signal)
}

// metricName renders spec.output.name, the name of the metric the rule produces.
func metricName(rule *dash0api.SignalToMetricsDefinition) string {
	if rule == nil {
		return ""
	}
	return rule.Spec.Output.Name
}
