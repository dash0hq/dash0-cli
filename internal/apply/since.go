package apply

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/confirmation"
	gitutil "github.com/dash0hq/dash0-cli/internal/git"
)

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

	// Datasets go last: assets live inside a dataset, and the API ignores an asset delete in a
	// dataset that is already deleted, so a dataset deleted first would keep its assets for good.
	var assets, datasets []gitutil.Deletion
	for _, d := range dp.Plan.ByIdentifier {
		if d.Kind == "dataset" {
			datasets = append(datasets, d)
		} else {
			assets = append(assets, d)
		}
	}

	deleteAll := func(deletions []gitutil.Deletion) error {
		for _, d := range deletions {
			wasDeclined, err := deleteRemovedAsset(ctx, apiClient, dataset, dp, d, force)
			if err != nil {
				return err
			}
			if wasDeclined {
				declined++
			}
		}
		return nil
	}

	if err := deleteAll(assets); err != nil {
		return declined, err
	}

	var nameIndex checkRuleNameIndex
	if len(dp.Plan.AlertsByName) > 0 {
		// One listing for the whole loop, not one per alert.
		var err error
		nameIndex, err = buildCheckRuleNameIndex(ctx, apiClient, dataset)
		if err != nil {
			return declined, err
		}
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
		alreadyDeleted, err := deleteCheckRuleByName(ctx, apiClient, dataset, name, nameIndex)
		if err != nil {
			return declined, fmt.Errorf("failed to delete check rule %q: %w", name, err)
		}
		if !alreadyDeleted {
			fmt.Printf("Check rule %q deleted\n", name)
		}
	}

	if err := deleteAll(datasets); err != nil {
		return declined, err
	}

	return declined, nil
}

// deleteRemovedAsset confirms and deletes one whole-asset deletion from the plan.
// It reports true when the user declined.
func deleteRemovedAsset(ctx context.Context, apiClient dash0api.Client, dataset *string, dp *gitutil.SincePlan, d gitutil.Deletion, force bool) (declined bool, err error) {
	displayKind := asset.KindDisplayName(d.Kind)
	// dp.Names is resolved from git history and best-effort: a lookup
	// failure falls back to an explicit "<name>" placeholder, matching
	// printDryRunWithDeletions' convention.
	name := dp.Names[d.Path]
	if name == "" {
		name = "<name>"
	}
	display := asset.FormatNameAndID(name, d.Identifier)
	if d.Kind == "slo" && !d.SLOUsesOrigin {
		fmt.Fprintf(os.Stderr, "warning: SLO %s was identified by dash0.com/id alone; SLO ids are server-assigned, so if this document was first applied to an organization without that id the live SLO now sits at a different one (see docs/commands.md's asset-identifiers section) and this delete may miss it\n", display)
	}
	if d.Kind == "generativeaievaluation" && !d.GenerativeAiEvaluationUsesOrigin {
		fmt.Fprintf(os.Stderr, "warning: eval %s was identified by dash0.com/id alone; eval ids are server-assigned, so if this document was first applied to an organization without that id the live eval now sits at a different one (see docs/commands.md's asset-identifiers section) and this delete may miss it\n", display)
	}
	if d.Kind == "spamfilter" && !d.SpamFilterUsesOrigin {
		fmt.Fprintf(os.Stderr, "warning: spam filter %s was identified by dash0.com/id alone; its live id may have been reassigned by the server since this identifier was recorded (see docs/commands.md's asset-identifiers section), so this delete may miss the actual live filter\n", display)
	}
	var prompt string
	if d.Kind == "recordingrule" {
		// The surviving PrometheusRule CRD's own file is not what's
		// being removed here -- only its recording-rule role. The
		// generic "removed since --since ref" phrasing below would
		// wrongly suggest the whole document is gone.
		prompt = fmt.Sprintf("Are you sure you want to delete %s %s, whose last record was removed from its PrometheusRule since --since ref? [y/N]: ", displayKind, display)
	} else if d.Kind == "dataset" {
		prompt = fmt.Sprintf("Are you sure you want to delete %s %s, removed since --since ref? %s [y/N]: ", displayKind, display, asset.DatasetSoftDeleteNote)
	} else {
		prompt = fmt.Sprintf("Are you sure you want to delete %s %s, removed since --since ref? [y/N]: ", displayKind, display)
	}
	confirmed, err := confirmation.ConfirmDestructiveOperation(ctx, prompt, force)
	if err != nil {
		return false, err
	}
	if !confirmed {
		fmt.Fprintf(os.Stderr, "%s %s: deletion declined\n", displayKind, display)
		return true, nil
	}
	alreadyDeleted, err := deleteAssetByKindAndIdentifier(ctx, apiClient, dataset, d, dp.DeclaredCheckRuleIDs)
	if err != nil {
		return false, fmt.Errorf("failed to delete %s %s: %w", displayKind, display, err)
	}
	// alreadyDeleted means IsAlreadyDeleted already printed its own
	// "was already deleted" line -- printing "deleted" here too would
	// contradict it and misrepresent a no-op as a real deletion in a CI
	// log or audit trail.
	if !alreadyDeleted {
		verb := "deleted"
		if d.Kind == "awsmetricscrapeconfig" {
			verb = "cleared" // the integration itself is kept
		}
		fmt.Printf("%s %s %s\n", displayKind, display, verb)
	}
	return false, nil
}

