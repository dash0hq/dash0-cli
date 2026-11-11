package asset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
)

// GenerativeAiEvaluationDisplayName names a generative AI evaluation in user-facing output.
const GenerativeAiEvaluationDisplayName = "Eval"

// generativeAiEvaluationRequiredFilterKey is the attribute that every eval must match with an
// is_set filter without a value. An empty values list counts as no values, as everywhere else in
// the CLI and the API. The API rejects a create or update without that filter.
const generativeAiEvaluationRequiredFilterKey = "gen_ai.operation.name"

// ParseGenerativeAiEvaluation decodes an eval document and checks what every eval entry point
// needs before an API call: the fields the API rejects when empty, the required
// gen_ai.operation.name is_set filter, and a sample rate between 0 and 1. The check runs before the
// first write, so a multi-document apply never writes some documents and then fails on a problem
// that the CLI can find by itself. spec.display.name and spec.judge.systemPrompt may be empty, and
// the model and output kind are not checked against a list, so a value the API adds later still
// passes.
//
// spec.enabled and spec.sample.rate must be present. The client types hold them as a plain bool and
// float32, so a document without them would be sent as enabled: false and rate: 0, an eval that
// never samples a trace.
func ParseGenerativeAiEvaluation(data []byte) (*dash0api.GenerativeAiEvaluationDefinition, error) {
	var evaluation dash0api.GenerativeAiEvaluationDefinition
	if err := dash0yaml.Unmarshal(data, &evaluation); err != nil {
		return nil, fmt.Errorf("failed to parse eval definition: %w", err)
	}
	var present struct {
		Spec struct {
			Enabled *bool `json:"enabled"`
			Sample  struct {
				Rate *float64 `json:"rate"`
			} `json:"sample"`
		} `json:"spec"`
	}
	if err := dash0yaml.Unmarshal(data, &present); err != nil {
		return nil, fmt.Errorf("failed to parse eval definition: %w", err)
	}
	if err := validateGenerativeAiEvaluation(&evaluation); err != nil {
		return nil, err
	}
	if present.Spec.Enabled == nil {
		return nil, errors.New("eval definition requires spec.enabled")
	}
	if present.Spec.Sample.Rate == nil {
		return nil, errors.New("eval definition requires spec.sample.rate")
	}
	return &evaluation, nil
}

func validateGenerativeAiEvaluation(evaluation *dash0api.GenerativeAiEvaluationDefinition) error {
	output, err := evaluation.Spec.Judge.Output.MarshalJSON()
	if err != nil {
		return fmt.Errorf("invalid spec.judge.output: %w", err)
	}
	var outputKind struct {
		Kind string `json:"kind"`
	}
	// A missing output marshals to "null", which decodes to an empty kind.
	_ = json.Unmarshal(output, &outputKind)

	for _, field := range []struct{ path, value string }{
		{"metadata.name", evaluation.Metadata.Name},
		{"spec.judge.model", string(evaluation.Spec.Judge.Model)},
		{"spec.judge.output.kind", outputKind.Kind},
		{"spec.judge.userPrompt", evaluation.Spec.Judge.UserPrompt},
	} {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("eval definition requires %s", field.path)
		}
	}

	if rate := evaluation.Spec.Sample.Rate; rate < 0 || rate > 1 {
		return fmt.Errorf("eval spec.sample.rate must be between 0 and 1, got %v", rate)
	}

	for _, filter := range evaluation.Spec.Match.Filters {
		hasValues := filter.Values != nil && len(*filter.Values) > 0
		if filter.Key == generativeAiEvaluationRequiredFilterKey && filter.Operator == dash0api.AttributeFilterOperatorIsSet &&
			filter.Value == nil && !hasValues {
			return nil
		}
	}
	return fmt.Errorf("eval spec.match.filters requires the filter {key: %s, operator: is_set} without a value", generativeAiEvaluationRequiredFilterKey)
}

// GenerativeAiEvaluationUsesOrigin reports whether an eval document carries a non-empty
// dash0.com/origin label. --since uses it the same way as SLOUsesOrigin: eval ids are
// server-assigned, so an id-only document that took ImportGenerativeAiEvaluation's POST
// fallback left the live eval at an id the document never learned.
func GenerativeAiEvaluationUsesOrigin(data []byte) (bool, error) {
	var evaluation dash0api.GenerativeAiEvaluationDefinition
	if err := dash0yaml.Unmarshal(data, &evaluation); err != nil {
		return false, fmt.Errorf("failed to decode eval: %w", err)
	}
	return dash0api.GetGenerativeAiEvaluationOrigin(&evaluation) != "", nil
}

