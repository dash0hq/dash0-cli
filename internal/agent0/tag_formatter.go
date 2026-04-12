package agent0

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// tagPattern matches paired XML-like tags: <TagName attr='val'></TagName>
// Go's regexp2 doesn't support backreferences, so we match opening + any closing tag
// and validate the tag name match in code.
var tagPattern = regexp.MustCompile(`<([A-Z][a-zA-Z]*)\s*([^>]*)>\s*</([A-Z][a-zA-Z]*)>`)

// attrPattern matches single-quoted attributes: key='value'
var attrPattern = regexp.MustCompile(`(\w+)='([^']*)'`)

// formatTags replaces agent0 custom XML-like tags with terminal-friendly text.
// This runs before markdown rendering so the tags are consumed, not escaped.
func formatTags(content string) string {
	return tagPattern.ReplaceAllStringFunc(content, func(match string) string {
		groups := tagPattern.FindStringSubmatch(match)
		if groups == nil {
			return match
		}

		tagName := groups[1]
		attrStr := groups[2]
		closingTag := groups[3]

		// Verify opening and closing tags match
		if tagName != closingTag {
			return match
		}

		attrs := parseAttrs(attrStr)

		switch tagName {
		case "Service":
			return formatService(attrs)
		case "TimeRange", "Timerange":
			return formatTimeRange(attrs)
		case "Metric":
			return formatMetric(attrs)
		case "Promql":
			return formatPromql(attrs)
		case "Document":
			return formatDocument(attrs)
		case "Dashboard":
			return formatDashboard(attrs)
		case "Span":
			return formatSpan(attrs)
		case "Log":
			return formatLog(attrs)
		case "Pod":
			return formatPod(attrs)
		case "FailedCheck":
			return formatFailedCheck(attrs)
		case "LinearIssue":
			return formatLinearIssue(attrs)
		case "LinearProject":
			return formatLinearProject(attrs)
		case "LinearTeam":
			return formatLinearTeam(attrs)
		case "Github":
			return formatGithub(attrs)
		case "Filters":
			return formatFilters(attrs)
		case "Website":
			return formatWebsite(attrs)
		case "Session":
			return formatSession(attrs)
		case "Workflow":
			return formatWorkflow(attrs)
		default:
			// Unknown tag: show tag name + first attribute value
			return formatGeneric(tagName, attrs)
		}
	})
}

func parseAttrs(s string) map[string]string {
	attrs := make(map[string]string)
	for _, m := range attrPattern.FindAllStringSubmatch(s, -1) {
		attrs[m[1]] = m[2]
	}
	return attrs
}

func formatService(attrs map[string]string) string {
	name := attrs["name"]
	ns := attrs["namespace"]
	if ns != "" {
		return fmt.Sprintf("`%s` (%s)", name, ns)
	}
	return fmt.Sprintf("`%s`", name)
}

func formatTimeRange(attrs map[string]string) string {
	from, errFrom := time.Parse(time.RFC3339Nano, attrs["from"])
	to, errTo := time.Parse(time.RFC3339Nano, attrs["to"])
	if errFrom != nil || errTo != nil {
		// Fallback: show raw values
		return fmt.Sprintf("%s – %s", attrs["from"], attrs["to"])
	}

	fromLocal := from.Local()
	toLocal := to.Local()

	if fromLocal.Day() == toLocal.Day() && fromLocal.Month() == toLocal.Month() {
		return fmt.Sprintf("%s – %s", fromLocal.Format("15:04"), toLocal.Format("15:04"))
	}
	return fmt.Sprintf("%s – %s", fromLocal.Format("Jan 2 15:04"), toLocal.Format("Jan 2 15:04"))
}

func formatMetric(attrs map[string]string) string {
	name := attrs["name"]
	metricType := attrs["type"]
	if metricType != "" {
		return fmt.Sprintf("`%s` (%s)", name, metricType)
	}
	return fmt.Sprintf("`%s`", name)
}

func formatPromql(attrs map[string]string) string {
	return fmt.Sprintf("`%s`", attrs["expression"])
}

func formatDocument(attrs map[string]string) string {
	title := attrs["title"]
	url := attrs["url"]
	if title != "" && url != "" {
		return fmt.Sprintf("[%s](%s)", title, url)
	}
	if title != "" {
		return title
	}
	return url
}

func formatDashboard(attrs map[string]string) string {
	id := attrs["dashboardId"]
	return fmt.Sprintf("Dashboard `%s`", id)
}

func formatSpan(attrs map[string]string) string {
	spanID := attrs["spanId"]
	if spanID == "" {
		spanID = attrs["id"]
	}
	traceID := attrs["traceId"]
	if traceID != "" {
		return fmt.Sprintf("Span `%s` (trace `%s`)", spanID, traceID)
	}
	return fmt.Sprintf("Span `%s`", spanID)
}

func formatLog(attrs map[string]string) string {
	return fmt.Sprintf("Log `%s`", attrs["id"])
}

func formatPod(attrs map[string]string) string {
	name := attrs["name"]
	ns := attrs["namespace"]
	if ns != "" {
		return fmt.Sprintf("`%s` (%s)", name, ns)
	}
	return fmt.Sprintf("`%s`", name)
}

func formatFailedCheck(attrs map[string]string) string {
	return fmt.Sprintf("Failed check `%s`", attrs["issueIdentifier"])
}

func formatLinearIssue(attrs map[string]string) string {
	return attrs["identifier"]
}

func formatLinearProject(attrs map[string]string) string {
	name := attrs["projectName"]
	if name != "" {
		return name
	}
	return attrs["projectId"]
}

func formatLinearTeam(attrs map[string]string) string {
	name := attrs["teamName"]
	key := attrs["teamKey"]
	if name != "" && key != "" {
		return fmt.Sprintf("%s (%s)", name, key)
	}
	if name != "" {
		return name
	}
	return key
}

func formatGithub(attrs map[string]string) string {
	url := attrs["url"]
	// Shorten GitHub URLs
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	return url
}

func formatFilters(attrs map[string]string) string {
	raw := attrs["filters"]
	if raw == "" {
		return "filters"
	}
	// Show a simplified version — the JSON is too verbose for inline display
	return fmt.Sprintf("filters: `%s`", truncateStr(raw, 60))
}

func formatWebsite(attrs map[string]string) string {
	name := attrs["serviceName"]
	path := attrs["pageUrlPath"]
	if path != "" {
		return fmt.Sprintf("`%s` (%s)", name, path)
	}
	return fmt.Sprintf("`%s`", name)
}

func formatSession(attrs map[string]string) string {
	return fmt.Sprintf("Session `%s`", attrs["sessionId"])
}

func formatWorkflow(attrs map[string]string) string {
	kind := attrs["kind"]
	if kind != "" {
		return fmt.Sprintf("Workflow (%s)", kind)
	}
	return "Workflow"
}

func formatGeneric(tagName string, attrs map[string]string) string {
	// Show tag name + first meaningful attribute value
	for _, key := range []string{"name", "id", "identifier", "title", "url"} {
		if v, ok := attrs[key]; ok {
			return fmt.Sprintf("%s: `%s`", tagName, v)
		}
	}
	return tagName
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