// deleteAssetByKindAndIdentifier dispatches a whole-asset deletion (an
// asset whose identifier disappeared entirely) to the matching per-kind
// delete API call, mirroring the dispatch applyDocument already uses for
// create/update.
//
// A 404 here always means "already gone" and is always tolerated,
// regardless of --force: --since's deletion phase is reconciling Dash0 to
// match git, and an asset that's already absent already matches the
// desired end state, whether or not the caller passed --force. --force
// keeps its own, separate job of skipping the confirmation prompt in
// applyDeletions, before this function is ever called. This is
// deliberately unlike the force-gated tolerance a standalone `<kind> delete`
// command uses: that command acts on one asset the caller named by hand, so
// a 404 without --force there is more likely a typo'd id worth surfacing
// loudly than a benign race.
//
// The returned bool reports whether the asset was already gone (rather
// than genuinely deleted by this call), so applyDeletions can avoid
// printing a "deleted" line that would contradict IsAlreadyDeleted's own
// "was already deleted" message -- printing both claims a deletion that
// never happened, which a CI log or audit trail would then take at face
// value.
func deleteAssetByKindAndIdentifier(ctx context.Context, apiClient dash0api.Client, dataset *string, d gitutil.Deletion, declared map[string]bool) (alreadyDeleted bool, err error) {
	kind, identifier := d.Kind, d.Identifier
	if kind == "prometheusrule" {
		return deletePrometheusRuleCRD(ctx, apiClient, dataset, identifier, d.PrometheusAlerts, declared)
	}

	switch kind {
	case "dashboard", "persesdashboard":
		err = apiClient.DeleteDashboard(ctx, identifier, dataset)
	case "checkrule":
		if declared[identifier] {
			// A surviving single-alert CRD's check rule lives here. Reporting
			// "nothing deleted" keeps applyDeletions from printing "deleted".
			warnSkippedDeclaredCheckRule(identifier, fmt.Sprintf("check rule %q", identifier))
			return true, nil
		}
		err = apiClient.DeleteCheckRule(ctx, identifier, dataset)
	case "syntheticcheck":
		err = apiClient.DeleteSyntheticCheck(ctx, identifier, dataset)
	case "slo":
		err = apiClient.DeleteSLO(ctx, identifier, dataset)
	case "recordingrule":
		// A surviving PrometheusRule CRD whose recording-rule role
		// disappeared entirely (see Diff's PrometheusRecordingRoleByIdentifier
		// handling) -- distinct from the whole-CRD "prometheusrule" case
		// above, which already attempts this endpoint unconditionally.
		err = apiClient.DeleteRecordingRule(ctx, identifier, dataset)
	case "view":
		err = apiClient.DeleteView(ctx, identifier, dataset)
	case "spamfilter":
		err = apiClient.DeleteSpamFilter(ctx, identifier, dataset)
	case "telemetrytransformationrule":
		err = apiClient.DeleteTelemetryTransformationRule(ctx, identifier, dataset)
	case "notificationchannel":
		err = apiClient.DeleteNotificationChannel(ctx, identifier)
	case "team":
		err = apiClient.DeleteTeam(ctx, identifier)
	case "skill":
		err = apiClient.DeleteCustomSkill(ctx, identifier)
	case "generativeaievaluation":
		err = apiClient.DeleteGenerativeAiEvaluation(ctx, identifier, dataset)
	case "dataset":
		// A soft delete: Dash0 keeps the dataset and its assets for 30 days.
		// The same path as `datasets delete`.
		err = apiClient.DeleteDataset(ctx, identifier)
	case "awsmetricscrapeconfig":
		// Clears the integration's scrape config and keeps the integration,
		// the same call as `aws-metric-scrape-configs delete`.
		err = apiClient.DeleteAwsMetricScrapeConfig(ctx, identifier)
	case "timeseriesaggregation":
		// The API returns 204 for an absent aggregation, so the tolerance
		// below never fires for this kind. It does fire for the cross-dataset
		// 400, which must not be mistaken for "already gone": the aggregation
		// exists and belongs to another dataset, so reporting it as reconciled
		// would be a lie in a CI log.
		err = apiClient.DeleteTimeSeriesAggregation(ctx, identifier, dataset)
		if asset.IsTimeSeriesAggregationWrongDataset(err) {
			return false, asset.WrapTimeSeriesAggregationWrongDataset(err, identifier)
		}
	default:
		return false, fmt.Errorf("unsupported kind for deletion: %s", kind)
	}

	ectx := client.ErrorContext{AssetType: asset.KindDisplayName(kind), AssetID: identifier}
	if err != nil {
		if client.IsAlreadyDeleted(err, true, ectx) {
			return true, nil
		}
		return false, client.HandleAPIError(err, ectx)
	}
	return false, nil
}

