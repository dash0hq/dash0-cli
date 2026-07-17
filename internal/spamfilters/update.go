package spamfilters

import (
	"context"
	"fmt"
	"os"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	var flags asset.FileInputFlags

	cmd := &cobra.Command{
		Use:   "update [origin-or-id] -f <file>",
		Short: "[experimental] Update a spam filter from a file",
		Long: `Update an existing spam filter from a YAML or JSON definition file. Use '-f -'
to read from stdin.

The positional argument selects the existing filter and accepts either
the 'dash0.com/origin' label or the 'dash0.com/id' label. Origin is the
recommended upsert key: the spam filter API uses it to route both create
and update through the same handler, so 'apply' is idempotent on origin
but not on user-supplied IDs (the server assigns the ID server-side on
create).

When the positional argument is omitted, the upsert key is taken from
the file in this order:
  1. 'dash0.com/origin' under metadata.labels (preferred)
  2. 'dash0.com/id'     under metadata.labels (fallback)

The apiVersion field on the document selects the schema: v1alpha1 uses
spec.contexts (array); v1alpha2 uses spec.context (scalar).` + internal.CONFIG_HINT,
		Example: `  # Update by origin
  dash0 --experimental spam-filters update <origin> -f filter.yaml

  # Update by ID
  dash0 --experimental spam-filters update <id> -f filter.yaml

  # Update using the origin (or id) from the file
  dash0 --experimental spam-filters update -f filter.yaml

  # Export, edit, and update
  dash0 --experimental spam-filters get <origin-or-id> -o yaml > filter.yaml
  # edit filter.yaml
  dash0 --experimental spam-filters update -f filter.yaml`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runUpdate(cmd.Context(), args, &flags)
		},
	}

	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runUpdate(ctx context.Context, args []string, flags *asset.FileInputFlags) error {
	raw, err := asset.ReadRawInput(flags.File, os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read spam filter definition: %w", err)
	}

	apiVersion, err := detectAPIVersion(raw)
	if err != nil {
		return err
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)

	switch apiVersion {
	case string(dash0api.SpamFilterApiVersionV1Alpha1):
		return runUpdateV1Alpha1(ctx, raw, args, apiClient, dataset, flags.DryRun)
	case string(dash0api.V1alpha2):
		return runUpdateV1Alpha2(ctx, raw, args, apiClient, dataset, flags.DryRun)
	default:
		// Unreachable: detectAPIVersion only returns supported values or an error.
		return fmt.Errorf("unsupported spam filter apiVersion %q (supported: %s)", apiVersion, joinQuoted(supportedAPIVersions))
	}
}

func runUpdateV1Alpha1(ctx context.Context, raw []byte, args []string, apiClient dash0api.Client, dataset *string, dryRun bool) error {
	filter, err := decodeV1Alpha1(raw)
	if err != nil {
		return err
	}

	fileOrigin := ""
	if filter.Metadata.Labels != nil && filter.Metadata.Labels.Dash0Comorigin != nil {
		fileOrigin = *filter.Metadata.Labels.Dash0Comorigin
	}
	key, err := resolveUpdateKey(args, fileOrigin, dash0api.GetSpamFilterID(filter))
	if err != nil {
		return err
	}

	before, err := apiClient.GetSpamFilter(ctx, key, dataset)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "spam filter",
			AssetID:   key,
		})
	}
	warnOnVersionMismatch(key, string(dash0api.SpamFilterApiVersionV1Alpha1), objectAPIVersion(before))

	if dryRun {
		return asset.PrintDiff(os.Stdout, "Spam filter", dash0api.GetSpamFilterName(filter), before, filter)
	}

	result, err := apiClient.UpdateSpamFilter(ctx, key, filter, dataset)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "spam filter",
			AssetID:   key,
			AssetName: dash0api.GetSpamFilterName(filter),
		})
	}

	return asset.PrintDiff(os.Stdout, "Spam filter", dash0api.GetSpamFilterName(result), before, result)
}

func runUpdateV1Alpha2(ctx context.Context, raw []byte, args []string, apiClient dash0api.Client, dataset *string, dryRun bool) error {
	filter, err := decodeV1Alpha2(raw)
	if err != nil {
		return err
	}

	fileOrigin, fileID := "", ""
	if filter.Metadata.Labels != nil {
		if filter.Metadata.Labels.Dash0Comorigin != nil {
			fileOrigin = *filter.Metadata.Labels.Dash0Comorigin
		}
		if filter.Metadata.Labels.Dash0Comid != nil {
			fileID = *filter.Metadata.Labels.Dash0Comid
		}
	}

	key, err := resolveUpdateKey(args, fileOrigin, fileID)
	if err != nil {
		return err
	}

	before, err := apiClient.GetSpamFilter(ctx, key, dataset)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "spam filter",
			AssetID:   key,
		})
	}
	warnOnVersionMismatch(key, string(dash0api.V1alpha2), objectAPIVersion(before))

	if dryRun {
		return asset.PrintDiff(os.Stdout, "Spam filter", filter.Metadata.Name, before, filter)
	}

	result, err := apiClient.UpdateSpamFilterV1Alpha2(ctx, key, filter, dataset)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "spam filter",
			AssetID:   key,
			AssetName: filter.Metadata.Name,
		})
	}

	return asset.PrintDiff(os.Stdout, "Spam filter", result.Metadata.Name, before, result)
}

// resolveUpdateKey returns the value to pass as the originOrId URL parameter
// for the PUT call. Precedence:
//  1. The positional argument, if provided.
//  2. The dash0.com/origin label in the file (origin is the preferred upsert
//     key; the spam filter API uses it to route create-or-update through the
//     same handler).
//  3. The dash0.com/id label in the file (kept as a fallback for files that
//     were exported before origin became a routine label, though apply
//     idempotency is not guaranteed through ID alone).
//
// A positional argument that matches neither the file's origin nor its ID
// is treated as a user error so the caller can fix one side before the PUT
// lands at the wrong record.
func resolveUpdateKey(args []string, fileOrigin, fileID string) (string, error) {
	if len(args) == 1 {
		arg := args[0]
		if fileOrigin != "" && fileID != "" && arg != fileOrigin && arg != fileID {
			return "", fmt.Errorf(
				"the argument %q does not match the origin %q or the ID %q in the file",
				arg, fileOrigin, fileID,
			)
		}
		if fileOrigin != "" && fileID == "" && arg != fileOrigin {
			return "", fmt.Errorf("the argument %q does not match the origin %q in the file", arg, fileOrigin)
		}
		if fileOrigin == "" && fileID != "" && arg != fileID {
			return "", fmt.Errorf("the argument %q does not match the ID %q in the file", arg, fileID)
		}
		return arg, nil
	}
	if fileOrigin != "" {
		return fileOrigin, nil
	}
	if fileID != "" {
		return fileID, nil
	}
	return "", fmt.Errorf(
		"no spam filter origin or ID provided as argument, and the file does not contain a 'dash0.com/origin' or 'dash0.com/id' label under metadata.labels",
	)
}

// warnOnVersionMismatch prints a stderr note when the server-stored apiVersion
// differs from the apiVersion in the input. The update is still attempted —
// PUT has create-or-replace semantics, and the user may be intentionally
// migrating between schemas — but the note makes the implicit conversion
// visible.
func warnOnVersionMismatch(id, inputVersion, serverVersion string) {
	if serverVersion == "" || serverVersion == inputVersion {
		return
	}
	fmt.Fprintf(os.Stderr,
		"Note: server-stored spam filter %q is %s; replacing with %s definition from input.\n",
		id, serverVersion, inputVersion,
	)
}
