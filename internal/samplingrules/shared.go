package samplingrules

import (
	"fmt"
	"io"
	"strconv"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/asset"
)

// assetType is the display name used in error messages and ErrorContext.
const assetType = "sampling rule"

// displayKind is the display name used in success messages and diffs, per the
// asset kind display-name table in docs/cli-naming-conventions.md.
const displayKind = asset.SamplingRuleDisplayKind

// origin aliases the asset helper so the command files read the same way they
// do for the other labels.
func origin(rule *dash0api.SamplingDefinition) string {
	return asset.GetSamplingRuleOrigin(rule)
}

// rateLimit renders spec.rateLimit.rate, the most traces per minute the rule
// keeps, or "" when the rule has no rate limit.
func rateLimit(rule *dash0api.SamplingDefinition) string {
	if rule == nil || rule.Spec.RateLimit == nil {
		return ""
	}
	return strconv.Itoa(rule.Spec.RateLimit.Rate)
}

// printWarnings writes each warning on its own "warning:" line.
func printWarnings(w io.Writer, warnings []string) {
	for _, warning := range warnings {
		fmt.Fprintf(w, "warning: %s\n", warning)
	}
}
