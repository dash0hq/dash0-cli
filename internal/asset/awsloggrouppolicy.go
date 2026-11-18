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
	// AwsLogGroupPolicyKind is the kind of an AWS log-group instrumentation policy document.
	AwsLogGroupPolicyKind = "Dash0AwsLogGroupInstrumentationPolicy"

	// AwsLogGroupPolicyDisplayKind is the name used in messages and diffs.
	AwsLogGroupPolicyDisplayKind = "AWS log-group policy"

	awsLogGroupPolicyAPIVersion = "dash0.com/v1alpha1"
)

// ErrAwsLogGroupPolicyMissingID is returned when an `apply` document does not
// name the AWS integration whose policy it replaces.
var ErrAwsLogGroupPolicyMissingID = errors.New(
	`Dash0AwsLogGroupInstrumentationPolicy requires metadata.labels["dash0.com/id"]: it is the id of the AWS integration whose log-group policy the document replaces`,
)

// awsLogGroupPolicyMetadataKeys lists the metadata a document exported with
// `get -o yaml` carries, so it round-trips without a warning. The CLI sends
// none of it.
var awsLogGroupPolicyMetadataKeys = map[string]map[string]bool{
	"labels": {"dash0.com/id": true, "dash0.com/origin": true, "dash0.com/source": true, "dash0.com/dataset": true, "dash0.com/version": true},
	"annotations": {
		"dash0.com/created-at": true, "dash0.com/updated-at": true,
		"dash0.com/deleted-at": true, "dash0.com/deletion-disposition": true,
	},
}

