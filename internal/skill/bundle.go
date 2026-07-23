// Package skill implements `dash0 skill install` and `dash0 skill show`,
// which distribute an Agent Skill (per the open agentskills.io
// specification) teaching AI coding agents how to use the dash0 CLI.
package skill

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed content/SKILL.md content/references/*.md
var bundleFS embed.FS

const skillMDPath = "content/SKILL.md"

// ManifestEntry describes one reference topic bundled with the skill.
type ManifestEntry struct {
	// Topic is the slug an agent passes to `dash0 skill show <topic>`. It
	// matches the actual top-level `dash0 <command>` name, not an internal
	// taxonomy category.
	Topic string
	// RelPath is the file's path relative to content/, e.g. "references/dashboards.md".
	RelPath string
	// Description is a one-line summary shown in SKILL.md's topic index.
	Description string
}

// Manifest lists every reference topic bundled with the skill, one per
// top-level dash0 command (excluding `version`, which is trivial enough to
// live only in SKILL.md's own prose).
var Manifest = []ManifestEntry{
	{"apply", "references/apply.md", "Create-or-update asset definitions from files, directories, or stdin"},
	{"api", "references/api.md", "Raw HTTP passthrough to any Dash0 API endpoint"},
	{"check-rules", "references/check-rules.md", "Check rule (alerting rule) CRUD, including PrometheusRule CRD import"},
	{"config", "references/config.md", "Profile management (create/update/list/select/delete) and config show"},
	{"dashboards", "references/dashboards.md", "Dashboard CRUD, including PersesDashboard CRD import"},
	{"failed-checks", "references/failed-checks.md", "Query active and historical alerting issues"},
	{"login", "references/login.md", "OAuth 2.0 login/logout and profile authentication states"},
	{"logs", "references/logs.md", "Query and send log records"},
	{"members", "references/members.md", "Organization membership management"},
	{"metrics", "references/metrics.md", "Instant PromQL queries"},
	{"notification-channels", "references/notification-channels.md", "Notification channel CRUD (organization-level, no dataset)"},
	{"otlp", "references/otlp.md", "Local OTLP forwarding proxy (otlp proxy)"},
	{"recording-rules", "references/recording-rules.md", "Recording rule CRUD (PrometheusRule CRD format)"},
	{"spam-filters", "references/spam-filters.md", "Spam filter CRUD (v1alpha1 and v1alpha2)"},
	{"spans", "references/spans.md", "Query and send spans"},
	{"synthetic-checks", "references/synthetic-checks.md", "Synthetic check CRUD"},
	{"teams", "references/teams.md", "Team management and membership"},
	{"traces", "references/traces.md", "Retrieve every span belonging to a trace"},
	{"views", "references/views.md", "View CRUD"},
}

// supportedHosts maps a detected agent slug (see agentmode.DetectAgentSlug)
// to the directory, relative to a base directory, where that host's Agent
// Skills convention expects to find the skill. This is the sole place to
// add a new host later.
var supportedHosts = map[string]string{
	"claude-code": ".claude/skills/dash0-cli",
	"codex":       ".agents/skills/dash0-cli",
	"cursor":      ".cursor/skills/dash0-cli",
	// GitHub Copilot CLI's project-level convention is .github/skills/;
	// ~/.copilot/skills/ is reserved for personal, home-directory-level
	// skills, not project-scoped ones like this.
	"copilot": ".github/skills/dash0-cli",
}

// supportedHostNames lists the supported hosts in a fixed, human-readable
// order for error messages.
var supportedHostNames = []string{"Claude Code", "Codex", "Cursor", "GitHub Copilot"}

func findTopic(topic string) (ManifestEntry, bool) {
	for _, e := range Manifest {
		if e.Topic == topic {
			return e, true
		}
	}
	return ManifestEntry{}, false
}

// TopicNames returns every valid topic name, in Manifest order.
func TopicNames() []string {
	names := make([]string, len(Manifest))
	for i, e := range Manifest {
		names[i] = e.Topic
	}
	return names
}

// SkillMD returns the embedded SKILL.md content.
func SkillMD() (string, error) {
	b, err := bundleFS.ReadFile(skillMDPath)
	if err != nil {
		return "", fmt.Errorf("failed to read embedded SKILL.md: %w", err)
	}
	return string(b), nil
}

// TopicContent returns the embedded reference content for the given topic.
func TopicContent(topic string) (string, error) {
	entry, ok := findTopic(topic)
	if !ok {
		return "", fmt.Errorf(
			"unknown skill topic %q\nHint: valid topics are: %s",
			topic, strings.Join(TopicNames(), ", "),
		)
	}
	b, err := bundleFS.ReadFile("content/" + entry.RelPath)
	if err != nil {
		return "", fmt.Errorf("failed to read embedded reference %q: %w", entry.RelPath, err)
	}
	return string(b), nil
}

// IsInstalled reports whether the skill bundle appears to already be
// installed under dir, checking every supported host's directory. This is a
// broader "has this project been set up at all" check, independent of
// which single host `skill install` would target on any given run.
func IsInstalled(dir string) bool {
	for _, hostDir := range supportedHosts {
		if _, err := os.Stat(filepath.Join(dir, hostDir, "SKILL.md")); err == nil {
			return true
		}
	}
	return false
}
