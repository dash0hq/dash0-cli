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
		format:     "warning: SLO %s was identified by dash0.com/id alone; SLO ids are server-assigned, so if this document was first applied to an organization without that id the live SLO now sits at a different one (see docs/commands.md's asset-identifiers section) and this delete may miss it",
	},
	"generativeaievaluation": {
		noun:       "eval",
		usesOrigin: GenerativeAiEvaluationUsesOrigin,
		format:     "warning: eval %s was identified by dash0.com/id alone; eval ids are server-assigned, so if this document was first applied to an organization without that id the live eval now sits at a different one (see docs/commands.md's asset-identifiers section) and this delete may miss it",
	},
	"signaltometrics": {
		noun:       "signal-to-metrics rule",
		usesOrigin: SignalToMetricsRuleUsesOrigin,
		format:     "warning: signal-to-metrics rule %s was identified by dash0.com/id alone; rule ids are server-assigned, so if this document was first applied to an organization without that id the live rule now sits at a different one (see docs/commands.md's asset-identifiers section) and this delete may miss it",
	},
	"sampling": {
		noun:       "sampling rule",
		usesOrigin: SamplingRuleUsesOrigin,
		format:     "warning: sampling rule %s was identified by dash0.com/id alone; if this document was first applied to an organization without that id, the live rule got a new id from the server (see docs/commands.md's asset-identifiers section) and this delete may miss it",
	},
	"team": {
		noun:       "team",
		usesOrigin: TeamUsesOrigin,
		format:     "warning: team %s was identified by dash0.com/id alone; team ids are server-assigned, so if this document was first applied to an organization without that id the live team now sits at a different one (see docs/commands.md's asset-identifiers section) and this delete may miss it",
	},
	"spamfilter": {
		noun:       "spam filter",
		usesOrigin: SpamFilterUsesOrigin,
		format:     "warning: spam filter %s was identified by dash0.com/id alone; its live id may have been reassigned by the server since this identifier was recorded (see docs/commands.md's asset-identifiers section), so this delete may miss the actual live filter",
	},
	"dashboard":                        nil,
	"checkrule":                        nil,
	"syntheticcheck":                   nil,
	"view":                             nil,
	"prometheusrule":                   nil,
	"persesdashboard":                  nil,
	"notificationchannel":              nil,
	"timeseriesaggregation":            nil,
	"dataset":                          nil,
	"skill":                            nil,
	"awsmetricscrapeconfig":            nil,
	"awsloggroupinstrumentationpolicy": nil,
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
