package spamfilters

import (
	"context"
	"fmt"
	"os"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/dash0hq/dash0-cli/internal/output"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var flags asset.ListFlags

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "[experimental] List spam filters",
		Long: `List all spam filters in the specified dataset. Both v1alpha1 (spec.contexts as
an array) and v1alpha2 (spec.context as a scalar) definitions are returned in
their native shape.` + internal.CONFIG_HINT,
		Example: `  # List spam filters (default: up to 50)
  dash0 --experimental spam-filters list

  # Output as YAML for backup or version control
  dash0 --experimental spam-filters list -o yaml > spam-filters.yaml

  # Output as JSON for scripting
  dash0 --experimental spam-filters list -o json

  # Output as CSV (pipe-friendly)
  dash0 --experimental spam-filters list -o csv

  # List without the header row (pipe-friendly)
  dash0 --experimental spam-filters list --skip-header`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runList(cmd.Context(), &flags)
		},
	}

	asset.RegisterListFlags(cmd, &flags)
	return cmd
}

func runList(ctx context.Context, flags *asset.ListFlags) error {
	if err := output.ValidateSkipHeader(flags.SkipHeader, flags.Output); err != nil {
		return err
	}

	apiUrl := client.ResolveApiUrl(ctx, flags.ApiUrl)
	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)
	items, err := apiClient.ListSpamFilterObjects(ctx, dataset)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "spam filter",
		})
	}

	if !flags.All && flags.Limit > 0 && len(items) > flags.Limit {
		items = items[:flags.Limit]
	}

	format, err := output.ParseFormat(flags.Output)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(format, os.Stdout, output.WithSkipHeader(flags.SkipHeader))

	switch format {
	case output.FormatJSON:
		return formatter.PrintJSON(toInterfaceSlice(items))
	case output.FormatYAML:
		return formatter.PrintMultiDocYAML(toInterfaceSlice(items))
	default:
		return printSpamFilterTable(formatter, items, format, apiUrl)
	}
}

func toInterfaceSlice(items []dash0api.SpamFilterObject) []interface{} {
	result := make([]interface{}, len(items))
	for i, f := range items {
		result[i] = f
	}
	return result
}

// renderContext renders the signal-type context column for either schema:
// v1alpha1 has spec.contexts (a comma-separated list of signal types),
// v1alpha2 has spec.context (a single signal type).
func renderContext(obj dash0api.SpamFilterObject) string {
	switch v := obj.(type) {
	case *dash0api.SpamFilter:
		if len(v.Spec.Contexts) == 0 {
			return ""
		}
		parts := make([]string, len(v.Spec.Contexts))
		for i, c := range v.Spec.Contexts {
			parts[i] = string(c)
		}
		return strings.Join(parts, ", ")
	case *dash0api.SpamFilterV1Alpha2:
		return string(v.Spec.Context)
	default:
		return ""
	}
}

// renderFilters renders the spec.filter list as a single-line, human-readable
// string suitable for a table/CSV cell — criteria joined by ` && ` to convey
// that filter criteria are AND-combined by the server. Both schemas share the
// same FilterCriteria type, so the rendering does not depend on the apiVersion.
func renderFilters(obj dash0api.SpamFilterObject) string {
	parts := criterionStrings(obj)
	return strings.Join(parts, " && ")
}

// renderFiltersBlock renders the spec.filter list across multiple lines for
// the verbose `get` output: the first criterion sits on the caller-provided
// label line; each subsequent criterion goes on its own line, indented so the
// joining ` && ` sits under the label and the criterion text aligns with the
// first one. Returns a string with no leading whitespace and no trailing
// newline.
//
// labelWidth is the number of characters in the prefix the caller will print
// (e.g. len("Filters: ") == 9). It is used to compute the indent so the
// continuation `&&` lines up under the label.
func renderFiltersBlock(obj dash0api.SpamFilterObject, labelWidth int) string {
	parts := criterionStrings(obj)
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}
	// Continuation prefix: enough spaces so that "&& " starts at column
	// labelWidth-2 — i.e. the `&&` ends where the label's last char ends.
	// For labelWidth=9 ("Filters: "), this yields 6 spaces + "&& " = 9 chars
	// total, so the criterion text aligns with the first criterion.
	indent := labelWidth - 3
	if indent < 0 {
		indent = 0
	}
	prefix := strings.Repeat(" ", indent) + "&& "

	var b strings.Builder
	b.WriteString(parts[0])
	for _, p := range parts[1:] {
		b.WriteByte('\n')
		b.WriteString(prefix)
		b.WriteString(p)
	}
	return b.String()
}

