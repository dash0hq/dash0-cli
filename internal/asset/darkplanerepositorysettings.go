package asset

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
)

// DarkplaneRepositorySettingsKind is the kind of an AutoMerge repository settings document.
const DarkplaneRepositorySettingsKind = "DarkplaneRepositorySettings"

// Limits the API enforces on path rules.
const (
	maxPathRules    = 50
	maxPathsPerRule = 50
)

// darkplaneRepositorySettingsRequiredSpecFields are the spec fields the PUT replaces with a zero
// value when absent. A missing softRiskCriteria would wipe the criteria, so a typo must not get
// that far.
var darkplaneRepositorySettingsRequiredSpecFields = []string{
	"botMode", "triggerMode", "waitForChecks", "softThreshold", "softRiskCriteria",
}

// darkplaneRepositorySettingsMetadataKeys lists the metadata `get -o yaml` exports. The server
// reads only dash0.com/version on write; the rest round-trips without a warning.
var darkplaneRepositorySettingsMetadataKeys = map[string]map[string]bool{
	"labels":      {"dash0.com/id": true, "dash0.com/version": true},
	"annotations": {"dash0.com/created-at": true, "dash0.com/updated-at": true},
}

// ParseDarkplaneRepositorySettings decodes and validates a DarkplaneRepositorySettings document
// before any API call, and derives the deprecated spec.protectedPaths from the protected path
// rules. It returns warnings for metadata the API ignores and for protected paths it folded
// into a rule.
func ParseDarkplaneRepositorySettings(raw []byte) (*dash0api.RepositorySettingsDefinition, []string, error) {
	jsonBytes, err := dash0yaml.ToJSON(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse AutoMerge settings definition: %w", err)
	}
	var doc struct {
		Kind     string                     `json:"kind"`
		Metadata map[string]json.RawMessage `json:"metadata"`
		Spec     map[string]json.RawMessage `json:"spec"`
	}
	if err := json.Unmarshal(jsonBytes, &doc); err != nil {
		return nil, nil, fmt.Errorf("failed to parse AutoMerge settings definition: %w", err)
	}
	if NormalizeKind(doc.Kind) != NormalizeKind(DarkplaneRepositorySettingsKind) {
		return nil, nil, fmt.Errorf("kind must be %s, got %q", DarkplaneRepositorySettingsKind, doc.Kind)
	}
	if doc.Spec == nil {
		return nil, nil, errors.New("spec is required: an update replaces the whole settings spec")
	}
	var missing []string
	for _, field := range darkplaneRepositorySettingsRequiredSpecFields {
		if value, ok := doc.Spec[field]; !ok || string(value) == "null" {
			missing = append(missing, "spec."+field)
		}
	}
	if len(missing) > 0 {
		return nil, nil, fmt.Errorf("%s required: an update replaces the whole settings spec, so export the current settings with 'get -o yaml' and edit that", strings.Join(missing, ", "))
	}

	specJSON, err := json.Marshal(doc.Spec)
	if err != nil {
		return nil, nil, err
	}
	var spec dash0api.RepositorySettingsSpec
	decoder := json.NewDecoder(bytes.NewReader(specJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&spec); err != nil {
		return nil, nil, fmt.Errorf("invalid spec: %w", err)
	}

	var settings dash0api.RepositorySettingsDefinition
	if err := json.Unmarshal(jsonBytes, &settings); err != nil {
		return nil, nil, fmt.Errorf("invalid metadata: %w", err)
	}
	settings.Kind = dash0api.RepositorySettingsDefinitionKind(DarkplaneRepositorySettingsKind)
	settings.UsesDefaultCriteria = nil

	warnings := darkplaneRepositorySettingsMetadataWarnings(doc.Metadata)
	rules, protectedPaths, folded := protectedPathFields(spec.PathRules, spec.ProtectedPaths)
	if len(folded) > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"spec.protectedPaths is deprecated: added a protected path rule for %s, which no protected rule covered. To stop protecting a path, remove it from spec.protectedPaths too",
			strings.Join(folded, ", ")))
	}
	spec.PathRules = rules
	spec.ProtectedPaths = protectedPaths
	settings.Spec = spec

	return &settings, warnings, nil
}

