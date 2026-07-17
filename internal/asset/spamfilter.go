package asset

import (
	"context"
	"fmt"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	sigsyaml "sigs.k8s.io/yaml"
)

// SpamFilterSupportedAPIVersions lists the spam filter apiVersion values the
// CLI understands. Exported so callers can render the supported list in their
// own error messages.
var SpamFilterSupportedAPIVersions = []string{
	string(dash0api.SpamFilterApiVersionV1Alpha1),
	string(dash0api.V1alpha2),
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
	if err := sigsyaml.Unmarshal(data, &disc); err != nil {
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
	case "", string(dash0api.SpamFilterApiVersionV1Alpha1):
		return string(dash0api.SpamFilterApiVersionV1Alpha1), nil
	case string(dash0api.V1alpha2):
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
	id := dash0api.GetSpamFilterID(filter)
	origin := ""
	if filter.Metadata.Labels != nil && filter.Metadata.Labels.Dash0Comorigin != nil {
		origin = *filter.Metadata.Labels.Dash0Comorigin
	}
	dash0api.StripSpamFilterServerFields(filter)

	upsertKey := origin
	if upsertKey == "" {
		upsertKey = id
	}

	action := ActionCreated
	var before any
	if upsertKey != "" {
		existing, err := apiClient.GetSpamFilter(ctx, upsertKey, dataset)
		if err == nil {
			action = ActionUpdated
			before = existing
		}
	}

	var result *dash0api.SpamFilter
	var err error
	if upsertKey != "" {
		result, err = apiClient.UpdateSpamFilter(ctx, upsertKey, filter, dataset)
	} else {
		result, err = apiClient.CreateSpamFilter(ctx, filter, dataset)
	}
	if err != nil {
		return ImportResult{}, err
	}

	resultID := dash0api.GetSpamFilterID(result)
	return ImportResult{Name: dash0api.GetSpamFilterName(result), ID: resultID, Action: action, Before: before, After: result}, nil
}

// ImportSpamFilterV1Alpha2 mirrors ImportSpamFilter for the v1alpha2 schema.
// The v1alpha2 type carries spec.context (scalar) instead of spec.contexts
// (array); the rest of the lifecycle — upsert-key selection (origin first,
// ID fallback), existence check via GetSpamFilter, create-vs-update routing —
// is identical. See ImportSpamFilter for the rationale.
func ImportSpamFilterV1Alpha2(ctx context.Context, apiClient dash0api.Client, filter *dash0api.SpamFilterV1Alpha2, dataset *string) (ImportResult, error) {
	id := v1Alpha2ID(filter)
	origin := ""
	if filter.Metadata.Labels != nil && filter.Metadata.Labels.Dash0Comorigin != nil {
		origin = *filter.Metadata.Labels.Dash0Comorigin
	}

	upsertKey := origin
	if upsertKey == "" {
		upsertKey = id
	}

	action := ActionCreated
	var before any
	if upsertKey != "" {
		existing, err := apiClient.GetSpamFilter(ctx, upsertKey, dataset)
		if err == nil {
			action = ActionUpdated
			before = existing
		}
	}

	var result *dash0api.SpamFilterV1Alpha2
	var err error
	if upsertKey != "" {
		result, err = apiClient.UpdateSpamFilterV1Alpha2(ctx, upsertKey, filter, dataset)
	} else {
		result, err = apiClient.CreateSpamFilterV1Alpha2(ctx, filter, dataset)
	}
	if err != nil {
		return ImportResult{}, err
	}

	return ImportResult{Name: result.Metadata.Name, ID: v1Alpha2ID(result), Action: action, Before: before, After: result}, nil
}

// v1Alpha2ID extracts the dash0.com/id label from a v1alpha2 filter. Kept here
// rather than re-exporting from the api client because the API client only
// exposes labelled accessors for v1alpha1.
func v1Alpha2ID(filter *dash0api.SpamFilterV1Alpha2) string {
	if filter == nil || filter.Metadata.Labels == nil || filter.Metadata.Labels.Dash0Comid == nil {
		return ""
	}
	return *filter.Metadata.Labels.Dash0Comid
}