// deletePrometheusRuleCRD deletes a whole PrometheusRule CRD by identifier.
// The same identifier may back a check rule (from the CRD's alerting
// rules), a recording rule (from its recording rules), or both — apply's
// own create/update dispatch (applyPrometheusRule) sends a mixed CRD to
// both endpoints, so a mixed CRD's deletion attempts both too.
//
// alerts is the CRD's alerting-rule list at the "before" (--since ref)
// snapshot (gitutil.Deletion.PrometheusAlerts). A CRD with two or more
// alerts has each alert's real check rule living at its own derived id
// (asset.DeriveAlertCheckRuleID), never at the CRD's literal identifier --
// see composePrometheusRuleNames' doc comment for why -- so deleting such a
// CRD must attempt each alert's derived id individually. A CRD with zero or
// one alert keeps using the literal identifier directly, matching how
// applyPrometheusRule creates it.
//
// Every endpoint attempted (every check-rule id, plus the recording-rule
// endpoint) tolerates a 404: this used to be gated on which endpoint(s) the
// CRD's content at --since's ref showed it using, but that signal is a
// single point in time, not a history of everything the identifier has
// ever used. A CRD that had a recording rule stripped from it in an
// earlier commit (leaving only its alerting rules), followed by the whole
// file being deleted, showed --since a ref where the file only ever had
// alerts — silently orphaning the recording rule created earlier, with no
// later git state able to recover that fact once the file is gone.
// Deleting is naturally idempotent (a 404 just means this endpoint never
// had anything for this identifier), so attempting all of them
// unconditionally is safe by the same logic every other kind already
// relies on. The one tradeoff: a check rule or recording rule that happens
// to share one of these identifiers by coincidence (not because it came
// from this CRD) would be deleted too — accepted as an edge case narrow
// enough not to justify leaving real orphaned assets behind.
//
// A 404 on any endpoint always means "already gone" (see
// deleteAssetByKindAndIdentifier's doc comment for why this is
// unconditional, unlike a standalone `<kind> delete` command).
//
// The returned bool follows deleteAssetByKindAndIdentifier's contract: true
// only when *nothing* attempted had anything left to delete (every check
// rule id and the recording rule all 404), so a mixed outcome -- anything
// genuinely deleted, the rest already gone -- is reported as a real
// deletion, matching the fact that something was.
func deletePrometheusRuleCRD(ctx context.Context, apiClient dash0api.Client, dataset *string, identifier string, alerts []asset.PrometheusAlertName, declared map[string]bool) (alreadyDeleted bool, err error) {
	checkRuleIDs := asset.CheckRuleIDsOccupiedByCRD(identifier, alerts)
	if len(alerts) > 1 {
		// A multi-alert CRD applied before per-alert derivation existed left
		// its one check rule at the literal id (docs/commands.md's multi-alert
		// migration note); the per-id 404 tolerance below makes this extra
		// attempt free when there is no such orphan.
		checkRuleIDs = append([]string{identifier}, checkRuleIDs...)
	}

	var lastCheckRuleNotFoundErr error
	checkRulesAllNotFound := true
	for _, id := range checkRuleIDs {
		if declared[id] {
			// Not attempted, so not a "found nothing" either.
			warnSkippedDeclaredCheckRule(id, fmt.Sprintf("PrometheusRule %q", identifier))
			continue
		}
		checkRuleErr := apiClient.DeleteCheckRule(ctx, id, dataset)
		if checkRuleErr == nil {
			checkRulesAllNotFound = false
			continue
		}
		if !dash0api.IsNotFound(checkRuleErr) {
			return false, client.HandleAPIError(checkRuleErr, client.ErrorContext{AssetType: "check rule", AssetID: id})
		}
		lastCheckRuleNotFoundErr = checkRuleErr
	}

	recordingRuleErr := apiClient.DeleteRecordingRule(ctx, identifier, dataset)
	recordingRuleNotFound := recordingRuleErr != nil && dash0api.IsNotFound(recordingRuleErr)
	if recordingRuleErr != nil && !recordingRuleNotFound {
		return false, client.HandleAPIError(recordingRuleErr, client.ErrorContext{AssetType: "recording rule", AssetID: identifier})
	}

	// "Genuinely gone" means 404 on every check-rule id attempted and on
	// the recording-rule endpoint -- nothing had anything for this CRD.
	if checkRulesAllNotFound && recordingRuleNotFound {
		notFoundErr := lastCheckRuleNotFoundErr
		if notFoundErr == nil {
			notFoundErr = recordingRuleErr
		}
		ectx := client.ErrorContext{AssetType: "PrometheusRule", AssetID: identifier}
		if client.IsAlreadyDeleted(notFoundErr, true, ectx) {
			return true, nil
		}
		return false, client.HandleAPIError(notFoundErr, ectx)
	}
	return false, nil
}

