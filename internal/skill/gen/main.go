// Command gen generates internal/skill/content/references/*.md from
// docs/commands.md. Run via `make skill-bundle` (which wraps `go run
// ./internal/skill/gen`), or directly with `-out <dir>` to generate into a
// scratch directory for drift checking (see `make skill-validate`).
//
// docs/commands.md documents some patterns once across several commands
// (the asset CRUD list/get/create/update/delete pattern; query-command
// mechanics like filter syntax) rather than once per command, so this
// generator can't just mechanically split the doc by heading. Instead each
// topic below names exactly which sections/labels it draws from — a
// hand-maintained map that must gain an entry whenever a new command or
// asset kind is added (see docs/adding-commands.md).
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const sourceDoc = "docs/commands.md"

// topicSpec describes how to assemble one reference topic file.
type topicSpec struct {
	name string

	// sections lists heading titles (normalized: backticks stripped,
	// trailing " (experimental)" stripped, case-insensitive) to extract in
	// full, in document order, concatenated with a blank line between them.
	sections []string

	// includeQuickRef, when true, prepends the "Asset type quick reference"
	// table from the Asset CRUD commands section.
	includeQuickRef bool

	// assetYAMLLabels, when non-empty, extracts the labeled YAML example(s)
	// from the "Asset YAML formats" section whose label line contains the
	// given substring (case-sensitive, matched against the label as
	// written in docs/commands.md).
	assetYAMLLabels []string

	// extraNote is hand-authored prose appended after the extracted
	// content, for asset-specific behavior that isn't cleanly extractable
	// from a single heading or label (e.g. CRD import support).
	extraNote string
}

var topics = []topicSpec{
	{name: "apply", sections: []string{"apply"}},
	{name: "api", sections: []string{"api"}},
	{
		name:            "check-rules",
		includeQuickRef: true,
		assetYAMLLabels: []string{"Check rule:"},
		extraNote: "`check-rules create` also accepts PrometheusRule CRD files. Each alerting rule in the CRD " +
			"is created as a separate check rule (recording rules are skipped), named `<group name> - <alert name>`, " +
			"matching the Dash0 Kubernetes operator and the Terraform provider. See the `recording-rules` topic for " +
			"the recording-rule half of a mixed PrometheusRule CRD.",
	},
	{
		name: "config",
		sections: []string{
			"config profiles create",
			"config profiles update",
			"config profiles list",
			"config profiles select",
			"config profiles delete",
			"config show",
		},
	},
	{
		name:            "dashboards",
		includeQuickRef: true,
		assetYAMLLabels: []string{"Dashboard:", "PersesDashboard"},
		extraNote: "`dashboards create` also accepts PersesDashboard CRD files (`perses.dev/v1alpha1` and " +
			"`perses.dev/v1alpha2`).",
	},
	{name: "failed-checks", sections: []string{"failed-checks query"}},
	{name: "login", sections: []string{"login", "logout"}},
	{name: "logs", sections: []string{"logs query", "logs send"}},
	{name: "members", sections: []string{"members list", "members invite", "members remove"}},
	{name: "metrics", sections: []string{"metrics instant"}},
	{
		name: "notification-channels",
		sections: []string{
			"notification-channels list",
			"notification-channels get",
			"notification-channels create",
			"notification-channels update",
			"notification-channels delete",
		},
	},
	{name: "otlp", sections: []string{"otlp proxy"}},
	{
		name:            "recording-rules",
		includeQuickRef: true,
		assetYAMLLabels: []string{"Recording rule", "Mixed PrometheusRule"},
		extraNote: "Recording rules use the PrometheusRule CRD format. A CRD that mixes alerting and recording " +
			"rules is dispatched to both the check-rule and recording-rule endpoints in a single `apply` — see the " +
			"\"Mixed PrometheusRule\" example above.",
	},
	{
		name:            "spam-filters",
		includeQuickRef: true,
		assetYAMLLabels: []string{"Spam filter (v1alpha1", "Spam filter (v1alpha2"},
		extraNote: "The `apiVersion` field on the document selects the schema (`v1alpha1` or `v1alpha2`); a " +
			"missing value defaults to `v1alpha1`. The `list` endpoint returns v1alpha1 definitions only; use " +
			"`spam-filters get <id>` to retrieve a filter in its native apiVersion.",
	},
	{name: "spans", sections: []string{"spans query", "spans send"}},
	{
		name:            "synthetic-checks",
		includeQuickRef: true,
		assetYAMLLabels: []string{"Synthetic check:"},
	},
	{
		name: "teams",
		sections: []string{
			"teams list", "teams get", "teams create", "teams update", "teams delete",
			"teams list-members", "teams add-members", "teams remove-members",
		},
	},
	{name: "traces", sections: []string{"traces get"}},
	{
		name:            "views",
		includeQuickRef: true,
		assetYAMLLabels: []string{"View:"},
	},
}

