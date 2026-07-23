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
}

// Manifest lists every reference topic bundled with the skill, one per
// top-level dash0 command (excluding `version`, which is trivial enough to
// live only in SKILL.md's own prose).
var Manifest = []ManifestEntry{
	{"apply", "references/apply.md"},
	{"api", "references/api.md"},
	{"check-rules", "references/check-rules.md"},
	{"config", "references/config.md"},
	{"dashboards", "references/dashboards.md"},
	{"failed-checks", "references/failed-checks.md"},
	{"login", "references/login.md"},
	{"logs", "references/logs.md"},
	{"members", "references/members.md"},
	{"metrics", "references/metrics.md"},
	{"notification-channels", "references/notification-channels.md"},
	{"otlp", "references/otlp.md"},
	{"recording-rules", "references/recording-rules.md"},
	{"spam-filters", "references/spam-filters.md"},
	{"spans", "references/spans.md"},
	{"synthetic-checks", "references/synthetic-checks.md"},
	{"teams", "references/teams.md"},
	{"traces", "references/traces.md"},
	{"views", "references/views.md"},
}

// supportedHost describes one Agent Skills host that `dash0 skill install`
// can target: the detected agent slug (see agentmode.DetectAgentSlug), the
// project-relative directory that host's convention expects, and the
// human-readable name shown in error messages.
type supportedHost struct {
	Slug        string
	Dir         string
	DisplayName string
}

// supportedHosts lists every host `dash0 skill install` can install into,
// in a fixed order used for error messages. This is the sole place to add
// a new host later — collapse-into-one-slice avoids the parallel
// map/slice drift trap.
//
// GitHub Copilot's project-level convention is .github/skills/;
// ~/.copilot/skills/ is reserved for personal, home-directory-level
// skills, not project-scoped ones like this.
var supportedHosts = []supportedHost{
	{Slug: "claude-code", Dir: ".claude/skills/dash0-cli", DisplayName: "Claude Code"},
	{Slug: "codex", Dir: ".agents/skills/dash0-cli", DisplayName: "Codex"},
	{Slug: "cursor", Dir: ".cursor/skills/dash0-cli", DisplayName: "Cursor"},
	{Slug: "copilot", Dir: ".github/skills/dash0-cli", DisplayName: "GitHub Copilot"},
}

// findSupportedHost returns the supportedHost entry for slug, or ok=false
// when the slug is unknown (typically because the agent host is detected
// via env vars but not yet a supported install target — e.g. aider, cline,
// windsurf, mcp).
func findSupportedHost(slug string) (supportedHost, bool) {
	for _, h := range supportedHosts {
		if h.Slug == slug {
			return h, true
		}
	}
	return supportedHost{}, false
}

// HostSupported reports whether slug is a supported `skill install` target.
// Callers use this to decide whether nudging the user toward
// `dash0 skill install` would actually help.
func HostSupported(slug string) bool {
	_, ok := findSupportedHost(slug)
	return ok
}

func supportedHostDisplayNames() []string {
	names := make([]string, len(supportedHosts))
	for i, h := range supportedHosts {
		names[i] = h.DisplayName
	}
	return names
}

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
//
// A host is considered installed only when BOTH SKILL.md and the first
// reference topic exist, so a partially written install (e.g., interrupted
// by ENOSPC after SKILL.md landed but before the references directory did)
// does not silently suppress the agent-mode install hint.
func IsInstalled(dir string) bool {
	if len(Manifest) == 0 {
		return false
	}
	// Pick a stable reference topic (first entry in Manifest) as the
	// "references directory present and populated" probe.
	sentinelRef := Manifest[0].RelPath
	for _, h := range supportedHosts {
		hostRoot := filepath.Join(dir, h.Dir)
		if _, err := os.Stat(filepath.Join(hostRoot, "SKILL.md")); err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(hostRoot, sentinelRef)); err != nil {
			continue
		}
		return true
	}
	return false
}
