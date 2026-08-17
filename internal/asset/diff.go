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
		if err := sigsyaml.Unmarshal(jsonBytes, &d); err != nil {
			return "", fmt.Errorf("failed to unmarshal dashboard: %w", err)
		}
		dash0api.StripDashboardServerFields(&d)
		stripped = &d
	case *dash0api.PrometheusAlertRule:
		var r dash0api.PrometheusAlertRule
		if err := sigsyaml.Unmarshal(jsonBytes, &r); err != nil {
			return "", fmt.Errorf("failed to unmarshal check rule: %w", err)
		}
		dash0api.StripCheckRuleServerFields(&r)
		stripped = &r
	case *dash0api.ViewDefinition:
		var v dash0api.ViewDefinition
		if err := sigsyaml.Unmarshal(jsonBytes, &v); err != nil {
			return "", fmt.Errorf("failed to unmarshal view: %w", err)
		}
		dash0api.StripViewServerFields(&v)
		SortViewPermissions(&v)
		stripped = &v
	case *dash0api.SyntheticCheckDefinition:
		var c dash0api.SyntheticCheckDefinition
		if err := sigsyaml.Unmarshal(jsonBytes, &c); err != nil {
			return "", fmt.Errorf("failed to unmarshal synthetic check: %w", err)
		}
		dash0api.StripSyntheticCheckServerFields(&c)
		SortSyntheticCheckPermissions(&c)
		stripped = &c
	case *dash0api.SpamFilter:
		var s dash0api.SpamFilter
		if err := sigsyaml.Unmarshal(jsonBytes, &s); err != nil {
			return "", fmt.Errorf("failed to unmarshal spam filter: %w", err)
		}
		dash0api.StripSpamFilterServerFields(&s)
		stripped = &s
	case *dash0api.SpamFilterV1Alpha2:
		var s dash0api.SpamFilterV1Alpha2
		if err := sigsyaml.Unmarshal(jsonBytes, &s); err != nil {
			return "", fmt.Errorf("failed to unmarshal spam filter: %w", err)
		}
		// No v1alpha2-specific Strip helper exists yet; the v1alpha1 helper
		// only touches metadata, so the inputs we control here are already
		// server-field-free. If a Strip*V1Alpha2 helper lands later, plug
		// it in here.
		stripped = &s
	case *dash0api.NotificationChannelDefinition:
		var c dash0api.NotificationChannelDefinition
		if err := sigsyaml.Unmarshal(jsonBytes, &c); err != nil {
			return "", fmt.Errorf("failed to unmarshal notification channel: %w", err)
		}
		dash0api.StripNotificationChannelServerFields(&c)
		stripped = &c
	case *dash0api.RecordingRule:
		var r dash0api.RecordingRule
		if err := sigsyaml.Unmarshal(jsonBytes, &r); err != nil {
			return "", fmt.Errorf("failed to unmarshal recording rule: %w", err)
		}
		dash0api.StripRecordingRuleServerFields(&r)
		stripped = &r
	case *dash0api.TeamDefinitionV1Alpha1:
		var t dash0api.TeamDefinitionV1Alpha1
		if err := sigsyaml.Unmarshal(jsonBytes, &t); err != nil {
			return "", fmt.Errorf("failed to unmarshal team: %w", err)
		}
		dash0api.StripTeamServerFields(&t)
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

// semanticCompareConfig returns the per-kind configuration for
// dash0yaml.Equivalent/Normalize, keyed off after's concrete type (mirroring
// marshalForDiff's own type switch). preservedAnnotationKeys reuses the
// already-exported dash0api.AnnotationSharing/AnnotationFolderPath constants
// rather than redefining them.
//
// *dash0api.PrometheusAlertRule needs two options, unlike every other kind
// here (which is Kubernetes-CRD-shaped, metadata:-nested, and uses the
// default root with dash0api.AnnotationSharing/AnnotationFolderPath as
// preserved keys):
//   - dash0yaml.WithAnnotationsRoot(""): it is flat (top-level
//     id/name/expression/labels/annotations, no "metadata:" nesting),
//     whether it originated from a native CheckRule document or was
//     extracted from a PrometheusRule CRD's alerting rules
//     (asset.ParseCheckRules produces the same flat wire type either way).
//   - dash0yaml.WithAnnotationsUnfiltered(): its annotations map holds
//     genuine user content (summary, description, sharing) directly, not
//     server-managed-by-default provenance metadata -- confirmed via the
//     generated PrometheusAlertRule_Annotations type, whose Sharing field
//     has JSON tag "sharing", not "dash0.com/sharing". Without this, an
//     empty preservedAnnotationKeys would strip the whole map, silently
//     hiding real content and sharing changes from drift detection.
func semanticCompareConfig(asset any) (extraIgnoredFields, preservedAnnotationKeys []string, opts []dash0yaml.Option) {
	switch asset.(type) {
	case *dash0api.DashboardDefinition:
		return nil, []string{dash0api.AnnotationSharing, dash0api.AnnotationFolderPath}, nil
	case *dash0api.PrometheusAlertRule:
		return nil, nil, []dash0yaml.Option{dash0yaml.WithAnnotationsRoot(""), dash0yaml.WithAnnotationsUnfiltered()}
	case *dash0api.ViewDefinition:
		return nil, []string{dash0api.AnnotationSharing, dash0api.AnnotationFolderPath}, nil
	case *dash0api.SyntheticCheckDefinition:
		return nil, []string{dash0api.AnnotationSharing}, nil
	case *dash0api.NotificationChannelDefinition:
		// spec.routing.assets is API-managed (a server-derived back-reference);
		// see RoutingAssetsWarning and CLAUDE.md's Asset annotations section.
		return []string{"spec.routing.assets"}, nil, nil
	default:
		// RecordingRule, SpamFilter, SpamFilterV1Alpha2, TeamDefinitionV1Alpha1:
		// metadata-nested (default root), no preserved annotations.
		return nil, nil, nil
	}
}

