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

// ImportSpamFilter creates or updates a v1alpha1 spam filter through
// RouteDocument. An id that is not found is never PUT, because the server
// would assign a fresh id and a repeated apply would create another filter.
func ImportSpamFilter(ctx context.Context, apiClient dash0api.Client, filter *dash0api.SpamFilter, dataset *string) (ImportResult, error) {
	plan, err := PlanSpamFilter(ctx, apiClient, filter, dataset)
	// A failed origin read does not block the write: the PUT reports the same error.
	if plan.Key == "" {
		return ImportResult{}, err
	}
	if plan.Action != ActionCreated {
		setSpamFilterOrigin(&filter.Metadata, "")
	}
	result, err := apiClient.UpdateSpamFilter(ctx, plan.Key, filter, dataset)
	if err != nil {
		return ImportResult{}, err
	}
	return ImportResult{Name: dash0api.GetSpamFilterName(result), ID: dash0api.GetSpamFilterID(result), Action: plan.Action, Before: plan.Before, After: result}, nil
}

// PlanSpamFilter reads the stored v1alpha1 spam filter that ImportSpamFilter
// would write. It returns any preflight error other than "not found"; an
// empty Key means the write has no target.
func PlanSpamFilter(ctx context.Context, apiClient dash0api.Client, filter *dash0api.SpamFilter, dataset *string) (ImportPlan, error) {
	// The strip clears the dash0.com/id label, so read it first.
	identity := SpamFilterIdentity(filter, filter.Metadata.Name, dataset)
	dash0api.StripSpamFilterServerFields(filter)
	return planSpamFilter(ctx, apiClient, identity, &filter.Metadata, filter, dataset)
}

// ImportSpamFilterV1Alpha2 mirrors ImportSpamFilter for the v1alpha2 schema,
// which carries spec.context (scalar) instead of spec.contexts (array).
func ImportSpamFilterV1Alpha2(ctx context.Context, apiClient dash0api.Client, filter *dash0api.SpamFilterV1Alpha2, dataset *string) (ImportResult, error) {
	plan, err := PlanSpamFilterV1Alpha2(ctx, apiClient, filter, dataset)
	if plan.Key == "" {
		return ImportResult{}, err
	}
	if plan.Action != ActionCreated {
		setSpamFilterOrigin(&filter.Metadata, "")
	}
	result, err := apiClient.UpdateSpamFilterV1Alpha2(ctx, plan.Key, filter, dataset)
	if err != nil {
		return ImportResult{}, err
	}
	return ImportResult{Name: result.Metadata.Name, ID: SpamFilterV1Alpha2ID(result), Action: plan.Action, Before: plan.Before, After: result}, nil
}

// PlanSpamFilterV1Alpha2 reads the stored spam filter that
// ImportSpamFilterV1Alpha2 would write. See PlanSpamFilter.
func PlanSpamFilterV1Alpha2(ctx context.Context, apiClient dash0api.Client, filter *dash0api.SpamFilterV1Alpha2, dataset *string) (ImportPlan, error) {
	identity := SpamFilterIdentity(filter, filter.Metadata.Name, dataset)
	dash0api.StripSpamFilterV1Alpha2ServerFields(filter)
	return planSpamFilter(ctx, apiClient, identity, &filter.Metadata, filter, dataset)
}

func planSpamFilter(ctx context.Context, apiClient dash0api.Client, identity Identity, metadata *dash0api.SpamFilterMetadata, proposed any, dataset *string) (ImportPlan, error) {
	get := func(ctx context.Context, key string) (dash0api.SpamFilterObject, error) {
		return apiClient.GetSpamFilter(ctx, key, dataset)
	}
	route, err := RouteDocument(ctx, identity, get, spamFilterOrigin)
	plan := ImportPlan{Key: route.Key, Action: ActionCreated, Proposed: proposed}
	if route.Key == "" || route.Create {
		setSpamFilterOrigin(metadata, route.Key)
		return plan, err
	}
	setSpamFilterOrigin(metadata, spamFilterOrigin(route.Stored))
	plan.Action, plan.Before = ActionUpdated, route.Stored
	return plan, err
}

// SpamFilterIdentity returns what RouteDocument routes a spam filter of either
// apiVersion by.
func SpamFilterIdentity(filter routedAsset, name string, dataset *string) Identity {
	return identityOf("spamfilter", name, DatasetName(dataset), filter)
}

func spamFilterOrigin(filter dash0api.SpamFilterObject) string {
	if asset, ok := filter.(routedAsset); ok {
		return asset.GetOrigin()
	}
	return ""
}

func setSpamFilterOrigin(metadata *dash0api.SpamFilterMetadata, origin string) {
	if metadata.Labels == nil {
		if origin == "" {
			return
		}
		metadata.Labels = &dash0api.SpamFilterLabels{}
	}
	metadata.Labels.Dash0Comorigin = optionalString(origin)
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