// warnSkippedDeclaredCheckRule reports a skipped check-rule deletion, so the
// orphan it leaves behind is not invisible.
func warnSkippedDeclaredCheckRule(id, removedDisplay string) {
	fmt.Fprintf(os.Stderr, "warning: not deleting check rule %q on behalf of removed %s: the current contents of -f still declare a check rule with that identifier, and deleting it would undo this run's own create/update; remove it by hand if it is a leftover\n", id, removedDisplay)
}

// deleteCheckRuleByName deletes the check rule index resolves name to. Name
// is the only way to target a single alerting rule removed from a
// PrometheusRule CRD that otherwise survives: the CRD's shared identifier
// can't distinguish between the alerts it contains.
//
// Not finding it by name at all, or a 404 on the delete itself, always means
// "already gone" (see deleteAssetByKindAndIdentifier's doc comment for why
// this is unconditional, unlike a standalone `<kind> delete` command). The
// returned bool follows the same contract: true means nothing was actually
// deleted by this call.
func deleteCheckRuleByName(ctx context.Context, apiClient dash0api.Client, dataset *string, name string, index checkRuleNameIndex) (alreadyDeleted bool, err error) {
	id, err := index.resolve(name)
	if err != nil {
		return false, err
	}
	if id == "" {
		fmt.Fprintf(os.Stderr, "Check rule %q was already deleted\n", name)
		return true, nil
	}

	err = apiClient.DeleteCheckRule(ctx, id, dataset)
	ectx := client.ErrorContext{AssetType: "check rule", AssetID: id, AssetName: name}
	if err != nil {
		if client.IsAlreadyDeleted(err, true, ectx) {
			return true, nil
		}
		return false, client.HandleAPIError(err, ectx)
	}
	return false, nil
}

