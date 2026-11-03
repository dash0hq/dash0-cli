package asset

import (
	"fmt"
	"io"
	"os"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
	dashcolor "github.com/dash0hq/dash0-cli/internal/color"
	"github.com/muesli/termenv"
	"github.com/pmezard/go-difflib/difflib"
	sigsyaml "sigs.k8s.io/yaml"
)

// marshalForDiff deep-copies a typed asset, strips server-generated fields via
// the per-type Strip*ServerFields functions, and marshals the result to YAML.
func marshalForDiff(asset any) (string, error) {
	jsonBytes, err := sigsyaml.Marshal(asset)
	if err != nil {
		return "", fmt.Errorf("failed to marshal asset: %w", err)
	}

	var stripped any
	switch asset.(type) {
	case *dash0api.DashboardDefinition:
		var d dash0api.DashboardDefinition
		if err := dash0yaml.Unmarshal(jsonBytes, &d); err != nil {
			return "", fmt.Errorf("failed to unmarshal dashboard: %w", err)
		}
		dash0api.StripDashboardServerFields(&d)
		stripped = &d
	case *dash0api.PrometheusAlertRule:
		var r dash0api.PrometheusAlertRule
		if err := dash0yaml.Unmarshal(jsonBytes, &r); err != nil {
			return "", fmt.Errorf("failed to unmarshal check rule: %w", err)
		}
		dash0api.StripCheckRuleServerFields(&r)
		stripped = &r
	case *dash0api.ViewDefinition:
		var v dash0api.ViewDefinition
		if err := dash0yaml.Unmarshal(jsonBytes, &v); err != nil {
			return "", fmt.Errorf("failed to unmarshal view: %w", err)
		}
		dash0api.StripViewServerFields(&v)
		SortViewPermissions(&v)
		stripped = &v
	case *dash0api.SyntheticCheckDefinition:
		var c dash0api.SyntheticCheckDefinition
		if err := dash0yaml.Unmarshal(jsonBytes, &c); err != nil {
			return "", fmt.Errorf("failed to unmarshal synthetic check: %w", err)
		}
		dash0api.StripSyntheticCheckServerFields(&c)
		SortSyntheticCheckPermissions(&c)
		stripped = &c
	case *dash0api.SpamFilter:
		var s dash0api.SpamFilter
		if err := dash0yaml.Unmarshal(jsonBytes, &s); err != nil {
			return "", fmt.Errorf("failed to unmarshal spam filter: %w", err)
		}
		dash0api.StripSpamFilterServerFields(&s)
		stripped = &s
	case *dash0api.SpamFilterV1Alpha2:
		var s dash0api.SpamFilterV1Alpha2
		if err := dash0yaml.Unmarshal(jsonBytes, &s); err != nil {
			return "", fmt.Errorf("failed to unmarshal spam filter: %w", err)
		}
		// No v1alpha2-specific Strip helper exists yet; the v1alpha1 helper
		// only touches metadata, so the inputs we control here are already
		// server-field-free. If a Strip*V1Alpha2 helper lands later, plug
		// it in here.
		stripped = &s
	case *dash0api.NotificationChannelDefinition:
		var c dash0api.NotificationChannelDefinition
		if err := dash0yaml.Unmarshal(jsonBytes, &c); err != nil {
			return "", fmt.Errorf("failed to unmarshal notification channel: %w", err)
		}
		dash0api.StripNotificationChannelServerFields(&c)
		stripped = &c
	case *dash0api.DatasetDefinition:
		var d dash0api.DatasetDefinition
		if err := dash0yaml.Unmarshal(jsonBytes, &d); err != nil {
			return "", fmt.Errorf("failed to unmarshal dataset: %w", err)
		}
		dash0api.StripDatasetServerFields(&d)
		SortDatasetPermissions(&d)
		stripped = &d
	case *dash0api.SkillDefinition:
		var sk dash0api.SkillDefinition
		if err := dash0yaml.Unmarshal(jsonBytes, &sk); err != nil {
			return "", fmt.Errorf("failed to unmarshal Agent0 skill: %w", err)
		}
		dash0api.StripCustomSkillServerFields(&sk)
		SortCustomSkillPermissions(&sk)
		stripped = &sk
	case *dash0api.RecordingRule:
		var r dash0api.RecordingRule
		if err := dash0yaml.Unmarshal(jsonBytes, &r); err != nil {
			return "", fmt.Errorf("failed to unmarshal recording rule: %w", err)
		}
		dash0api.StripRecordingRuleServerFields(&r)
		stripped = &r
	case *dash0api.TeamDefinitionV1Alpha1:
		var t dash0api.TeamDefinitionV1Alpha1
		if err := dash0yaml.Unmarshal(jsonBytes, &t); err != nil {
			return "", fmt.Errorf("failed to unmarshal team: %w", err)
		}
		dash0api.StripTeamServerFields(&t)
		stripped = &t
	case *dash0api.SloDefinition:
		var s dash0api.SloDefinition
		if err := dash0yaml.Unmarshal(jsonBytes, &s); err != nil {
			return "", fmt.Errorf("failed to unmarshal SLO: %w", err)
		}
		// Without this the SLO fell through to the default branch and neither
		// side of the diff was normalized. Both `before` (from GetSLO) and
		// `after` (the PUT response) carry server-managed metadata, and the
		// server bumps dash0.com/updated-at (and dash0.com/version) on every
		// PUT — even for a byte-identical body. Re-applying an unchanged
		// document therefore rendered a spurious updated-at/version diff
		// instead of "no changes". Stripping both sides makes the comparison
		// reflect only the user-authored document.
		dash0api.StripSLOServerFields(&s)
		stripped = &s
	case *dash0api.TimeSeriesAggregationDefinition:
		var a dash0api.TimeSeriesAggregationDefinition
		if err := dash0yaml.Unmarshal(jsonBytes, &a); err != nil {
			return "", fmt.Errorf("failed to unmarshal time series aggregation: %w", err)
		}
		// Without this the diff churns on dash0.com/version, which the server
		// increments on every PUT, so a reapply of unchanged content would
		// never report "no changes".
		dash0api.StripTimeSeriesAggregationServerFields(&a)
		// Clear the id too, which the strip keeps on purpose. The upsert key
		// here is the origin, so a hand-authored file never carries an id, and
		// keeping it would show up as a phantom removal in `update --dry-run`.
		dash0api.ClearTimeSeriesAggregationID(&a)
		stripped = &a
	case *dash0api.AuthTokenDefinition, *dash0api.AuthTokenResponseDefinition:
		// Decoding into the request type drops spec.token, so a diff can never
		// show the secret, even if a caller passes a response that still holds it.
		var t dash0api.AuthTokenDefinition
		if err := dash0yaml.Unmarshal(jsonBytes, &t); err != nil {
			return "", fmt.Errorf("failed to unmarshal auth token: %w", err)
		}
		dash0api.StripAuthTokenServerFields(&t)
		stripped = &t
	default:
		stripped = asset
	}

	out, err := sigsyaml.Marshal(stripped)
	if err != nil {
		return "", fmt.Errorf("failed to marshal asset for diff: %w", err)
	}
	return string(out), nil
}

