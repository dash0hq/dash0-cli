package asset

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
)

const (
	// AwsMetricScrapeConfigKind is the kind of an AWS metric scrape config document.
	AwsMetricScrapeConfigKind = "Dash0AwsMetricScrapeConfig"

	awsMetricScrapeConfigAPIVersion = "dash0.com/v1alpha1"
)

// ErrAwsMetricScrapeConfigMissingID is returned when an `apply` document does
// not name the AWS integration whose config it replaces.
var ErrAwsMetricScrapeConfigMissingID = errors.New(
	`Dash0AwsMetricScrapeConfig requires metadata.labels["dash0.com/id"]: it is the id of the AWS integration whose scrape config the document replaces`,
)

// awsMetricScrapeConfigMetadataKeys lists the metadata a document exported with
// `get -o yaml` carries. The server ignores all of it on write, and the CLI does
// not send dash0.com/origin, so these round-trip without a warning. Anything
// else is user intent the API would drop, so ParseAwsMetricScrapeConfig warns.
var awsMetricScrapeConfigMetadataKeys = map[string]map[string]bool{
	"labels":      {"dash0.com/id": true, "dash0.com/origin": true, "dash0.com/source": true, "dash0.com/dataset": true},
	"annotations": {"dash0.com/created-at": true, "dash0.com/updated-at": true},
}

// ParseAwsMetricScrapeConfig decodes and validates a Dash0AwsMetricScrapeConfig
// document before any API call. It returns warnings for metadata the API
// ignores on write.
//
// spec.namespaces is required: the client sends a missing list as [], which
// stops all scraping, so a typo must not get that far. Unknown fields under
// spec are rejected for the same reason.
func ParseAwsMetricScrapeConfig(raw []byte) (*dash0api.AwsMetricScrapeConfigDefinition, []string, error) {
	jsonBytes, err := dash0yaml.ToJSON(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse AWS metric scrape config definition: %w", err)
	}
	var doc struct {
		APIVersion string                     `json:"apiVersion"`
		Kind       string                     `json:"kind"`
		Metadata   map[string]json.RawMessage `json:"metadata"`
		Spec       map[string]json.RawMessage `json:"spec"`
	}
	if err := json.Unmarshal(jsonBytes, &doc); err != nil {
		return nil, nil, fmt.Errorf("failed to parse AWS metric scrape config definition: %w", err)
	}
	if NormalizeKind(doc.Kind) != "awsmetricscrapeconfig" {
		return nil, nil, fmt.Errorf("kind must be %s, got %q", AwsMetricScrapeConfigKind, doc.Kind)
	}
	if doc.APIVersion != awsMetricScrapeConfigAPIVersion {
		return nil, nil, fmt.Errorf("apiVersion must be %s, got %q", awsMetricScrapeConfigAPIVersion, doc.APIVersion)
	}
	if ns, ok := doc.Spec["namespaces"]; !ok || string(ns) == "null" {
		return nil, nil, errors.New("spec.namespaces is required; set it to [] to stop all scraping for the integration")
	}

	specJSON, err := json.Marshal(doc.Spec)
	if err != nil {
		return nil, nil, err
	}
	var spec dash0api.AwsMetricScrapeConfigSpec
	decoder := json.NewDecoder(bytes.NewReader(specJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&spec); err != nil {
		return nil, nil, fmt.Errorf("invalid spec: %w", err)
	}

	var cfg dash0api.AwsMetricScrapeConfigDefinition
	if err := json.Unmarshal(jsonBytes, &cfg); err != nil {
		return nil, nil, fmt.Errorf("invalid metadata: %w", err)
	}
	cfg.ApiVersion = awsMetricScrapeConfigAPIVersion
	cfg.Kind = AwsMetricScrapeConfigKind
	cfg.Spec = spec

	return &cfg, awsMetricScrapeConfigMetadataWarnings(doc.Metadata), nil
}

func awsMetricScrapeConfigMetadataWarnings(metadata map[string]json.RawMessage) []string {
	var ignored []string
	for field, value := range metadata {
		if field == "name" {
			continue
		}
		allowed, known := awsMetricScrapeConfigMetadataKeys[field]
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
	return []string{fmt.Sprintf("the Dash0 API ignores %s on an AWS metric scrape config; only spec is stored", strings.Join(ignored, ", "))}
}

// AwsMetricScrapeConfigIsSet reports whether the integration has a stored
// config. The server omits the annotations of one never written or cleared.
func AwsMetricScrapeConfigIsSet(cfg *dash0api.AwsMetricScrapeConfigDefinition) bool {
	return cfg != nil && cfg.Metadata.Annotations != nil
}

// ImportAwsMetricScrapeConfig replaces the scrape config of an AWS integration.
//
// The metadata is not sent. The server ignores all of it except
// dash0.com/origin, and the CLI does not claim ownership by sending an origin.
// The preflight GET supplies the diff, and fails with a 404 for an unknown
// integration before anything is written.
func ImportAwsMetricScrapeConfig(
	ctx context.Context,
	apiClient dash0api.Client,
	cfg *dash0api.AwsMetricScrapeConfigDefinition,
	integrationID string,
) (ImportResult, error) {
	before, err := apiClient.GetAwsMetricScrapeConfig(ctx, integrationID)
	if err != nil {
		return ImportResult{}, err
	}

	body := *cfg
	body.Metadata = dash0api.AwsMetricScrapeConfigMetadata{}
	result, err := apiClient.UpdateAwsMetricScrapeConfig(ctx, integrationID, &body)
	if err != nil {
		return ImportResult{}, err
	}

	importResult := ImportResult{
		Name:   dash0api.GetAwsMetricScrapeConfigName(result),
		ID:     integrationID,
		Action: ActionCreated,
		After:  result,
	}
	if AwsMetricScrapeConfigIsSet(before) {
		importResult.Action, importResult.Before = ActionUpdated, before
	}
	return importResult, nil
}

// normalizeAwsMetricScrapeConfigForDiff keeps only what the CLI writes: the
// spec, sorted the way the server returns it. The origin is dropped too,
// because the CLI never sends it, so it would only show up as phantom drift.
func normalizeAwsMetricScrapeConfigForDiff(cfg *dash0api.AwsMetricScrapeConfigDefinition) {
	cfg.Metadata = dash0api.AwsMetricScrapeConfigMetadata{}
	sort.SliceStable(cfg.Spec.Namespaces, func(i, j int) bool {
		return cfg.Spec.Namespaces[i].Namespace < cfg.Spec.Namespaces[j].Namespace
	})
	for _, ns := range cfg.Spec.Namespaces {
		sort.Strings(ns.Metrics)
	}
}
