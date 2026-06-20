package spamfilters

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/confirmation"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/spf13/cobra"
)

func newDeleteCmd() *cobra.Command {
	var flags asset.DeleteFlags

	cmd := &cobra.Command{
		Use:     "delete <id>",
		Aliases: []string{"remove"},
		Short:   "[experimental] Delete a spam filter",
		Long:    `Delete a spam filter by its ID. Use --force to skip the confirmation prompt.` + internal.CONFIG_HINT,
		Example: `  # Delete with confirmation prompt
  dash0 --experimental spam-filters delete <id>

  # Delete without confirmation (for scripts and automation)
  dash0 --experimental spam-filters delete <id> --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runDelete(cmd.Context(), args[0], &flags)
		},
	}

	asset.RegisterDeleteFlags(cmd, &flags)
	return cmd
}

func runDelete(ctx context.Context, id string, flags *asset.DeleteFlags) error {
	confirmed, err := confirmation.ConfirmDestructiveOperation(
		ctx,
		fmt.Sprintf("Are you sure you want to delete spam filter %q? [y/N]: ", id),
		flags.Force,
	)
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Println("Deletion cancelled")
		return nil
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)
	err = deleteWithVersionConflictRetry(ctx, apiClient, id, dataset)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "spam filter",
			AssetID:   id,
		})
	}

	fmt.Printf("Spam filter %q deleted\n", id)
	return nil
}

// deleteWithVersionConflictRetry retries the delete on 409 "dataset version conflict" responses.
// The spam filter API uses ClickHouse MVCC and can return a transient version conflict
// immediately after an upsert; the server explicitly asks callers to retry in that case.
func deleteWithVersionConflictRetry(ctx context.Context, apiClient dash0api.Client, id string, dataset *string) error {
	const maxAttempts = 4
	const baseWait = 500 * time.Millisecond

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(baseWait * time.Duration(attempt)):
			}
		}

		lastErr = apiClient.DeleteSpamFilter(ctx, id, dataset)
		if lastErr == nil {
			return nil
		}
		if !isVersionConflict(lastErr) {
			return lastErr
		}
	}
	return lastErr
}

func isVersionConflict(err error) bool {
	if !dash0api.IsConflict(err) {
		return false
	}
	var apiErr *dash0api.APIError
	if errors.As(err, &apiErr) {
		return strings.Contains(strings.ToLower(apiErr.Message), "version conflict") ||
			strings.Contains(strings.ToLower(apiErr.Body), "version conflict")
	}
	return false
}