// ParseAwsLogGroupPolicy decodes and validates a
// Dash0AwsLogGroupInstrumentationPolicy document before any API call. It
// returns warnings for metadata the CLI does not send.
//
// spec.rules is required: the client sends a missing list as [], which stops
// the policy from matching any log group, so a typo must not get that far.
// Unknown fields under spec are rejected for the same reason.
func ParseAwsLogGroupPolicy(raw []byte) (*dash0api.AwsLogGroupInstrumentationPolicy, []string, error) {
	jsonBytes, err := dash0yaml.ToJSON(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse AWS log-group policy definition: %w", err)
	}
	var doc struct {
		APIVersion string                     `json:"apiVersion"`
		Kind       string                     `json:"kind"`
		Metadata   map[string]json.RawMessage `json:"metadata"`
		Spec       map[string]json.RawMessage `json:"spec"`
	}
	if err := json.Unmarshal(jsonBytes, &doc); err != nil {
		return nil, nil, fmt.Errorf("failed to parse AWS log-group policy definition: %w", err)
	}
	if NormalizeKind(doc.Kind) != "awsloggroupinstrumentationpolicy" {
		return nil, nil, fmt.Errorf("kind must be %s, got %q", AwsLogGroupPolicyKind, doc.Kind)
	}
	if doc.APIVersion != awsLogGroupPolicyAPIVersion {
		return nil, nil, fmt.Errorf("apiVersion must be %s, got %q", awsLogGroupPolicyAPIVersion, doc.APIVersion)
	}
	if rules, ok := doc.Spec["rules"]; !ok || string(rules) == "null" {
		return nil, nil, errors.New("spec.rules is required; set it to [] for a policy that matches no log group")
	}

	specJSON, err := json.Marshal(doc.Spec)
	if err != nil {
		return nil, nil, err
	}
	var spec dash0api.AwsLogGroupPolicySpec
	decoder := json.NewDecoder(bytes.NewReader(specJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&spec); err != nil {
		return nil, nil, fmt.Errorf("invalid spec: %w", err)
	}
	if err := validateAwsLogGroupPolicySpec(&spec); err != nil {
		return nil, nil, err
	}

	var policy dash0api.AwsLogGroupInstrumentationPolicy
	if err := json.Unmarshal(jsonBytes, &policy); err != nil {
		return nil, nil, fmt.Errorf("invalid metadata: %w", err)
	}
	policy.ApiVersion = awsLogGroupPolicyAPIVersion
	policy.Kind = AwsLogGroupPolicyKind
	policy.Spec = spec

	return &policy, awsLogGroupPolicyMetadataWarnings(doc.Metadata), nil
}

// validateAwsLogGroupPolicySpec catches the enum mistakes the API would reject,
// so `apply` fails before writing the documents ahead of this one. Operand
// rules stay with the server.
func validateAwsLogGroupPolicySpec(spec *dash0api.AwsLogGroupPolicySpec) error {
	if d := spec.OnNoLongerMatching; d != nil && !d.Valid() {
		return fmt.Errorf("spec.onNoLongerMatching must be %q or %q, got %q", dash0api.AwsLogGroupPolicyDispositionKeep, dash0api.AwsLogGroupPolicyDispositionRemove, *d)
	}
	for i, rule := range spec.Rules {
		for j, condition := range rule.Conditions {
			if !condition.Operator.Valid() {
				return fmt.Errorf("spec.rules[%d].conditions[%d].operator %q is not supported (supported: is, is_one_of, is_set, starts_with, ends_with, contains)", i, j, condition.Operator)
			}
		}
	}
	return nil
}

func awsLogGroupPolicyMetadataWarnings(metadata map[string]json.RawMessage) []string {
	var ignored []string
	for field, value := range metadata {
		if field == "name" {
			continue
		}
		allowed, known := awsLogGroupPolicyMetadataKeys[field]
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
	return []string{fmt.Sprintf("the Dash0 API ignores %s on an AWS log-group policy; only spec is stored", strings.Join(ignored, ", "))}
}

// ImportAwsLogGroupPolicy creates or replaces the log-group policy of an AWS
// integration, and recreates a deleted one.
//
// No metadata is sent. Without dash0.com/version the write is not fenced, so
// the last write wins, which is what a declarative file means. Without
// dash0.com/origin the CLI never conflicts with the origin that owns the policy.
func ImportAwsLogGroupPolicy(
	ctx context.Context,
	apiClient dash0api.Client,
	policy *dash0api.AwsLogGroupInstrumentationPolicy,
	integrationID string,
) (ImportResult, error) {
	plan, err := PlanAwsLogGroupPolicy(ctx, apiClient, policy, integrationID)
	if err != nil {
		return ImportResult{}, err
	}

	body := *policy
	body.Metadata = dash0api.AwsLogGroupPolicyMetadata{}
	result, err := apiClient.UpdateAwsLogGroupInstrumentationPolicy(ctx, integrationID, &body)
	if err != nil {
		return ImportResult{}, err
	}

	return ImportResult{
		Name:   dash0api.GetAwsLogGroupInstrumentationPolicyName(result),
		ID:     integrationID,
		Action: plan.Action,
		Before: plan.Before,
		After:  result,
	}, nil
}

// PlanAwsLogGroupPolicy reads the policy ImportAwsLogGroupPolicy would
// replace. An integration without a policy, or with a deleted one, makes the
// write a create. An unknown integration fails here with a 404 from the write
// path, not the read, because the read cannot tell it apart from "no policy".
func PlanAwsLogGroupPolicy(
	ctx context.Context,
	apiClient dash0api.Client,
	policy *dash0api.AwsLogGroupInstrumentationPolicy,
	integrationID string,
) (ImportPlan, error) {
	plan := ImportPlan{Key: integrationID, Action: ActionCreated, Proposed: policy}
	before, err := apiClient.GetAwsLogGroupInstrumentationPolicy(ctx, integrationID)
	if dash0api.IsNotFound(err) {
		return plan, nil
	}
	if err != nil {
		return plan, err
	}
	if !dash0api.IsAwsLogGroupInstrumentationPolicyDeleted(before) {
		plan.Action, plan.Before = ActionUpdated, before
	}
	return plan, nil
}

// normalizeAwsLogGroupPolicyForDiff keeps only what the CLI writes: the spec,
// with the server's defaults filled in.
func normalizeAwsLogGroupPolicyForDiff(policy *dash0api.AwsLogGroupInstrumentationPolicy) {
	policy.Metadata = dash0api.AwsLogGroupPolicyMetadata{}
	if policy.Spec.OnNoLongerMatching == nil {
		keep := dash0api.AwsLogGroupPolicyDispositionKeep
		policy.Spec.OnNoLongerMatching = &keep
	}
	if policy.Spec.Rules == nil {
		policy.Spec.Rules = []dash0api.AwsLogGroupPolicyRule{}
	}
	for i := range policy.Spec.Rules {
		if policy.Spec.Rules[i].Conditions == nil {
			policy.Spec.Rules[i].Conditions = []dash0api.AwsLogGroupPolicyCondition{}
		}
	}
}