func darkplaneRepositorySettingsMetadataWarnings(metadata map[string]json.RawMessage) []string {
	var ignored []string
	for field, value := range metadata {
		if field == "name" {
			continue
		}
		allowed, known := darkplaneRepositorySettingsMetadataKeys[field]
		if !known {
			ignored = append(ignored, "metadata."+field)
			continue
		}
		var entries map[string]json.RawMessage
		if json.Unmarshal(value, &entries) != nil {
			continue // a non-map value already failed the typed decode
		}
		for key := range entries {
			if !allowed[key] {
				ignored = append(ignored, fmt.Sprintf("metadata.%s[%q]", field, key))
			}
		}
	}
	if len(ignored) == 0 {
		return nil
	}
	sort.Strings(ignored)
	return []string{fmt.Sprintf("the Dash0 API ignores %s on AutoMerge settings; only spec and the dash0.com/version label are read", strings.Join(ignored, ", "))}
}

// protectedPathFields returns the path rules and the deprecated protectedPaths to write, the same
// way the Dash0 settings editor saves them. A protected path no protected rule covers is folded
// into a new protected rule, so a path is never silently un-protected; protectedPaths then mirrors
// the protected rules that name no authors, plus whatever the rule cap left unfolded. The API
// asks writers to keep protectedPaths in step with the protected rules rather than empty it.
func protectedPathFields(
	authored *[]dash0api.RepositoryPathRule,
	protectedPaths []string,
) (rules *[]dash0api.RepositoryPathRule, mirrored []string, folded []string) {
	var stored []dash0api.RepositoryPathRule
	if authored != nil {
		stored = normalizePathRules(*authored)
	}

	covered := map[string]bool{}
	for _, path := range mirroredProtectedPaths(stored) {
		covered[path] = true
	}
	var uncovered []string
	for _, path := range protectedPaths {
		path = trimPathPattern(path)
		if path != "" && !covered[path] && !slices.Contains(uncovered, path) {
			uncovered = append(uncovered, path)
		}
	}

	var unfolded []string
	budget := max(0, maxPathRules-len(stored))
	for start := 0; start < len(uncovered); start += maxPathsPerRule {
		chunk := uncovered[start:min(start+maxPathsPerRule, len(uncovered))]
		if budget == 0 {
			unfolded = append(unfolded, chunk...)
			continue
		}
		stored = append(stored, dash0api.RepositoryPathRule{Level: dash0api.PathRuleLevelProtected, Paths: chunk})
		folded = append(folded, chunk...)
		budget--
	}

	mirrored = mirroredProtectedPaths(stored)
	for _, path := range unfolded {
		if !slices.Contains(mirrored, path) {
			mirrored = append(mirrored, path)
		}
	}
	if mirrored == nil {
		mirrored = []string{}
	}
	if authored == nil && len(stored) == 0 {
		return nil, mirrored, folded
	}
	if stored == nil {
		stored = []dash0api.RepositoryPathRule{}
	}
	return &stored, mirrored, folded
}

// mirroredProtectedPaths leaves out author-scoped protected rules: protectedPaths has no author
// dimension, so mirroring one would protect the path from everyone.
func mirroredProtectedPaths(rules []dash0api.RepositoryPathRule) []string {
	var paths []string
	for _, rule := range rules {
		if rule.Level != dash0api.PathRuleLevelProtected || (rule.Authors != nil && len(*rule.Authors) > 0) {
			continue
		}
		for _, path := range rule.Paths {
			if path != "" && !slices.Contains(paths, path) {
				paths = append(paths, path)
			}
		}
	}
	return paths
}

func normalizePathRules(rules []dash0api.RepositoryPathRule) []dash0api.RepositoryPathRule {
	out := make([]dash0api.RepositoryPathRule, 0, len(rules))
	for _, rule := range rules {
		paths := make([]string, 0, len(rule.Paths))
		for _, path := range rule.Paths {
			if path = trimPathPattern(path); path != "" {
				paths = append(paths, path)
			}
		}
		if len(paths) == 0 {
			continue
		}
		rule.Paths = paths
		if rule.Name != nil {
			if name := strings.TrimSpace(*rule.Name); name != "" {
				rule.Name = &name
			} else {
				rule.Name = nil
			}
		}
		out = append(out, rule)
	}
	return out
}

func trimPathPattern(pattern string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.In(r, unicode.C, unicode.Zl, unicode.Zp) {
			return -1
		}
		return r
	}, pattern))
}

// DarkplaneRepositorySettingsID returns the repository id the document names, or "".
func DarkplaneRepositorySettingsID(settings *dash0api.RepositorySettingsDefinition) string {
	return dash0api.GetDarkplaneRepositorySettingsID(settings)
}