// ImportGenerativeAiEvaluation creates or upserts a generative AI evaluation.
//
// The upsert keys follow ImportSLO:
//
//   - With a dash0.com/origin label, the eval is PUT to that origin. PUT to an unknown origin
//     creates the eval, so the preflight GET only decides "created" vs "updated" and supplies
//     the diff; any error other than a 404 is surfaced.
//   - With only a dash0.com/id label, a preflight GET decides the route: PUT on hit, POST on a
//     404 (the id belongs to another organization), and any other error is surfaced so a
//     transient failure never creates a duplicate.
//   - With neither, POST creates the eval.
//
// The identifiers are read before StripGenerativeAiEvaluationServerFields, which removes
// dash0.com/id (the server rejects a create that sets it) and dash0.com/version. Without the
// version, the PUT overwrites unconditionally. The strip keeps dash0.com/origin, so the body
// of an origin PUT carries the same origin as the path.
func ImportGenerativeAiEvaluation(ctx context.Context, apiClient dash0api.Client, evaluation *dash0api.GenerativeAiEvaluationDefinition, dataset *string) (ImportResult, error) {
	id := dash0api.GetGenerativeAiEvaluationID(evaluation)
	plan, err := PlanGenerativeAiEvaluation(ctx, apiClient, evaluation, dataset)
	if err != nil {
		return ImportResult{}, err
	}

	var result *dash0api.GenerativeAiEvaluationDefinition
	if plan.Key != "" {
		result, err = apiClient.UpdateGenerativeAiEvaluation(ctx, plan.Key, evaluation, dataset)
	} else {
		result, err = apiClient.CreateGenerativeAiEvaluation(ctx, evaluation, dataset)
	}
	if err != nil && plan.Key != "" && plan.Before == nil && dash0api.IsNotFound(err) {
		// A PUT to an origin that this dataset does not have creates the eval, unless another
		// dataset already uses the origin: origins are unique per organization, while each eval
		// belongs to one dataset. The API then answers 404, which reads as a missing eval.
		// The cause is wrapped with %v, so client.HandleAPIError does not replace this message
		// with a generic "not found".
		return ImportResult{}, fmt.Errorf(
			"eval origin %q already exists in another dataset:\n  %v"+
				"\nHint: origins are unique per organization while each eval belongs to one dataset, "+
				"so one document cannot be applied to two datasets. Use a distinct %s per dataset, "+
				"for example %q",
			plan.Key, err, OriginLabel, plan.Key+"-staging",
		)
	}
	if err != nil {
		return ImportResult{}, err
	}

	if resultID := dash0api.GetGenerativeAiEvaluationID(result); resultID != "" {
		id = resultID
	}
	name := dash0api.GetGenerativeAiEvaluationName(result)
	if err := RequireNotDeleted(GenerativeAiEvaluationDisplayName, name, id, GenerativeAiEvaluationDeletedAt(result)); err != nil {
		return ImportResult{}, err
	}
	return ImportResult{Name: name, ID: id, Action: plan.Action, Before: plan.Before, After: result}, nil
}

// PlanGenerativeAiEvaluation reads the stored eval that ImportGenerativeAiEvaluation would write.
// An empty Key means a POST: the document has no origin, and its id is unknown or missing.
func PlanGenerativeAiEvaluation(ctx context.Context, apiClient dash0api.Client, evaluation *dash0api.GenerativeAiEvaluationDefinition, dataset *string) (ImportPlan, error) {
	origin := dash0api.GetGenerativeAiEvaluationOrigin(evaluation)
	id := dash0api.GetGenerativeAiEvaluationID(evaluation)
	dash0api.StripGenerativeAiEvaluationServerFields(evaluation)

	plan := ImportPlan{Action: ActionCreated, Proposed: evaluation}
	switch {
	case origin != "":
		// PUT to an unknown origin creates the eval, so a 404 keeps the origin as the key.
		plan.Key = origin
		existing, err := apiClient.GetGenerativeAiEvaluation(ctx, origin, dataset)
		switch {
		case err == nil:
			plan.Action, plan.Before = actionForExisting(GenerativeAiEvaluationDeletedAt(existing)), existing
		case dash0api.IsNotFound(err):
		default:
			// Keep the plan, so a caller that drops a denied read still names the eval by its origin.
			return plan, err
		}
	case id != "":
		existing, err := apiClient.GetGenerativeAiEvaluation(ctx, id, dataset)
		switch {
		case err == nil:
			plan.Key = id
			plan.Action, plan.Before = actionForExisting(GenerativeAiEvaluationDeletedAt(existing)), existing
		case dash0api.IsNotFound(err):
			// The id belongs to another organization: POST creates the eval.
		default:
			return plan, err
		}
	}
	return plan, nil
}

// GenerativeAiEvaluationDeletedAt returns when the eval was deleted, or nil when it is not deleted.
func GenerativeAiEvaluationDeletedAt(evaluation *dash0api.GenerativeAiEvaluationDefinition) *time.Time {
	if evaluation == nil || evaluation.Metadata.Annotations == nil {
		return nil
	}
	return evaluation.Metadata.Annotations.Dash0ComdeletedAt
}

// ExplainGenerativeAiEvaluationError adds the cause and the fix to the two 400s a write to an
// existing eval hits when the document changes a field that is fixed at create: metadata.name
// and spec.judge.output.kind. It returns nil for any other error, so the caller falls back to
// client.HandleAPIError.
//
// The advice goes behind "\nHint:" so agent mode lifts it into the JSON error's hint field.
func ExplainGenerativeAiEvaluationError(err error, name string) error {
	var apiErr *dash0api.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest {
		return nil
	}
	// Matching on the text is unavoidable, because the errors carry no machine-readable discriminator.
	haystack := strings.ToLower(apiErr.Message + " " + apiErr.Body)
	if !strings.Contains(haystack, "cannot change after create") {
		return nil
	}
	switch {
	case strings.Contains(haystack, "metadata.name"):
		return fmt.Errorf(
			"eval %q: metadata.name cannot change after the eval is created:\n  %w"+
				"\nHint: metadata.name identifies the eval and its results. Restore the stored metadata.name "+
				"(`dash0 -X evals get <origin-or-id> -o yaml` shows it), and change spec.display.name to rename the eval in Dash0. "+
				"To use a new metadata.name, create a new eval with a different %s",
			name, err, OriginLabel,
		)
	case strings.Contains(haystack, "output.kind"):
		return fmt.Errorf(
			"eval %q: spec.judge.output.kind cannot change after the eval is created:\n  %w"+
				"\nHint: results are stored per output kind. Restore the stored spec.judge.output.kind, "+
				"or create a new eval with a different metadata.name and %s for the new output kind",
			name, err, OriginLabel,
		)
	default:
		return nil
	}
}
