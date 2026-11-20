package slos

import (
	"context"
	"fmt"
	"os"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	var flags asset.FileInputFlags

	cmd := &cobra.Command{
		Use:   "update [id] -f <file>",
		Short: "Update an SLO from a file",
		Long: `Update an existing SLO from a YAML or JSON definition file in OpenSLO v1 format. Use '-f -' to read from stdin.

If the ID argument is omitted, the file's dash0.com/id label is read first, then its dash0.com/origin label when the id is not found, as apply does.` + internal.CONFIG_HINT,
		Example: `  # Update an SLO from a file
  dash0 slos update <id> -f slo.yaml

  # Update using the origin or ID from the file
  dash0 slos update -f slo.yaml

  # Export, edit, and update
  dash0 slos get <id> -o yaml > slo.yaml
  # edit slo.yaml
  dash0 slos update -f slo.yaml`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd.Context(), args, &flags)
		},
	}

	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runUpdate(ctx context.Context, args []string, flags *asset.FileInputFlags) error {
	var slo dash0api.SloDefinition
	if err := asset.ReadDefinition(flags.File, &slo, os.Stdin); err != nil {
		return fmt.Errorf("failed to read SLO definition: %w", err)
	}

	fileID := dash0api.GetSLOID(&slo)
	fileOrigin := dash0api.GetSLOOrigin(&slo)

	if len(args) == 1 {
		// Only enforceable when the document pins both handles, in which case
		// the argument has to be one of them. With just one label present the
		// argument may legitimately be the other — GET/PUT take an
		// origin-or-id path segment, and an origin-pinned document says
		// nothing about the server-assigned id — and there is no offline way
		// to tell a valid counterpart from a typo. A wrong value surfaces as a
		// clean "SLO not found" from the GET below.
		if fileID != "" && fileOrigin != "" && args[0] != fileID && args[0] != fileOrigin {
			return fmt.Errorf("the ID argument %q does not match the dash0.com/id or dash0.com/origin label in the file (id=%q, origin=%q)", args[0], fileID, fileOrigin)
		}
	}

	updateKey := asset.UpdateKey{Noun: "SLO", Upsert: &slo}
	identity := asset.Identity{ID: fileID, Origin: fileOrigin}
	if len(args) == 0 {
		if err := asset.CheckRouteUpdate(nil, updateKey, identity); err != nil {
			return err
		}
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)

	// Without an argument, the read takes the file's id first, then its
	// origin, as apply does. update never creates, so a missing SLO fails here.
	get := func(ctx context.Context, key string) (*dash0api.SloDefinition, error) {
		return apiClient.GetSLO(ctx, key, dataset)
	}
	var id string
	var before *dash0api.SloDefinition
	if len(args) == 1 {
		id = args[0]
		before, err = get(ctx, id)
	} else {
		id, before, err = asset.RouteUpdate(ctx, nil, updateKey, identity, get)
	}
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "SLO",
			AssetID:   id,
		})
	}

	// Strip server-managed fields before diffing or sending the body back. An
	// exported SLO (e.g. via `dash0 slos get -o yaml`) carries dash0.com/origin
	// plus version, dataset, source, and created/updated timestamps, which the
	// server rejects or echoes back as spurious diff noise (the same class of
	// "origin does not match" 400 that bit views). The apply code path
	// (asset.ImportSLO) does the same strip; without it, `apply -f` and
	// `update -f` diverge on the same exported file.
	dash0api.StripSLOServerFields(&slo)

	if flags.DryRun {
		return asset.PrintUpdateOutcome(os.Stdout, asset.UpdateOutcome{
			DisplayKind:     "SLO",
			Name:            dash0api.GetSLOName(&slo),
			ID:              id,
			Before:          before,
			After:           &slo,
			BeforeDeletedAt: asset.SLODeletedAt(before),
			DryRun:          true,
		})
	}

	result, err := apiClient.UpdateSLO(ctx, id, &slo, dataset)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "SLO",
			AssetID:   id,
			AssetName: dash0api.GetSLOName(&slo),
		})
	}

	return asset.PrintUpdateOutcome(os.Stdout, asset.UpdateOutcome{
		DisplayKind:     "SLO",
		Name:            dash0api.GetSLOName(result),
		ID:              id,
		Before:          before,
		After:           result,
		BeforeDeletedAt: asset.SLODeletedAt(before),
		AfterDeletedAt:  asset.SLODeletedAt(result),
	})
}