// criterionStrings formats each spec.filter entry as `key <op> value` (or
// multi-value variants). Shared by the inline (`renderFilters`) and block
// (`renderFiltersBlock`) renderers so both stay in sync.
func criterionStrings(obj dash0api.SpamFilterObject) []string {
	criteria := filterCriteria(obj)
	parts := make([]string, 0, len(criteria))
	for _, f := range criteria {
		parts = append(parts, formatCriterion(f))
	}
	return parts
}

func filterCriteria(obj dash0api.SpamFilterObject) dash0api.FilterCriteria {
	switch v := obj.(type) {
	case *dash0api.SpamFilter:
		return v.Spec.Filter
	case *dash0api.SpamFilterV1Alpha2:
		return v.Spec.Filter
	default:
		return nil
	}
}

// formatCriterion renders a single AttributeFilter as `key <operator> value`.
// The value union may carry a scalar string, a list, or be absent (for
// is_set / is_not_set); we render the most informative form available.
func formatCriterion(f dash0api.AttributeFilter) string {
	op := string(f.Operator)
	key := string(f.Key)

	if f.Values != nil {
		var values []string
		for _, item := range *f.Values {
			if sv, err := item.AsAttributeFilterStringValue(); err == nil {
				values = append(values, sv)
			}
		}
		if len(values) > 0 {
			return fmt.Sprintf("%s %s %s", key, op, strings.Join(values, " | "))
		}
		return fmt.Sprintf("%s %s", key, op)
	}
	if f.Value != nil {
		if sv, err := f.Value.AsAttributeFilterStringValue(); err == nil {
			return fmt.Sprintf("%s %s %s", key, op, sv)
		}
	}
	return fmt.Sprintf("%s %s", key, op)
}

func printSpamFilterTable(f *output.Formatter, items []dash0api.SpamFilterObject, format output.Format, apiUrl string) error {
	columns := []output.Column{
		{Header: internal.HEADER_NAME, Width: 40, Value: func(item interface{}) string {
			return objectName(item.(dash0api.SpamFilterObject))
		}},
		{Header: internal.HEADER_ID, Width: 36, Value: func(item interface{}) string {
			return objectID(item.(dash0api.SpamFilterObject))
		}},
		{Header: internal.HEADER_CONTEXT, Width: 16, Value: func(item interface{}) string {
			return renderContext(item.(dash0api.SpamFilterObject))
		}},
	}

	if format == output.FormatWide || format == output.FormatCSV {
		columns = append(columns,
			output.Column{Header: internal.HEADER_API_VERSION, Width: 12, Value: func(item interface{}) string {
				return objectAPIVersion(item.(dash0api.SpamFilterObject))
			}},
			output.Column{Header: internal.HEADER_FILTERS, Width: 60, Value: func(item interface{}) string {
				return renderFilters(item.(dash0api.SpamFilterObject))
			}},
			output.Column{Header: internal.HEADER_DATASET, Width: 15, Value: func(item interface{}) string {
				return objectDataset(item.(dash0api.SpamFilterObject))
			}},
			output.Column{Header: internal.HEADER_ORIGIN, Width: 20, Value: func(item interface{}) string {
				return objectOrigin(item.(dash0api.SpamFilterObject))
			}},
		)
	}

	if len(items) == 0 {
		fmt.Println("No spam filters found.")
		return nil
	}

	data := make([]interface{}, len(items))
	for i, r := range items {
		data[i] = r
	}

	if format == output.FormatCSV {
		return f.PrintCSV(columns, data)
	}
	return f.PrintTable(columns, data)
}