// checkRuleNameIndex maps each live check rule's name to every check rule
// carrying it, built once per run so resolving N removed alerts costs one
// listing rather than N, and so a shared name is visible as the ambiguity it
// is rather than resolving to whichever the API listed first.
type checkRuleNameIndex map[string][]checkRuleMatch

type checkRuleMatch struct {
	id     string
	source string // dash0.com/origin-derived system of record, "" when unset
}

// foreignCheckRuleSources are the systems of record --since must not delete
// on behalf of an alert removed from -f: a same-named check rule owned by one
// of them merely collides. A denylist, not an allowlist: the CLI strips
// dash0.com/origin before sending, so its own check rules read back as "api"
// or unset, and CrdSource's contract treats an unknown value as "api" too.
var foreignCheckRuleSources = map[string]bool{
	string(dash0api.Ui):        true,
	string(dash0api.Terraform): true,
	string(dash0api.Operator):  true,
	string(dash0api.Platform):  true,
}

func buildCheckRuleNameIndex(ctx context.Context, apiClient dash0api.Client, dataset *string) (checkRuleNameIndex, error) {
	index := checkRuleNameIndex{}
	iter := apiClient.ListCheckRulesIter(ctx, dataset)
	for iter.Next() {
		item := iter.Current()
		if item.Name == nil {
			continue
		}
		match := checkRuleMatch{id: item.Id}
		if item.Source != nil {
			match.source = string(*item.Source)
		}
		index[*item.Name] = append(index[*item.Name], match)
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to list check rules: %w", err)
	}
	return index, nil
}

// resolve returns the id of the one deletable check rule named name, "" when
// there is none, or an error when two carry it -- deleting the wrong one is
// unrecoverable, and the CRD's identifier cannot disambiguate them.
func (index checkRuleNameIndex) resolve(name string) (string, error) {
	var deletable, foreign []string
	for _, match := range index[name] {
		if foreignCheckRuleSources[match.source] {
			foreign = append(foreign, match.source)
			continue
		}
		deletable = append(deletable, match.id)
	}

	if len(deletable) > 1 {
		sort.Strings(deletable)
		return "", fmt.Errorf("check rule %q is ambiguous: %d check rules carry that name (%s)\nHint: --since resolves an alert removed from a surviving PrometheusRule CRD by name, since the CRD's shared identifier cannot distinguish its alerts; rename or delete the duplicates in Dash0, or skip --since for this invocation", name, len(deletable), strings.Join(deletable, ", "))
	}
	if len(deletable) == 0 {
		if len(foreign) > 0 {
			sort.Strings(foreign)
			fmt.Fprintf(os.Stderr, "warning: not deleting check rule %q: every check rule with that name is managed by another system (%s), not by this repository\n", name, strings.Join(foreign, ", "))
		}
		return "", nil
	}
	return deletable[0], nil
}
