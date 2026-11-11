package asset

import (
	"context"
	"fmt"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
)

// SpamFilterSupportedAPIVersions lists the spam filter apiVersion values the
// CLI understands. Exported so callers can render the supported list in their
// own error messages.
var SpamFilterSupportedAPIVersions = []string{
	string(dash0api.SpamFilterApiVersionV1Alpha1V1alpha1),
	string(dash0api.SpamFilterApiVersionV1Alpha2V1alpha2),
}

// DetectSpamFilterAPIVersion peeks at the apiVersion field on a YAML or JSON
// spam filter document so the caller can route to the matching client method.
// A missing value defaults to v1alpha1, matching the API client's
// decodeSpamFilterObject behavior. An unsupported value is rejected with
// the list of supported versions so the user knows what to fix.
func DetectSpamFilterAPIVersion(data []byte) (string, error) {
	var disc struct {
		ApiVersion string `json:"apiVersion"`
	}
	if err := dash0yaml.Unmarshal(data, &disc); err != nil {
		return "", fmt.Errorf("failed to detect spam filter apiVersion: %w", err)
	}
	normalized, ok := dash0api.NormalizeDash0ApiVersion(disc.ApiVersion)
	if !ok {
		quoted := make([]string, len(SpamFilterSupportedAPIVersions))
		for i, v := range SpamFilterSupportedAPIVersions {
			quoted[i] = fmt.Sprintf("%q", v)
		}
		return "", fmt.Errorf(
			"unsupported spam filter apiVersion %q (supported: %s)",
			disc.ApiVersion,
			strings.Join(quoted, ", "),
		)
	}
	switch normalized {
	case "", string(dash0api.SpamFilterApiVersionV1Alpha1V1alpha1):
		return string(dash0api.SpamFilterApiVersionV1Alpha1V1alpha1), nil
	case string(dash0api.SpamFilterApiVersionV1Alpha2V1alpha2):
		return normalized, nil
	default:
		quoted := make([]string, len(SpamFilterSupportedAPIVersions))
		for i, v := range SpamFilterSupportedAPIVersions {
			quoted[i] = fmt.Sprintf("%q", v)
		}
		return "", fmt.Errorf(
			"unsupported spam filter apiVersion %q (supported: %s)",
			disc.ApiVersion,
			strings.Join(quoted, ", "),
		)
	}
}

// SpamFilterUsesOrigin reports whether a spam filter document (v1alpha1 or
// v1alpha2 — both share the same metadata/labels shape) carries a non-empty
// dash0.com/origin label.
//
// --since uses this to warn when a spam filter is about to be deleted by
// dash0.com/id alone: per ImportSpamFilter's upsert-key selection, an
// ID-only spam filter's id is reassigned server-side on its first PUT to a
// brand-new id, so the id recorded in the git history --since diffs against
// may no longer match the live asset's actual id by the time the document
// is deleted — deleting by that stale id either 404s (hard-failing without
// --force) or is silently treated as already-deleted (with --force),
// leaving the real live filter orphaned either way. There is no local,
// API-free way to recover the live id from git history alone; the origin
// label is the only identifier that never gets reassigned.
func SpamFilterUsesOrigin(data []byte) (bool, error) {
	var filter dash0api.SpamFilter
	if err := dash0yaml.Unmarshal(data, &filter); err != nil {
		return false, fmt.Errorf("failed to decode Dash0SpamFilter: %w", err)
	}
	return filter.Metadata.Labels != nil && filter.Metadata.Labels.Dash0Comorigin != nil && *filter.Metadata.Labels.Dash0Comorigin != "", nil
}