// semanticEquivalent reports whether beforeYAML and afterYAML (already
// stripped and marshaled by marshalForDiff) are semantically equivalent for
// drift-detection purposes: ignoring server-managed metadata, empty
// containers, non-preserved annotations, slice order, duration-string
// formatting, and numeric type differences. after's concrete type selects
// the per-kind comparison config via semanticCompareConfig; afterYAML is
// also the reference document for ConditionallyIgnoredFields (fields only
// ignored when the local/proposed definition doesn't set them).
func semanticEquivalent(after any, beforeYAML, afterYAML string) (bool, error) {
	extraIgnored, preserved, opts := semanticCompareConfig(after)
	extraIgnored = append(extraIgnored, dash0yaml.AbsentFields([]byte(afterYAML), dash0yaml.ConditionallyIgnoredFields)...)
	return dash0yaml.Equivalent([]byte(beforeYAML), []byte(afterYAML), extraIgnored, preserved, opts...)
}

// HasDifference reports whether before and after are semantically different
// for drift-detection purposes (see semanticEquivalent). It lets a caller
// that needs to count pending changes (e.g. `dash0 diff`) do so without
// re-implementing PrintDiff's comparison.
func HasDifference(before, after any) (bool, error) {
	beforeYAML, err := marshalForDiff(before)
	if err != nil {
		return false, fmt.Errorf("failed to marshal before state: %w", err)
	}
	afterYAML, err := marshalForDiff(after)
	if err != nil {
		return false, fmt.Errorf("failed to marshal after state: %w", err)
	}
	equivalent, err := semanticEquivalent(after, beforeYAML, afterYAML)
	if err != nil {
		return false, fmt.Errorf("failed to compare semantic equivalence: %w", err)
	}
	return !equivalent, nil
}

// PrintDiff computes a unified diff between the before and after states of an
// asset and writes it to w. If the two are semantically equivalent (see
// semanticEquivalent), a "no changes" message is printed instead of a diff.
//
// The diff itself is rendered from normalized YAML (server-managed metadata,
// empty containers, and non-preserved annotations stripped), not the raw
// stripped YAML, so ignored-field noise doesn't clutter the output. Two
// equivalences the comparison honors are not reflected in the rendered
// text, since they exist only inside the comparator, not as an output
// transform: slice element order and duration-string formatting (e.g. "2m"
// vs "2m0s"). So a real change that coexists with an unrelated reordering or
// duration-format difference elsewhere in the same document may show that
// difference as extra noise alongside the real one -- this never produces a
// false "no changes" or misses a real change, which is what matters for
// drift detection; it is a rendering-only rough edge.
func PrintDiff(w io.Writer, displayKind, name string, before, after any) error {
	beforeYAML, err := marshalForDiff(before)
	if err != nil {
		return fmt.Errorf("failed to marshal before state: %w", err)
	}

	afterYAML, err := marshalForDiff(after)
	if err != nil {
		return fmt.Errorf("failed to marshal after state: %w", err)
	}

	extraIgnored, preserved, opts := semanticCompareConfig(after)
	extraIgnored = append(extraIgnored, dash0yaml.AbsentFields([]byte(afterYAML), dash0yaml.ConditionallyIgnoredFields)...)

	equivalent, err := dash0yaml.Equivalent([]byte(beforeYAML), []byte(afterYAML), extraIgnored, preserved, opts...)
	if err != nil {
		return fmt.Errorf("failed to compare semantic equivalence: %w", err)
	}
	if equivalent {
		fmt.Fprintf(w, "%s %q: no changes\n", displayKind, name)
		return nil
	}

	normalizedBefore, err := dash0yaml.Normalize([]byte(beforeYAML), extraIgnored, preserved, opts...)
	if err != nil {
		return fmt.Errorf("failed to normalize before state: %w", err)
	}
	normalizedAfter, err := dash0yaml.Normalize([]byte(afterYAML), extraIgnored, preserved, opts...)
	if err != nil {
		return fmt.Errorf("failed to normalize after state: %w", err)
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
		return fmt.Errorf("failed to compute diff: %w", err)
	}

	if text == "" {
		// Equivalent() already reported a real difference, so this should
		// be unreachable in practice -- Normalize doesn't erase anything
		// Equivalent's own comparator additionally tolerates (slice order,
		// duration formatting) that could otherwise fully explain a
		// "different" verdict with no visible text diff. Kept as a safe
		// fallback rather than an assumption.
		fmt.Fprintf(w, "%s %q: no changes\n", displayKind, name)
		return nil
	}

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
