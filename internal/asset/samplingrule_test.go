package asset

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	samplingTestOrigin = "dash0-cli_checkout-errors"
	samplingTestID     = "4b7e2d91-3c5a-4f08-9e61-2a8d7c4f1b35"
)

func samplingDoc(labels, spec string) []byte {
	doc := "kind: Dash0Sampling\nmetadata:\n  name: checkout-errors\n"
	if labels != "" {
		doc += "  labels:\n" + labels
	}
	return []byte(doc + "spec:\n" + spec)
}

const samplingSpec = `  display:
    name: Keep checkout errors
  enabled: true
  conditions:
    kind: and
    spec:
      conditions:
        - kind: ottl
          spec:
            ottl: resource.attributes["service.name"] == "checkout"
        - kind: error
          spec: {}
`

func parseSampling(t *testing.T, labels, spec string) *dash0api.SamplingDefinition {
	t.Helper()
	rule, err := ParseSamplingRule(samplingDoc(labels, spec))
	require.NoError(t, err)
	return rule
}

func TestGetSamplingRuleOrigin(t *testing.T) {
	assert.Equal(t, samplingTestOrigin, GetSamplingRuleOrigin(parseSampling(t, "    dash0.com/origin: "+samplingTestOrigin+"\n", samplingSpec)))
	assert.Empty(t, GetSamplingRuleOrigin(parseSampling(t, "    dash0.com/id: "+samplingTestID+"\n", samplingSpec)))
	assert.Empty(t, GetSamplingRuleOrigin(&dash0api.SamplingDefinition{}))
	assert.Empty(t, GetSamplingRuleOrigin(nil))
}

// StripSamplingRuleServerFields clears the origin label, so the import helper
// must read it first or every origin document would POST a duplicate.
func TestGetSamplingRuleOrigin_MustBeReadBeforeStrip(t *testing.T) {
	rule := parseSampling(t, "    dash0.com/origin: "+samplingTestOrigin+"\n", samplingSpec)

	dash0api.StripSamplingRuleServerFields(rule)

	assert.Empty(t, GetSamplingRuleOrigin(rule), "Strip clears the origin label; the import helper must read it first")
}

func TestSamplingRuleKey(t *testing.T) {
	both := "    dash0.com/origin: " + samplingTestOrigin + "\n    dash0.com/id: " + samplingTestID + "\n"
	assert.Equal(t, samplingTestOrigin, SamplingRuleKey(parseSampling(t, both, samplingSpec)), "origin wins over id")
	assert.Equal(t, samplingTestID, SamplingRuleKey(parseSampling(t, "    dash0.com/id: "+samplingTestID+"\n", samplingSpec)))
	assert.Empty(t, SamplingRuleKey(parseSampling(t, "", samplingSpec)))
	assert.Empty(t, SamplingRuleKey(nil))
}

func TestParseSamplingRule_RequiresEnabled(t *testing.T) {
	spec := strings.Replace(samplingSpec, "  enabled: true\n", "", 1)

	_, err := ParseSamplingRule(samplingDoc("", spec))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires spec.enabled")
	assert.Contains(t, err.Error(), "\nHint: set spec.enabled to true or false")
}

func TestParseSamplingRule_EnabledFalseIsAccepted(t *testing.T) {
	rule := parseSampling(t, "", strings.Replace(samplingSpec, "enabled: true", "enabled: false", 1))
	assert.False(t, rule.Spec.Enabled)
}

func TestParseSamplingRule_RequiresConditions(t *testing.T) {
	for name, spec := range map[string]string{
		"missing": "  enabled: true\n",
		"null":    "  enabled: true\n  conditions: null\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseSamplingRule(samplingDoc("", spec))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "requires spec.conditions")
		})
	}
}

func TestParseSamplingRule_RejectsMalformedYAML(t *testing.T) {
	_, err := ParseSamplingRule([]byte("kind: Dash0Sampling\nspec: ["))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse sampling rule definition")
}