type heading struct {
	level int
	title string // raw title text, as written after the leading #'s
	start int     // line index of the heading line itself
}

func main() {
	outDir := flag.String("out", "internal/skill/content", "base output directory; references/*.md are written under <out>/references")
	flag.Parse()

	repoRoot, err := findRepoRoot()
	if err != nil {
		fatal(err)
	}

	lines, err := readLines(filepath.Join(repoRoot, sourceDoc))
	if err != nil {
		fatal(fmt.Errorf("failed to read %s: %w", sourceDoc, err))
	}

	headings := parseHeadings(lines)

	quickRef, err := extractSection(lines, headings, "asset type quick reference")
	if err != nil {
		fatal(fmt.Errorf("could not find 'Asset type quick reference' section: %w", err))
	}

	refsDir := *outDir
	if !filepath.IsAbs(refsDir) {
		refsDir = filepath.Join(repoRoot, refsDir)
	}
	refsDir = filepath.Join(refsDir, "references")
	if err := os.MkdirAll(refsDir, 0o755); err != nil {
		fatal(err)
	}

	for _, spec := range topics {
		content, err := buildTopic(spec, lines, headings, quickRef)
		if err != nil {
			fatal(fmt.Errorf("topic %q: %w", spec.name, err))
		}
		content = transform(content)
		content = header(spec.name) + content
		outPath := filepath.Join(refsDir, spec.name+".md")
		if err := os.WriteFile(outPath, []byte(content), 0o644); err != nil {
			fatal(err)
		}
	}
}

