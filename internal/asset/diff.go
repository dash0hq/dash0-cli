package asset

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

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
		// The API stores frequency in Go's canonical form ("30m0s"), so a
		// file that writes "30m" would otherwise never diff clean.
		if c.Spec.Frequency != nil {
			if frequency, err := time.ParseDuration(string(*c.Spec.Frequency)); err == nil {
				canonical := dash0api.Duration(frequency.String())
				c.Spec.Frequency = &canonical
			}
		}
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
	case *dash0api.AwsMetricScrapeConfigDefinition:
		var c dash0api.AwsMetricScrapeConfigDefinition
		if err := dash0yaml.Unmarshal(jsonBytes, &c); err != nil {
			return "", fmt.Errorf("failed to unmarshal AWS metric scrape config: %w", err)
		}
		normalizeAwsMetricScrapeConfigForDiff(&c)
		stripped = &c
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
	text, err := UnifiedDiff(displayKind, before, after)
	if err != nil {
		return err
	}

	if text == "" {
		fmt.Fprintf(w, "%s %q: no changes\n", displayKind, name)
		return nil
	}

	return WriteDiff(w, text)
}

// PrintChanges writes the unified diff between the before and after states of
// an asset to w, and writes nothing when they are the same.
func PrintChanges(w io.Writer, displayKind string, before, after any) error {
	text, err := UnifiedDiff(displayKind, before, after)
	if err != nil || text == "" {
		return err
	}
	return WriteDiff(w, text)
}

// semanticCompareConfig returns the per-kind options for dash0yaml.Equivalent
// and dash0yaml.Normalize, keyed off after's concrete type.
//
// Every annotation takes part in the comparison. marshalForDiff has already
// removed the server-managed ones through each kind's Strip*ServerFields, and
// any other annotation is user content, such as a synthetic check's
// dash0.com/folder-path or an SLO's dash0.com/display-name.
func semanticCompareConfig(asset any) (extraIgnoredFields, preservedAnnotationKeys []string, opts []dash0yaml.Option) {
	switch asset.(type) {
	case *dash0api.PrometheusAlertRule:
		return nil, nil, []dash0yaml.Option{dash0yaml.WithFlatDocument()}
	case *dash0api.RecordingRule:
		// The API does not store a recording rule CRD's annotations, so they
		// can never match the stored rule.
		return nil, nil, nil
	case *dash0api.NotificationChannelDefinition:
		// spec.routing.assets is API-managed; see RoutingAssetsWarning.
		return []string{"spec.routing.assets"}, nil, []dash0yaml.Option{dash0yaml.WithAnnotationsUnfiltered()}
	default:
		return nil, nil, []dash0yaml.Option{dash0yaml.WithAnnotationsUnfiltered()}
	}
}

// UnifiedDiff returns the unified diff between before and after, or "" when
// they are semantically the same. dash0 diff uses it to compare each update
// once and print the same text later with WriteDiff.
func UnifiedDiff(displayKind string, before, after any) (string, error) {
	after = withOmittedFieldsAsStored(before, after)
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

// WriteDiff writes text from UnifiedDiff to w, colorized unless color is off.
func WriteDiff(w io.Writer, text string) error {
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

// Server defaults the API stores when a write leaves the field out, even over
// a stored value that differs.
const (
	notificationChannelDefaultFrequency  = "10m0s"
	timeSeriesAggregationDefaultPriority = 2
)

// withOmittedFieldsAsStored returns after with each field it leaves out set to
// what the API stores for that write. The semantic comparison drops a stored
// value that after leaves out, so without this a write that resets the field
// to its default would show as no change, and a write that keeps the stored
// value would show as a removal.
func withOmittedFieldsAsStored(before, after any) any {
	switch stored := before.(type) {
	case *dash0api.PrometheusAlertRule:
		return withCheckRuleEnabledDefault(stored, after)
	case *dash0api.NotificationChannelDefinition:
		channel, ok := after.(*dash0api.NotificationChannelDefinition)
		if !ok || channel == nil || stored == nil || stored.Spec.Frequency == nil || channel.Spec.Frequency != nil {
			return after
		}
		withDefault := *channel
		frequency := notificationChannelDefaultFrequency
		withDefault.Spec.Frequency = &frequency
		return &withDefault
	case *dash0api.TimeSeriesAggregationDefinition:
		aggregation, ok := after.(*dash0api.TimeSeriesAggregationDefinition)
		if !ok || aggregation == nil || stored == nil || stored.Spec.Priority == nil || aggregation.Spec.Priority != nil {
			return after
		}
		withDefault := *aggregation
		priority := timeSeriesAggregationDefaultPriority
		withDefault.Spec.Priority = &priority
		return &withDefault
	case *dash0api.SkillDefinition:
		return withStoredSkillDefaultEnabled(stored, after)
	}
	return after
}

// withStoredSkillDefaultEnabled returns after with each grant that leaves
// defaultEnabled out set to the stored grant's value, because the API keeps
// that value on such a write.
func withStoredSkillDefaultEnabled(stored *dash0api.SkillDefinition, after any) any {
	skill, ok := after.(*dash0api.SkillDefinition)
	if !ok || skill == nil || skill.Spec.Permissions == nil || stored == nil || stored.Spec.Permissions == nil {
		return after
	}
	storedDefaults := map[string]*bool{}
	for _, grant := range *stored.Spec.Permissions {
		storedDefaults[permissionSortKey(grant.Role, grant.TeamId, grant.UserId)] = grant.DefaultEnabled
	}
	grants := make([]dash0api.SkillPermission, len(*skill.Spec.Permissions))
	copy(grants, *skill.Spec.Permissions)
	for i, grant := range grants {
		if grant.DefaultEnabled == nil {
			grants[i].DefaultEnabled = storedDefaults[permissionSortKey(grant.Role, grant.TeamId, grant.UserId)]
		}
	}
	withStored := *skill
	withStored.Spec.Permissions = &grants
	return &withStored
}

// withCheckRuleEnabledDefault returns after with the server default for
// enabled, true, when after leaves enabled out and before is a disabled rule.
// The API enables a rule whose PUT has no enabled field, while the semantic
// comparison drops a stored false that after leaves out, so the re-enabled
// rule would show as no change.
func withCheckRuleEnabledDefault(stored *dash0api.PrometheusAlertRule, after any) any {
	if stored == nil || stored.Enabled == nil || *stored.Enabled {
		return after
	}
	rule, ok := after.(*dash0api.PrometheusAlertRule)
	if !ok || rule == nil || rule.Enabled != nil {
		return after
	}
	withDefault := *rule
	enabled := true
	withDefault.Enabled = &enabled
	return &withDefault
}