// andOf builds an and condition over n children of the given kind, in YAML.
func andOf(n int, child string) string {
	var b strings.Builder
	b.WriteString("  enabled: true\n  conditions:\n    kind: and\n    spec:\n      conditions:\n")
	for i := 0; i < n; i++ {
		b.WriteString(child)
	}
	return b.String()
}

const (
	ottlChild          = "        - kind: ottl\n          spec:\n            ottl: span.name == \"checkout\"\n"
	errorChild         = "        - kind: error\n          spec: {}\n"
	probabilisticChild = "        - kind: probabilistic\n          spec:\n            rate: 0.1\n"
)

func TestParseSamplingRule_MatchingConditionLimit(t *testing.T) {
	t.Run("32 conditions are accepted", func(t *testing.T) {
		_, err := ParseSamplingRule(samplingDoc("", andOf(16, ottlChild+errorChild)))
		require.NoError(t, err)
	})
	t.Run("33 conditions are rejected", func(t *testing.T) {
		_, err := ParseSamplingRule(samplingDoc("", andOf(33, ottlChild)))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "holds 33 ottl and error conditions, more than the limit of 32")
		assert.Contains(t, err.Error(), "\nHint: Dash0 does not load a rule above the limit")
	})
	t.Run("probabilistic conditions do not count", func(t *testing.T) {
		_, err := ParseSamplingRule(samplingDoc("", andOf(32, ottlChild)+strings.Repeat(probabilisticChild, 10)))
		require.NoError(t, err)
	})
	t.Run("nested and conditions count", func(t *testing.T) {
		nested := "        - kind: and\n          spec:\n            conditions:\n" +
			strings.Repeat("              - kind: error\n                spec: {}\n", 2)
		_, err := ParseSamplingRule(samplingDoc("", andOf(31, ottlChild)+nested))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "holds 33 ottl and error conditions")
	})
}

func TestDescribeSamplingRuleConditions(t *testing.T) {
	tests := map[string]struct {
		spec string
		want string
	}{
		"and of ottl and error": {samplingSpec, "and(ottl, error)"},
		"probabilistic":         {"  enabled: true\n  conditions:\n    kind: probabilistic\n    spec:\n      rate: 0.25\n", "probabilistic 0.25"},
		"single error":          {"  enabled: true\n  conditions:\n    kind: error\n    spec: {}\n", "error"},
		"nested and": {
			"  enabled: true\n  conditions:\n    kind: and\n    spec:\n      conditions:\n" + errorChild +
				"        - kind: and\n          spec:\n            conditions:\n              - kind: probabilistic\n                spec:\n                  rate: 0.5\n",
			"and(error, and(probabilistic 0.5))",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tt.want, DescribeSamplingRuleConditions(parseSampling(t, "", tt.spec)))
		})
	}
	assert.Empty(t, DescribeSamplingRuleConditions(nil))
	assert.Empty(t, DescribeSamplingRuleConditions(&dash0api.SamplingDefinition{}))
}

func TestSamplingRuleUsesOrigin(t *testing.T) {
	usesOrigin, err := SamplingRuleUsesOrigin(samplingDoc("    dash0.com/origin: "+samplingTestOrigin+"\n", samplingSpec))
	require.NoError(t, err)
	assert.True(t, usesOrigin)

	usesOrigin, err = SamplingRuleUsesOrigin(samplingDoc("    dash0.com/id: "+samplingTestID+"\n", samplingSpec))
	require.NoError(t, err)
	assert.False(t, usesOrigin)

	_, err = SamplingRuleUsesOrigin([]byte("metadata: ["))
	require.Error(t, err)
}

func TestKindDisplayName_SamplingRule(t *testing.T) {
	assert.Equal(t, "Sampling rule", KindDisplayName("Dash0Sampling"))
}

func TestIsValidKind_SamplingRule(t *testing.T) {
	assert.True(t, IsValidKind("Dash0Sampling"))
	assert.False(t, IsValidKind("Dash0SamplingRule"))
}