// ImportSpamFilter creates or updates a v1alpha1 spam filter via the standard
// CRUD APIs.
//
// Upsert key selection — important quirk of the spam filter API:
//
//   - If the input has a user-defined origin (label `dash0.com/origin`),
//     the origin is used as the upsert key. PUT to /spam-filters/{origin}
//     creates the asset on first call and updates it on subsequent calls.
//     This is the only reliable path to idempotent `apply`.
//   - If the input has a user-defined ID (label `dash0.com/id`) but no
//     origin, the ID is used as the upsert key. The server only honors
//     this when a record already exists at that ID — a PUT to a brand-new
//     ID generates a fresh server-side ID, so user-defined-id-only flows
//     do not get true idempotency.
//   - Otherwise, CREATE is used and the server assigns an ID.
//
// dash0.com/id is captured before StripSpamFilterServerFields runs because
// that helper clears the id label along with the server source label.
func ImportSpamFilter(ctx context.Context, apiClient dash0api.Client, filter *dash0api.SpamFilter, dataset *string) (ImportResult, error) {
	// A failed preflight does not block the write: the PUT reports the same error.
	plan, _ := PlanSpamFilter(ctx, apiClient, filter, dataset)

	var (
		result *dash0api.SpamFilter
		err    error
	)
	if plan.Key != "" {
		result, err = apiClient.UpdateSpamFilter(ctx, plan.Key, filter, dataset)
	} else {
		result, err = apiClient.CreateSpamFilter(ctx, filter, dataset)
	}
	if err != nil {
		return ImportResult{}, err
	}

	resultID := dash0api.GetSpamFilterID(result)
	return ImportResult{Name: dash0api.GetSpamFilterName(result), ID: resultID, Action: plan.Action, Before: plan.Before, After: result}, nil
}

// PlanSpamFilter reads the stored v1alpha1 spam filter that ImportSpamFilter
// would write. Origin wins over id. It returns any preflight error other than
// "not found".
func PlanSpamFilter(ctx context.Context, apiClient dash0api.Client, filter *dash0api.SpamFilter, dataset *string) (ImportPlan, error) {
	// StripSpamFilterServerFields clears the dash0.com/id label, so read it first.
	key := PreferredKey(filter)
	dash0api.StripSpamFilterServerFields(filter)
	return planSpamFilterByKey(ctx, apiClient, key, filter, dataset)
}

// ImportSpamFilterV1Alpha2 mirrors ImportSpamFilter for the v1alpha2 schema.
// The v1alpha2 type carries spec.context (scalar) instead of spec.contexts
// (array); the rest of the lifecycle — upsert-key selection (origin first,
// ID fallback), existence check via GetSpamFilter, create-vs-update routing —
// is identical. See ImportSpamFilter for the rationale.
func ImportSpamFilterV1Alpha2(ctx context.Context, apiClient dash0api.Client, filter *dash0api.SpamFilterV1Alpha2, dataset *string) (ImportResult, error) {
	// A failed preflight does not block the write: the PUT reports the same error.
	plan, _ := PlanSpamFilterV1Alpha2(ctx, apiClient, filter, dataset)

	var (
		result *dash0api.SpamFilterV1Alpha2
		err    error
	)
	if plan.Key != "" {
		result, err = apiClient.UpdateSpamFilterV1Alpha2(ctx, plan.Key, filter, dataset)
	} else {
		result, err = apiClient.CreateSpamFilterV1Alpha2(ctx, filter, dataset)
	}
	if err != nil {
		return ImportResult{}, err
	}

	return ImportResult{Name: result.Metadata.Name, ID: SpamFilterV1Alpha2ID(result), Action: plan.Action, Before: plan.Before, After: result}, nil
}

// PlanSpamFilterV1Alpha2 reads the stored spam filter that
// ImportSpamFilterV1Alpha2 would write. Origin wins over id. It returns any
// preflight error other than "not found".
func PlanSpamFilterV1Alpha2(ctx context.Context, apiClient dash0api.Client, filter *dash0api.SpamFilterV1Alpha2, dataset *string) (ImportPlan, error) {
	return planSpamFilterByKey(ctx, apiClient, PreferredKey(filter), filter, dataset)
}

func planSpamFilterByKey(ctx context.Context, apiClient dash0api.Client, key string, proposed any, dataset *string) (ImportPlan, error) {
	plan := ImportPlan{Key: key, Action: ActionCreated, Proposed: proposed}
	if key == "" {
		return plan, nil
	}
	existing, err := apiClient.GetSpamFilter(ctx, key, dataset)
	if err != nil {
		return plan, ignoreNotFound(err)
	}
	plan.Action, plan.Before = ActionUpdated, existing
	return plan, nil
}

// SpamFilterV1Alpha2ID extracts the dash0.com/id label from a v1alpha2 filter. Kept here
// rather than re-exporting from the api client because the API client only
// exposes labelled accessors for v1alpha1.
func SpamFilterV1Alpha2ID(filter *dash0api.SpamFilterV1Alpha2) string {
	if filter == nil || filter.Metadata.Labels == nil || filter.Metadata.Labels.Dash0Comid == nil {
		return ""
	}
	return *filter.Metadata.Labels.Dash0Comid
}