// PrintDiff computes a unified diff between the before and after states of an
// asset and writes it to w. If there are no changes, a "no changes" message is
// printed instead.
func PrintDiff(w io.Writer, displayKind, name string, before, after any) error {
	text, err := unifiedDiff(displayKind, before, after)
	if err != nil {
		return err
	}

	if text == "" {
		fmt.Fprintf(w, "%s %q: no changes\n", displayKind, name)
		return nil
	}

	return writeDiff(w, text)
}

// PrintChanges writes the unified diff between the before and after states of
// an asset to w, and writes nothing when they are the same.
func PrintChanges(w io.Writer, displayKind string, before, after any) error {
	text, err := unifiedDiff(displayKind, before, after)
	if err != nil || text == "" {
		return err
	}
	return writeDiff(w, text)
}

// semanticCompareConfig returns the per-kind options for dash0yaml.Equivalent
// and dash0yaml.Normalize, keyed off after's concrete type.
//
// *dash0api.PrometheusAlertRule is flat (top-level annotations, no metadata:
// nesting), and its annotations hold user content (summary, description, the
// bare "sharing" key) rather than server provenance, so it compares them
// unfiltered. Every other kind is metadata-nested and keeps only the
// user-settable annotations listed here.
func semanticCompareConfig(asset any) (extraIgnoredFields, preservedAnnotationKeys []string, opts []dash0yaml.Option) {
	switch asset.(type) {
	case *dash0api.DashboardDefinition, *dash0api.ViewDefinition:
		return nil, []string{dash0api.AnnotationSharing, dash0api.AnnotationFolderPath}, nil
	case *dash0api.SloDefinition:
		return nil, []string{dash0api.AnnotationSharing, dash0api.AnnotationFolderPath, "dash0.com/enabled"}, nil
	case *dash0api.PrometheusAlertRule:
		return nil, nil, []dash0yaml.Option{dash0yaml.WithAnnotationsRoot(""), dash0yaml.WithAnnotationsUnfiltered()}
	case *dash0api.SyntheticCheckDefinition:
		return nil, []string{dash0api.AnnotationSharing}, nil
	case *dash0api.NotificationChannelDefinition:
		// spec.routing.assets is API-managed; see RoutingAssetsWarning.
		return []string{"spec.routing.assets"}, nil, nil
	default:
		return nil, nil, nil
	}
}

