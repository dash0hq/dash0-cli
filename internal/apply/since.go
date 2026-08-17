package apply

import (
	"context"
	"fmt"
	"os"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/confirmation"
	gitutil "github.com/dash0hq/dash0-cli/internal/git"
)

// computeDeletionPlan resolves flags.Since against the git repository
// containing flags.File and diffs the identifier set at that ref against
// flags.File's current disk contents. It never talks to the Dash0 API —
// everything it needs comes from git and the local filesystem.
func computeDeletionPlan(ctx context.Context, flags *applyFlags) (*gitutil.SincePlan, error) {
	return gitutil.ComputeSincePlan(ctx, flags.File, flags.Since)
}

// Rendering (text and agent-mode JSON) for --dry-run, with or without a
// deletion plan, lives in dryrun.go.

// applyDeletions carries out dp's deletion plan against the Dash0 API,
// prompting per asset (skipped when force is set) exactly like every
// standalone `<kind> delete --force`. It returns the number of deletions the
// caller declined so runApply can report a non-zero exit even though the
// rest of the run succeeded.
//
// dp.Warning (set when --since's ref is a non-ancestor) is not printed here:
// runApply already surfaced it once, as part of confirming whether to run
// the deletion phase at all, before calling this function. Printing it again
// here would show the identical line to the user twice for no reason.
func applyDeletions(ctx context.Context, apiClient dash0api.Client, dataset *string, dp *gitutil.SincePlan, force bool) (int, error) {
	declined := 0

	for _, d := range dp.Plan.ByIdentifier {
		displayKind := asset.KindDisplayName(d.Kind)
		// dp.Names is resolved from git history and best-effort: a lookup
		// failure falls back to an explicit "<name>" placeholder, matching
		// printDryRunWithDeletions' convention.
		name := dp.Names[d.Path]
		if name == "" {
			name = "<name>"
		}
		display := asset.FormatNameAndID(name, d.Identifier)
		if d.Kind == "spamfilter" && !d.SpamFilterUsesOrigin {
			fmt.Fprintf(os.Stderr, "warning: spam filter %s was identified by dash0.com/id alone; its live id may have been reassigned by the server since this identifier was recorded (see docs/commands.md's asset-identifiers section), so this delete may miss the actual live filter\n", display)
		}
		prompt := fmt.Sprintf("Are you sure you want to delete %s %s, removed since --since ref? [y/N]: ", displayKind, display)
		confirmed, err := confirmation.ConfirmDestructiveOperation(ctx, prompt, force)
		if err != nil {
			return declined, err
		}
		if !confirmed {
			fmt.Fprintf(os.Stderr, "%s %s: deletion declined\n", displayKind, display)
			declined++
			continue
		}
		if err := deleteAssetByKindAndIdentifier(ctx, apiClient, dataset, d, force); err != nil {
			return declined, fmt.Errorf("failed to delete %s %s: %w", displayKind, display, err)
		}
		fmt.Printf("%s %s deleted\n", displayKind, display)
	}

	for _, a := range dp.Plan.AlertsByName {
		name := a.CheckRuleName()
		prompt := fmt.Sprintf("Are you sure you want to delete check rule %q, an alert removed from a PrometheusRule since --since ref? [y/N]: ", name)
		confirmed, err := confirmation.ConfirmDestructiveOperation(ctx, prompt, force)
		if err != nil {
			return declined, err
		}
		if !confirmed {
			fmt.Fprintf(os.Stderr, "Check rule %q: deletion declined\n", name)
			declined++
			continue
		}
		if err := deleteCheckRuleByName(ctx, apiClient, dataset, name, force); err != nil {
			return declined, fmt.Errorf("failed to delete check rule %q: %w", name, err)
		}
		fmt.Printf("Check rule %q deleted\n", name)
	}

	return declined, nil
}

// deleteAssetByKindAndIdentifier dispatches a whole-asset deletion (an
// asset whose identifier disappeared entirely) to the matching per-kind
// delete API call, mirroring the dispatch applyDocument already uses for
// create/update.
func deleteAssetByKindAndIdentifier(ctx context.Context, apiClient dash0api.Client, dataset *string, d gitutil.Deletion, force bool) error {
	kind, identifier := d.Kind, d.Identifier
	if kind == "prometheusrule" {
		return deletePrometheusRuleCRD(ctx, apiClient, dataset, identifier, d.PrometheusRuleEndpoints, force)
	}

	var err error
	switch kind {
	case "dashboard", "persesdashboard":
		err = apiClient.DeleteDashboard(ctx, identifier, dataset)
	case "checkrule":
		err = apiClient.DeleteCheckRule(ctx, identifier, dataset)
	case "syntheticcheck":
		err = apiClient.DeleteSyntheticCheck(ctx, identifier, dataset)
	case "view":
		err = apiClient.DeleteView(ctx, identifier, dataset)
	case "spamfilter":
		err = apiClient.DeleteSpamFilter(ctx, identifier, dataset)
	case "notificationchannel":
		err = apiClient.DeleteNotificationChannel(ctx, identifier)
	case "team":
		err = apiClient.DeleteTeam(ctx, identifier)
	default:
		return fmt.Errorf("unsupported kind for deletion: %s", kind)
	}

	ectx := client.ErrorContext{AssetType: asset.KindDisplayName(kind), AssetID: identifier}
	if err != nil {
		if client.IsAlreadyDeleted(err, force, ectx) {
			return nil
		}
		return client.HandleAPIError(err, ectx)
	}
	return nil
}

