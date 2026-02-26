package asset

import (
	"fmt"
	"net/url"
	"strings"
)

// Deeplink path patterns per asset type.
const (
	deeplinkPathDashboard      = "/goto/dashboards"
	deeplinkPathCheckRule      = "/goto/alerting/check-rules"
	deeplinkPathSyntheticCheck = "/goto/alerting/synthetics"
	deeplinkPathView           = "/goto/logs"
	deeplinkPathTeam           = "/goto/settings/teams"
	deeplinkPathMember         = "/goto/settings/members"

	// View-type-specific deeplink paths.
	deeplinkPathViewLogs         = "/goto/logs"
	deeplinkPathViewTracing      = "/goto/traces/explorer"
	deeplinkPathViewMetrics      = "/goto/metrics/explorer"
	deeplinkPathViewServiceMap   = "/goto/services/map"
	deeplinkPathViewResources    = "/goto/resources/table"
	deeplinkPathViewFailedChecks = "/goto/alerting/failed-checks"
	deeplinkPathViewWebEvents    = "/goto/web-events/explorer"

	deeplinkQueryDashboard      = "dashboard_id"
	deeplinkQueryCheckRule      = "check_rule_id"
	deeplinkQuerySyntheticCheck = "check_id"
	deeplinkQueryView           = "view_id"
	deeplinkQueryTeam           = "team_id"
	deeplinkQueryMember         = "member_id"
)

// DeeplinkURL constructs a deeplink URL for the given asset type and ID.
// The base URL is derived from the API URL by extracting the domain suffix
// (e.g. "dash0.com" from "api.us-west-2.aws.dash0.com") and prepending "app.".
// Returns an empty string if the API URL is empty or cannot be parsed.
func DeeplinkURL(apiUrl, assetType, assetID string) string {
	baseURL := deeplinkBaseURL(apiUrl)
	if baseURL == "" {
		return ""
	}

	path, queryParam := deeplinkPathAndQuery(assetType)
	if path == "" {
		return ""
	}

	return fmt.Sprintf("%s%s?%s=%s", baseURL, path, queryParam, url.QueryEscape(assetID))
}

// ViewDeeplinkURL constructs a deeplink URL for a view, using the view's type
// to determine the correct path (e.g. /goto/spans for span views, /goto/logs
// for log views). Returns an empty string if the API URL is empty, cannot be
// parsed, or the view type is unknown.
func ViewDeeplinkURL(apiUrl, viewType, assetID string) string {
	baseURL := deeplinkBaseURL(apiUrl)
	if baseURL == "" {
		return ""
	}

	path := viewTypePath(viewType)
	if path == "" {
		return ""
	}

	return fmt.Sprintf("%s%s?%s=%s", baseURL, path, deeplinkQueryView, url.QueryEscape(assetID))
}

// viewTypePath maps a view type string to the corresponding deeplink path.
func viewTypePath(viewType string) string {
	switch strings.ToLower(viewType) {
	case "logs":
		return deeplinkPathViewLogs
	case "spans":
		return deeplinkPathViewTracing
	case "metrics":
		return deeplinkPathViewMetrics
	case "services":
		return deeplinkPathViewServiceMap
	case "resources":
		return deeplinkPathViewResources
	case "failed_checks":
		return deeplinkPathViewFailedChecks
	case "web_events":
		return deeplinkPathViewWebEvents
	default:
		return ""
	}
}

// deeplinkBaseURL extracts the domain suffix from an API URL and returns
// the base URL for deeplinks (e.g. "https://app.dash0.com").
func deeplinkBaseURL(apiUrl string) string {
	if apiUrl == "" {
		return ""
	}

	parsed, err := url.Parse(apiUrl)
	if err != nil || parsed.Host == "" {
		return ""
	}

	suffix := domainSuffix(parsed.Hostname())
	if suffix == "" {
		return ""
	}

	return fmt.Sprintf("https://app.%s", suffix)
}

// domainSuffix extracts the last two labels from a hostname.
// For example, "api.us-west-2.aws.dash0.com" returns "dash0.com".
func domainSuffix(hostname string) string {
	parts := strings.Split(hostname, ".")
	if len(parts) < 2 {
		return ""
	}
	return strings.Join(parts[len(parts)-2:], ".")
}

// deeplinkPathAndQuery returns the URL path and query parameter name for a
// given asset type.
func deeplinkPathAndQuery(assetType string) (string, string) {
	switch strings.ToLower(assetType) {
	case "dashboard":
		return deeplinkPathDashboard, deeplinkQueryDashboard
	case "checkrule", "check rule", "prometheusrule":
		return deeplinkPathCheckRule, deeplinkQueryCheckRule
	case "syntheticcheck", "synthetic check":
		return deeplinkPathSyntheticCheck, deeplinkQuerySyntheticCheck
	case "view":
		return deeplinkPathView, deeplinkQueryView
	case "team":
		return deeplinkPathTeam, deeplinkQueryTeam
	case "member":
		return deeplinkPathMember, deeplinkQueryMember
	default:
		return "", ""
	}
}
