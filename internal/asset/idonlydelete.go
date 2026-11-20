package asset

import "fmt"

// idOnlyDeleteWarning is what `apply --since` needs for a kind whose live asset
// can sit at a different id than the one a document recorded: an id-only
// document applied where that id is unknown makes the server assign a new one.
type idOnlyDeleteWarning struct {
	// noun names the kind in the error for a document that fails to decode.
	noun       string
	usesOrigin func(data []byte) (bool, error)
	// format takes the asset's name and id, as asset.FormatNameAndID prints them.
	format string
}

// idOnlyDeleteWarnings maps every normalized kind to its warning, or to nil
// when the kind needs none. A test requires a key for every kind in ValidKinds.
var idOnlyDeleteWarnings = map[string]*idOnlyDeleteWarning{
	"slo": {
		noun:       "SLO",
		usesOrigin: SLOUsesOrigin,
		format:     "warning: SLO %s was identified by dash0.com/id alone; --since deletes it at that id, or at its inferred origin when the id is not found, but an SLO that an earlier dash0 version created by POST sits at a server-assigned id, and this delete misses it; set dash0.com/origin on SLOs you manage with --since",
	},
	"generativeaievaluation": {
		noun:       "eval",
		usesOrigin: GenerativeAiEvaluationUsesOrigin,
		format:     "warning: eval %s was identified by dash0.com/id alone; --since deletes it at that id, or at its inferred origin when the id is not found, but an eval that an earlier dash0 version created by POST sits at a server-assigned id, and this delete misses it; set dash0.com/origin on evals you manage with --since",
	},
	"signaltometrics": {
		noun:       "signal-to-metrics rule",
		usesOrigin: SignalToMetricsRuleUsesOrigin,
		format:     "warning: signal-to-metrics rule %s was identified by dash0.com/id alone; --since deletes it at that id, or at its inferred origin when the id is not found, but a signal-to-metrics rule that an earlier dash0 version created by POST sits at a server-assigned id, and this delete misses it; set dash0.com/origin on signal-to-metrics rules you manage with --since",
	},
	"sampling": {
		noun:       "sampling rule",
		usesOrigin: SamplingRuleUsesOrigin,
		format:     "warning: sampling rule %s was identified by dash0.com/id alone; --since deletes it at that id, or at its inferred origin when the id is not found, but a sampling rule that an earlier dash0 version created by POST sits at a server-assigned id, and this delete misses it; set dash0.com/origin on sampling rules you manage with --since",
	},
	"team": {
		noun:       "team",
		usesOrigin: TeamUsesOrigin,
		format:     "warning: team %s was identified by dash0.com/id alone; --since deletes it at that id, or at its inferred origin when the id is not found, but a team that an earlier dash0 version created by POST sits at a server-assigned id, and this delete misses it; set dash0.com/origin on teams you manage with --since",
	},
	"spamfilter": {
		noun:       "spam filter",
		usesOrigin: SpamFilterUsesOrigin,
		format:     "warning: spam filter %s was identified by dash0.com/id alone; --since deletes it at that id, or at its inferred origin when the id is not found, but a spam filter that an earlier dash0 version created by POST sits at a server-assigned id, and this delete misses it; set dash0.com/origin on spam filters you manage with --since",
	},
	"dashboard":             nil,
	"checkrule":             nil,
	"syntheticcheck":        nil,
	"view":                  nil,
	"prometheusrule":        nil,
	"persesdashboard":       nil,
	"ephemeraldashboard":    nil,
	"notificationchannel":   nil,
	"timeseriesaggregation": nil,
	"dataset":               nil,
	"skill":                 nil,
	"awsmetricscrapeconfig": nil,
}

// IdentifiedByIDOnly reports whether a document of a kind that needs the
// id-only delete warning carries no dash0.com/origin label. It is false for
// every other kind.
func IdentifiedByIDOnly(kind string, data []byte) (bool, error) {
	warning := idOnlyDeleteWarnings[NormalizeKind(kind)]
	if warning == nil {
		return false, nil
	}
	usesOrigin, err := warning.usesOrigin(data)
	if err != nil {
		return false, fmt.Errorf("failed to determine %s identifier source: %w", warning.noun, err)
	}
	return !usesOrigin, nil
}

// IDOnlyDeleteWarning returns the warning `apply --since` prints before it
// deletes an asset that IdentifiedByIDOnly reported. display is the asset's
// name and id.
func IDOnlyDeleteWarning(kind, display string) string {
	warning := idOnlyDeleteWarnings[NormalizeKind(kind)]
	if warning == nil {
		return ""
	}
	return fmt.Sprintf(warning.format, display)
}