// deletePrometheusRuleCRD deletes a whole PrometheusRule CRD by identifier.
// The same identifier may back a check rule (from the CRD's alerting rules),
// a recording rule (from its recording rules), or both — apply's own
// create/update dispatch (applyPrometheusRule) sends a mixed CRD to both
// endpoints, so a mixed CRD's deletion attempts both too.
//
// endpoints (extracted from the CRD's content at --since's ref, before it
// was deleted) says which endpoint(s) the CRD actually used. Only those are
// called: unconditionally attempting both and tolerating a 404 from
// whichever wasn't used would silently delete an unrelated asset that
// happens to carry the same identifier on the endpoint this CRD never used.
// If endpoints reports neither (only possible for a Snapshot built before
// this field existed, or corrupted git history), both are attempted and a
// 404 from either is tolerated, matching the old best-effort behavior.
func deletePrometheusRuleCRD(ctx context.Context, apiClient dash0api.Client, dataset *string, identifier string, endpoints gitutil.PrometheusRuleEndpoints, force bool) error {
	tryCheckRule := endpoints.HasAlerts
	tryRecordingRule := endpoints.HasRecords
	if !tryCheckRule && !tryRecordingRule {
		tryCheckRule, tryRecordingRule = true, true
	}

	var checkRuleErr, recordingRuleErr error
	if tryCheckRule {
		checkRuleErr = apiClient.DeleteCheckRule(ctx, identifier, dataset)
	}
	if tryRecordingRule {
		recordingRuleErr = apiClient.DeleteRecordingRule(ctx, identifier, dataset)
	}

	checkRuleNotFound := checkRuleErr != nil && dash0api.IsNotFound(checkRuleErr)
	recordingRuleNotFound := recordingRuleErr != nil && dash0api.IsNotFound(recordingRuleErr)

	if checkRuleErr != nil && !checkRuleNotFound {
		ectx := client.ErrorContext{AssetType: "check rule", AssetID: identifier}
		if client.IsAlreadyDeleted(checkRuleErr, force, ectx) {
			return nil
		}
		return client.HandleAPIError(checkRuleErr, ectx)
	}
	if recordingRuleErr != nil && !recordingRuleNotFound {
		ectx := client.ErrorContext{AssetType: "recording rule", AssetID: identifier}
		if client.IsAlreadyDeleted(recordingRuleErr, force, ectx) {
			return nil
		}
		return client.HandleAPIError(recordingRuleErr, ectx)
	}

	// "Genuinely gone" means 404 on every endpoint that was actually tried —
	// an endpoint that was never tried (because the CRD didn't use it)
	// contributes no signal either way.
	genuinelyGone := (!tryCheckRule || checkRuleNotFound) && (!tryRecordingRule || recordingRuleNotFound)
	if genuinelyGone {
		ectx := client.ErrorContext{AssetType: "PrometheusRule", AssetID: identifier}
		firstErr := checkRuleErr
		if firstErr == nil {
			firstErr = recordingRuleErr
		}
		if client.IsAlreadyDeleted(firstErr, force, ectx) {
			return nil
		}
		return client.HandleAPIError(firstErr, ectx)
	}
	return nil
}

// deleteCheckRuleByName resolves a check rule by its exact name (the "<group
// name> - <alert name>" composed by apply's create/update path) and deletes
// it. This is the only way to target a single alerting rule removed from a
// PrometheusRule CRD that otherwise survives: the CRD's shared identifier
// can't distinguish between the alerts it contains.
func deleteCheckRuleByName(ctx context.Context, apiClient dash0api.Client, dataset *string, name string, force bool) error {
	id, err := findCheckRuleIDByName(ctx, apiClient, dataset, name)
	if err != nil {
		return err
	}
	if id == "" {
		if force {
			fmt.Fprintf(os.Stderr, "Check rule %q was already deleted\n", name)
			return nil
		}
		return fmt.Errorf("check rule %q not found (already deleted?)", name)
	}

	err = apiClient.DeleteCheckRule(ctx, id, dataset)
	ectx := client.ErrorContext{AssetType: "check rule", AssetID: id, AssetName: name}
	if err != nil {
		if client.IsAlreadyDeleted(err, force, ectx) {
			return nil
		}
		return client.HandleAPIError(err, ectx)
	}
	return nil
}

// findCheckRuleIDByName lists every check rule in dataset and returns the ID
// of the first one whose name matches exactly, or "" if none matches.
func findCheckRuleIDByName(ctx context.Context, apiClient dash0api.Client, dataset *string, name string) (string, error) {
	iter := apiClient.ListCheckRulesIter(ctx, dataset)
	for iter.Next() {
		item := iter.Current()
		if item.Name != nil && *item.Name == name {
			return item.Id, nil
		}
	}
	if err := iter.Err(); err != nil {
		return "", fmt.Errorf("failed to list check rules: %w", err)
	}
	return "", nil
}