// Sampling rule labels hold a nested custom map, so the identifier probe must
// not assume every label value is a string.
func TestExtractIdentifier_SamplingRule(t *testing.T) {
	custom := "    custom:\n      team: checkout\n"
	tests := []struct {
		name   string
		labels string
		want   string
	}{
		{"origin wins over id", "    dash0.com/origin: " + samplingTestOrigin + "\n    dash0.com/id: " + samplingTestID + "\n" + custom, samplingTestOrigin},
		{"id when origin is absent", "    dash0.com/id: " + samplingTestID + "\n" + custom, samplingTestID},
		{"neither label", custom, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identifier, err := ExtractIdentifier(samplingDoc(tt.labels, samplingSpec))
			require.NoError(t, err)
			assert.Equal(t, tt.want, identifier)
		})
	}
}

func TestParseDocumentHeader_SamplingRule(t *testing.T) {
	kind, name, id, err := ParseDocumentHeader(samplingDoc("    dash0.com/origin: "+samplingTestOrigin+"\n    custom:\n      team: checkout\n", samplingSpec))

	require.NoError(t, err)
	assert.Equal(t, "Dash0Sampling", kind)
	assert.Equal(t, "Keep checkout errors", name, "the name comes from spec.display.name")
	assert.Equal(t, samplingTestOrigin, id)
}

// samplingClient serves sampling rule reads from fixed values and records
// every call, so a test can assert what the plan step reads and writes.
type samplingClient struct {
	dash0api.Client
	stored  *dash0api.SamplingDefinition
	getErr  error
	list    []*dash0api.SamplingDefinition
	listErr error
	calls   []string
}

func (c *samplingClient) GetSamplingRule(_ context.Context, originOrID string, _ *string) (*dash0api.SamplingDefinition, error) {
	c.calls = append(c.calls, "GET "+originOrID)
	return c.stored, c.getErr
}

func (c *samplingClient) ListSamplingRules(context.Context, *string) ([]*dash0api.SamplingDefinition, error) {
	c.calls = append(c.calls, "LIST")
	return c.list, c.listErr
}

func apiError(status int, message string) error {
	return &dash0api.APIError{StatusCode: status, Status: http.StatusText(status), Message: message}
}

var (
	errSamplingNotFound     = apiError(http.StatusNotFound, "Not Found: The requested sampling rule does not exist or is inaccessible to you.")
	errSamplingWrongDataset = apiError(http.StatusBadRequest, "Bad Request: A sampling rule with this origin exists, but it is associated with a different dataset.")
)

func storedSamplingRule(id string, enabled bool) *dash0api.SamplingDefinition {
	return &dash0api.SamplingDefinition{
		Metadata: dash0api.SamplingMetadata{Name: "stored", Labels: &dash0api.SamplingLabels{Dash0Comid: &id}},
		Spec:     dash0api.SamplingSpec{Enabled: enabled},
	}
}

