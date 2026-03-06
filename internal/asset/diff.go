package asset

import (
	"fmt"
	"io"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/fatih/color"
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
		StripDashboardServerFields(&d)
		stripped = &d
	case *dash0api.PrometheusAlertRule:
		var r dash0api.PrometheusAlertRule
		if err := sigsyaml.Unmarshal(jsonBytes, &r); err != nil {
			return "", fmt.Errorf("failed to unmarshal check rule: %w", err)
		}
		StripCheckRuleServerFields(&r)
		stripped = &r
	case *dash0api.ViewDefinition:
		var v dash0api.ViewDefinition
		if err := sigsyaml.Unmarshal(jsonBytes, &v); err != nil {
			return "", fmt.Errorf("failed to unmarshal view: %w", err)
		}
		StripViewServerFields(&v)
		stripped = &v
	case *dash0api.SyntheticCheckDefinition:
		var c dash0api.SyntheticCheckDefinition
		if err := sigsyaml.Unmarshal(jsonBytes, &c); err != nil {
			return "", fmt.Errorf("failed to unmarshal synthetic check: %w", err)
		}
		StripSyntheticCheckServerFields(&c)
		stripped = &c
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
	beforeYAML, err := marshalForDiff(before)
	if err != nil {
		return fmt.Errorf("failed to marshal before state: %w", err)
	}

	afterYAML, err := marshalForDiff(after)
	if err != nil {
		return fmt.Errorf("failed to marshal after state: %w", err)
	}

	diff := difflib.UnifiedDiff{
		A:        difflib.SplitLines(beforeYAML),
		B:        difflib.SplitLines(afterYAML),
		FromFile: fmt.Sprintf("%s (before)", displayKind),
		ToFile:   fmt.Sprintf("%s (after)", displayKind),
		Context:  3,
	}

	text, err := difflib.GetUnifiedDiffString(diff)
	if err != nil {
		return fmt.Errorf("failed to compute diff: %w", err)
	}

	if text == "" {
		fmt.Fprintf(w, "%s %q: no changes\n", displayKind, name)
		return nil
	}

	if color.NoColor {
		_, err := io.WriteString(w, text)
		return err
	}

	return writeColorizedDiff(w, text)
}

var (
	colorRed   = color.New(color.FgRed)
	colorGreen = color.New(color.FgGreen)
	colorCyan  = color.New(color.FgCyan)
	colorBold  = color.New(color.Bold)
)

func writeColorizedDiff(w io.Writer, text string) error {
	for _, line := range strings.Split(text, "\n") {
		if line == "" {
			continue
		}
		var err error
		switch {
		case strings.HasPrefix(line, "---"), strings.HasPrefix(line, "+++"):
			_, err = colorBold.Fprintln(w, line)
		case strings.HasPrefix(line, "@@"):
			_, err = colorCyan.Fprintln(w, line)
		case strings.HasPrefix(line, "-"):
			_, err = colorRed.Fprintln(w, line)
		case strings.HasPrefix(line, "+"):
			_, err = colorGreen.Fprintln(w, line)
		default:
			_, err = fmt.Fprintln(w, line)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