func buildTopic(spec topicSpec, lines []string, headings []heading, quickRef string) (string, error) {
	var b strings.Builder

	if spec.includeQuickRef {
		b.WriteString(quickRef)
		b.WriteString("\n\n")
	}

	for _, title := range spec.sections {
		section, err := extractSection(lines, headings, title)
		if err != nil {
			return "", err
		}
		b.WriteString(section)
		b.WriteString("\n\n")
	}

	if len(spec.assetYAMLLabels) > 0 {
		yamlSection, err := extractSection(lines, headings, "asset yaml formats")
		if err != nil {
			return "", fmt.Errorf("could not find 'Asset YAML formats' section: %w", err)
		}
		yamlLines := strings.Split(yamlSection, "\n")
		for _, label := range spec.assetYAMLLabels {
			block, err := extractYAMLBlock(yamlLines, label)
			if err != nil {
				return "", err
			}
			b.WriteString(block)
			b.WriteString("\n\n")
		}
	}

	if spec.extraNote != "" {
		b.WriteString(spec.extraNote)
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n") + "\n", nil
}

// normalizeHeading strips backticks and a trailing " (experimental)"
// qualifier, then lowercases and trims, so headings like
// "### `notification-channels list` (experimental)" compare equal to
// "notification-channels list".
var experimentalSuffix = regexp.MustCompile(`(?i)\s*\(experimental\)\s*$`)

func normalizeHeading(s string) string {
	s = strings.ReplaceAll(s, "`", "")
	s = experimentalSuffix.ReplaceAllString(s, "")
	return strings.ToLower(strings.TrimSpace(s))
}

func parseHeadings(lines []string) []heading {
	headingRe := regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	var out []heading
	for i, line := range lines {
		m := headingRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		out = append(out, heading{level: len(m[1]), title: m[2], start: i})
	}
	return out
}

// extractSection returns the full text (heading line through the line
// before the next heading at the same or shallower level) of the first
// heading whose normalized title equals title, which must already be
// lowercase.
func extractSection(lines []string, headings []heading, title string) (string, error) {
	for i, h := range headings {
		if normalizeHeading(h.title) != title {
			continue
		}
		end := len(lines)
		for j := i + 1; j < len(headings); j++ {
			if headings[j].level <= h.level {
				end = headings[j].start
				break
			}
		}
		return strings.TrimRight(strings.Join(lines[h.start:end], "\n"), "\n"), nil
	}
	return "", fmt.Errorf("heading %q not found", title)
}

// extractYAMLBlock finds the first line in yamlLines that contains label
// and returns that line through the end of the fenced code block(s) that
// immediately follow it (stopping at the next non-code, non-blank line
// after a closing fence).
func extractYAMLBlock(yamlLines []string, label string) (string, error) {
	start := -1
	for i, line := range yamlLines {
		if strings.Contains(line, label) {
			start = i
			break
		}
	}
	if start == -1 {
		return "", fmt.Errorf("label %q not found in Asset YAML formats section", label)
	}

	end := len(yamlLines)
	inFence := false
	sawFence := false
	for i := start + 1; i < len(yamlLines); i++ {
		trimmed := strings.TrimSpace(yamlLines[i])
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			sawFence = true
			if !inFence {
				end = i + 1
			}
			continue
		}
		if !inFence && sawFence {
			if trimmed == "" {
				continue
			}
			end = i
			break
		}
	}

	return strings.TrimRight(strings.Join(yamlLines[start:end], "\n"), "\n"), nil
}

// flagTableRe matches a Markdown table header row for a flag reference
// table, in any of the shapes used across docs/commands.md
// (e.g. "| Flag | Short | Default | Description |").
var flagTableHeaderRe = regexp.MustCompile(`(?i)^\|\s*flag\s*\|`)

const flagTablePointer = "_For the exact, always-current flag list, run `dash0 --agent-mode <command> --help`._"

// transform strips flag tables (replacing each with a pointer to
// `--agent-mode <command> --help`) and rewrites the one non-portable
// cross-reference to documentation.md, mirroring
// .github/workflows/sync-docs/transformations.yaml.
func transform(content string) string {
	lines := strings.Split(content, "\n")
	var out []string
	i := 0
	for i < len(lines) {
		line := lines[i]
		if flagTableHeaderRe.MatchString(line) {
			out = append(out, flagTablePointer)
			i++ // header row
			if i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "|") {
				i++ // separator row
			}
			for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "|") {
				i++ // data rows
			}
			continue
		}
		out = append(out, line)
		i++
	}
	result := strings.Join(out, "\n")
	result = strings.ReplaceAll(
		result,
		"](documentation.md#code-blocks)",
		"](https://github.com/dash0hq/dash0-cli/blob/main/docs/documentation.md#code-blocks)",
	)
	return result
}

func header(topic string) string {
	return fmt.Sprintf(
		"<!-- Generated by internal/skill/gen from docs/commands.md. Do not edit by hand — run `make skill-bundle`. -->\n\n# %s\n\n",
		topic,
	)
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

// findRepoRoot walks up from the current directory looking for go.mod.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find go.mod above %s", dir)
		}
		dir = parent
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "skillgen:", err)
	os.Exit(1)
}