func TestPlanSamplingRule_ByOrigin(t *testing.T) {
	labels := "    dash0.com/origin: " + samplingTestOrigin + "\n    dash0.com/id: " + samplingTestID + "\n    dash0.com/dataset: elsewhere\n"

	t.Run("unknown origin is still the key", func(t *testing.T) {
		client := &samplingClient{getErr: errSamplingNotFound}
		rule := parseSampling(t, labels, samplingSpec)

		plan, err := PlanSamplingRule(context.Background(), client, rule, nil)

		require.NoError(t, err)
		assert.Equal(t, samplingTestOrigin, plan.Key, "PUT to an unknown origin creates the rule")
		assert.Equal(t, ActionCreated, plan.Action)
		assert.Empty(t, dash0api.GetSamplingRuleID(rule), "the path selects the rule, so the body carries no id")
		assert.Empty(t, dash0api.GetSamplingRuleDataset(rule), "the dataset label would win over --dataset")
		assert.Empty(t, GetSamplingRuleOrigin(rule))
	})
	t.Run("existing origin is an update", func(t *testing.T) {
		stored := storedSamplingRule(samplingTestID, true)
		client := &samplingClient{stored: stored}

		plan, err := PlanSamplingRule(context.Background(), client, parseSampling(t, labels, samplingSpec), nil)

		require.NoError(t, err)
		assert.Equal(t, ActionUpdated, plan.Action)
		assert.Same(t, stored, plan.Before)
		assert.Empty(t, plan.Warnings, "sampling is already on through this rule")
	})
	t.Run("rule in another dataset is explained", func(t *testing.T) {
		client := &samplingClient{getErr: errSamplingWrongDataset}

		_, err := PlanSamplingRule(context.Background(), client, parseSampling(t, labels, samplingSpec), dash0api.DatasetPtr("staging"))

		require.Error(t, err)
		assert.True(t, IsSamplingRuleWrongDataset(err))
		assert.Contains(t, err.Error(), `sampling rule "`+samplingTestOrigin+`" already exists in another dataset`)
		assert.Contains(t, err.Error(), "\nHint: origins and IDs are unique per organization")
	})
	t.Run("other read errors are returned with the key", func(t *testing.T) {
		client := &samplingClient{getErr: apiError(http.StatusForbidden, "Forbidden")}

		plan, err := PlanSamplingRule(context.Background(), client, parseSampling(t, labels, samplingSpec), nil)

		require.Error(t, err)
		assert.Equal(t, samplingTestOrigin, plan.Key)
	})
}

func TestPlanSamplingRule_ByIDOnly(t *testing.T) {
	labels := "    dash0.com/id: " + samplingTestID + "\n"

	t.Run("known id keeps the id", func(t *testing.T) {
		client := &samplingClient{stored: storedSamplingRule(samplingTestID, true)}
		rule := parseSampling(t, labels, samplingSpec)

		plan, err := PlanSamplingRule(context.Background(), client, rule, nil)

		require.NoError(t, err)
		assert.Equal(t, samplingTestID, plan.Key)
		assert.Equal(t, ActionUpdated, plan.Action)
		assert.Equal(t, samplingTestID, dash0api.GetSamplingRuleID(rule), "a PUT to the id may carry the same id")
	})
	t.Run("unknown id falls back to POST without the id", func(t *testing.T) {
		client := &samplingClient{getErr: errSamplingNotFound, list: []*dash0api.SamplingDefinition{storedSamplingRule("other", true)}}
		rule := parseSampling(t, labels, samplingSpec)

		plan, err := PlanSamplingRule(context.Background(), client, rule, nil)

		require.NoError(t, err)
		assert.Empty(t, plan.Key, "an empty key means POST")
		assert.Equal(t, ActionCreated, plan.Action)
		assert.Empty(t, dash0api.GetSamplingRuleID(rule), "the server rejects an id in a POST body")
	})
	t.Run("rule in another dataset is explained", func(t *testing.T) {
		client := &samplingClient{getErr: errSamplingWrongDataset}

		_, err := PlanSamplingRule(context.Background(), client, parseSampling(t, labels, samplingSpec), nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), `sampling rule "`+samplingTestID+`" already exists in another dataset`)
	})
	t.Run("other read errors are surfaced", func(t *testing.T) {
		client := &samplingClient{getErr: apiError(http.StatusUnauthorized, "Unauthorized")}

		_, err := PlanSamplingRule(context.Background(), client, parseSampling(t, labels, samplingSpec), nil)

		require.Error(t, err)
		assert.True(t, dash0api.IsUnauthorized(err))
	})
}

func TestPlanSamplingRule_WithoutIdentifierReadsNothing(t *testing.T) {
	client := &samplingClient{list: []*dash0api.SamplingDefinition{storedSamplingRule("other", true)}}

	plan, err := PlanSamplingRule(context.Background(), client, parseSampling(t, "", samplingSpec), nil)

	require.NoError(t, err)
	assert.Empty(t, plan.Key)
	assert.Equal(t, []string{"LIST"}, client.calls, "only the turn-on check reads, and no rule is fetched")
}

