package spamfilters

import (
	"fmt"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/asset"
	sigsyaml "sigs.k8s.io/yaml"
)

// detectAPIVersion is a local alias for asset.DetectSpamFilterAPIVersion.
// Kept as a thin wrapper so existing tests and callers in this package
// continue to compile.
func detectAPIVersion(data []byte) (string, error) {
	return asset.DetectSpamFilterAPIVersion(data)
}

// supportedAPIVersions exposes the api-version list defined in
// internal/asset for callers within the spamfilters package.
var supportedAPIVersions = asset.SpamFilterSupportedAPIVersions

// joinQuoted renders the values as a comma-separated list with each value
// wrapped in double quotes, suitable for embedding in error messages.
func joinQuoted(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = fmt.Sprintf("%q", v)
	}
	return strings.Join(quoted, ", ")
}

// objectMetadata returns the SpamFilterMetadata that is present on every
// concrete spam filter type, regardless of apiVersion. This lets the rest
// of the package access the name and labels without re-running the type
// switch at every callsite.
func objectMetadata(obj dash0api.SpamFilterObject) dash0api.SpamFilterMetadata {
	switch v := obj.(type) {
	case *dash0api.SpamFilter:
		return v.Metadata
	case *dash0api.SpamFilterV1Alpha2:
		return v.Metadata
	default:
		return dash0api.SpamFilterMetadata{}
	}
}

// objectAPIVersion returns the apiVersion string for a spam filter object.
// For v1alpha1 the field is optional on the wire, so an absent value is
// reported as v1alpha1 — the version the type itself represents.
func objectAPIVersion(obj dash0api.SpamFilterObject) string {
	var raw string
	switch v := obj.(type) {
	case *dash0api.SpamFilter:
		if v.ApiVersion != nil {
			raw = string(*v.ApiVersion)
		}
	case *dash0api.SpamFilterV1Alpha2:
		raw = string(v.ApiVersion)
	default:
		return ""
	}
	if raw == "" {
		return string(dash0api.SpamFilterApiVersionV1Alpha1V1alpha1)
	}
	normalized, ok := dash0api.NormalizeDash0ApiVersion(raw)
	if !ok {
		return raw
	}
	return normalized
}

// objectKind returns the metadata.kind on a spam filter object as a string.
func objectKind(obj dash0api.SpamFilterObject) string {
	switch v := obj.(type) {
	case *dash0api.SpamFilter:
		return string(v.Kind)
	case *dash0api.SpamFilterV1Alpha2:
		return string(v.Kind)
	default:
		return ""
	}
}

// objectName returns the metadata.name on a spam filter object.
func objectName(obj dash0api.SpamFilterObject) string {
	return objectMetadata(obj).Name
}

// objectID returns the dash0.com/id label value, or "" if not set.
func objectID(obj dash0api.SpamFilterObject) string {
	labels := objectMetadata(obj).Labels
	if labels == nil || labels.Dash0Comid == nil {
		return ""
	}
	return *labels.Dash0Comid
}

// objectOrigin returns the dash0.com/origin label value, or "" if not set.
func objectOrigin(obj dash0api.SpamFilterObject) string {
	labels := objectMetadata(obj).Labels
	if labels == nil || labels.Dash0Comorigin == nil {
		return ""
	}
	return *labels.Dash0Comorigin
}

// objectDataset returns the dash0.com/dataset label value, or "" if not set.
func objectDataset(obj dash0api.SpamFilterObject) string {
	labels := objectMetadata(obj).Labels
	if labels == nil || labels.Dash0Comdataset == nil {
		return ""
	}
	return *labels.Dash0Comdataset
}

// objectFilterCount returns the number of filter criteria attached to a
// spam filter object, regardless of apiVersion.
func objectFilterCount(obj dash0api.SpamFilterObject) int {
	switch v := obj.(type) {
	case *dash0api.SpamFilter:
		return len(v.Spec.Filter)
	case *dash0api.SpamFilterV1Alpha2:
		return len(v.Spec.Filter)
	default:
		return 0
	}
}

// decodeV1Alpha1 unmarshals raw YAML/JSON into the v1alpha1 spam filter
// type. The version is included in the error so users distinguish a
// malformed v1alpha1 document from a version-mismatch.
func decodeV1Alpha1(data []byte) (*dash0api.SpamFilter, error) {
	var filter dash0api.SpamFilter
	if err := sigsyaml.Unmarshal(data, &filter); err != nil {
		return nil, fmt.Errorf("failed to parse v1alpha1 spam filter: %w", err)
	}
	return &filter, nil
}

// decodeV1Alpha2 unmarshals raw YAML/JSON into the v1alpha2 spam filter
// type. Same error-tagging rationale as decodeV1Alpha1.
func decodeV1Alpha2(data []byte) (*dash0api.SpamFilterV1Alpha2, error) {
	var filter dash0api.SpamFilterV1Alpha2
	if err := sigsyaml.Unmarshal(data, &filter); err != nil {
		return nil, fmt.Errorf("failed to parse v1alpha2 spam filter: %w", err)
	}
	return &filter, nil
}