// UnifiedDiff returns the unified diff between before and after, or "" when
// they are semantically the same. dash0 diff uses it to compare each update
// once and print the same text later with WriteDiff.
func UnifiedDiff(displayKind string, before, after any) (string, error) {
	return unifiedDiff(displayKind, before, after)
}

// WriteDiff writes text from UnifiedDiff to w, colorized unless color is off.
func WriteDiff(w io.Writer, text string) error {
	return writeDiff(w, text)
}

func unifiedDiff(displayKind string, before, after any) (string, error) {
	beforeYAML, err := marshalForDiff(before)
	if err != nil {
		return "", fmt.Errorf("failed to marshal before state: %w", err)
	}

	afterYAML, err := marshalForDiff(after)
	if err != nil {
		return "", fmt.Errorf("failed to marshal after state: %w", err)
	}

	// Compare semantically: key and slice order, duration formatting
	// ("2m" vs "2m0s"), int-vs-float, empty containers, and server-populated
	// defaults absent from after are not changes.
	extraIgnored, preserved, opts := semanticCompareConfig(after)
	extraIgnored = append(extraIgnored, dash0yaml.AbsentFields([]byte(afterYAML), dash0yaml.ConditionallyIgnoredFields())...)
	// after is the reference: Equivalent skips server-added fields only when
	// they are absent from the reference, and diff's after is the local file.
	equivalent, err := dash0yaml.Equivalent([]byte(afterYAML), []byte(beforeYAML), extraIgnored, preserved, opts...)
	if err != nil {
		return "", fmt.Errorf("failed to compare semantic equivalence: %w", err)
	}
	if equivalent {
		return "", nil
	}

	// Render from normalized YAML so ignored fields do not clutter the diff.
	// Slice order and duration formatting are only tolerated by the
	// comparator, so a real change next to one of those can still show it
	// as extra noise; it never hides a real change.
	normalizedAfter, normalizedBefore, err := dash0yaml.NormalizePair([]byte(afterYAML), []byte(beforeYAML), extraIgnored, preserved, opts...)
	if err != nil {
		return "", fmt.Errorf("failed to normalize states: %w", err)
	}

	diff := difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(normalizedBefore)),
		B:        difflib.SplitLines(string(normalizedAfter)),
		FromFile: fmt.Sprintf("%s (before)", displayKind),
		ToFile:   fmt.Sprintf("%s (after)", displayKind),
		Context:  3,
	}

	text, err := difflib.GetUnifiedDiffString(diff)
	if err != nil {
		return "", fmt.Errorf("failed to compute diff: %w", err)
	}
	return text, nil
}

func writeDiff(w io.Writer, text string) error {
	if dashcolor.NoColor {
		_, err := io.WriteString(w, text)
		return err
	}
	return writeColorizedDiff(w, text)
}

func writeColorizedDiff(w io.Writer, text string) error {
	o := termenv.NewOutput(os.Stdout)
	for _, line := range strings.Split(text, "\n") {
		if line == "" {
			continue
		}
		var styled string
		switch {
		case strings.HasPrefix(line, "---"), strings.HasPrefix(line, "+++"):
			styled = o.String(line).Bold().String()
		case strings.HasPrefix(line, "@@"):
			styled = o.String(line).Foreground(o.Color("6")).String() // cyan
		case strings.HasPrefix(line, "-"):
			styled = o.String(line).Foreground(o.Color("1")).String() // red
		case strings.HasPrefix(line, "+"):
			styled = o.String(line).Foreground(o.Color("2")).String() // green
		default:
			styled = line
		}
		if _, err := fmt.Fprintln(w, styled); err != nil {
			return err
		}
	}
	return nil
}