func TestSamplingTurnOnWarning(t *testing.T) {
	enabled := func(t *testing.T) *dash0api.SamplingDefinition { return parseSampling(t, "", samplingSpec) }
	disabled := func(t *testing.T) *dash0api.SamplingDefinition {
		return parseSampling(t, "", strings.Replace(samplingSpec, "enabled: true", "enabled: false", 1))
	}

	t.Run("first enabled rule in the dataset warns", func(t *testing.T) {
		client := &samplingClient{list: []*dash0api.SamplingDefinition{storedSamplingRule("other", false)}}

		warning := SamplingTurnOnWarning(context.Background(), client, enabled(t), nil, dash0api.DatasetPtr("staging"))

		assert.Equal(t, `sampling rule "Keep checkout errors" is the first enabled sampling rule in dataset "staging", so it turns sampling on: `+
			"Dash0 then drops every trace that no enabled sampling rule keeps, and dropped traces cannot be recovered", warning)
	})
	t.Run("no dataset means default", func(t *testing.T) {
		warning := SamplingTurnOnWarning(context.Background(), &samplingClient{}, enabled(t), nil, nil)
		assert.Contains(t, warning, `in dataset "default"`)
	})
	t.Run("another enabled rule means sampling is on already", func(t *testing.T) {
		client := &samplingClient{list: []*dash0api.SamplingDefinition{storedSamplingRule("other", true)}}
		assert.Empty(t, SamplingTurnOnWarning(context.Background(), client, enabled(t), nil, nil))
	})
	t.Run("the rule being written never counts as another enabled rule", func(t *testing.T) {
		existing := storedSamplingRule(samplingTestID, false)
		// Even if the list already shows the rule as enabled, for example
		// after a concurrent write, it is the rule being written, not another
		// rule that keeps sampling on.
		listed := storedSamplingRule(samplingTestID, true)
		client := &samplingClient{list: []*dash0api.SamplingDefinition{listed}}

		assert.NotEmpty(t, SamplingTurnOnWarning(context.Background(), client, enabled(t), existing, nil))
	})
	t.Run("an already enabled rule does not warn", func(t *testing.T) {
		client := &samplingClient{}
		assert.Empty(t, SamplingTurnOnWarning(context.Background(), client, enabled(t), storedSamplingRule(samplingTestID, true), nil))
		assert.Empty(t, client.calls, "no list is needed")
	})
	t.Run("a disabled rule does not warn", func(t *testing.T) {
		client := &samplingClient{}
		assert.Empty(t, SamplingTurnOnWarning(context.Background(), client, disabled(t), nil, nil))
		assert.Empty(t, client.calls)
	})
	t.Run("a failed list warns softly instead of failing", func(t *testing.T) {
		client := &samplingClient{listErr: errors.New("connection reset")}

		warning := SamplingTurnOnWarning(context.Background(), client, enabled(t), nil, nil)

		assert.Contains(t, warning, "could not be listed (connection reset)")
		assert.Contains(t, warning, "if it is the first enabled rule there, it turns sampling on")
	})
}

func TestSamplingRuleListDataset(t *testing.T) {
	empty := ""
	for _, dataset := range []*string{nil, &empty} {
		got := SamplingRuleListDataset(dataset)
		require.NotNil(t, got, "the list endpoint returns every dataset without the parameter")
		assert.Equal(t, "default", *got)
	}
	assert.Equal(t, "staging", *SamplingRuleListDataset(dash0api.DatasetPtr("staging")))
}

func TestIsSamplingRuleWrongDataset(t *testing.T) {
	assert.True(t, IsSamplingRuleWrongDataset(errSamplingWrongDataset))
	assert.True(t, IsSamplingRuleWrongDataset(fmt.Errorf("wrapped: %w", errSamplingWrongDataset)))
	assert.False(t, IsSamplingRuleWrongDataset(errSamplingNotFound))
	assert.False(t, IsSamplingRuleWrongDataset(apiError(http.StatusBadRequest, "Bad Request: ID should not be provided for creation.")))
	assert.False(t, IsSamplingRuleWrongDataset(nil))
}
