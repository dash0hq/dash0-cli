package asset

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validEvalYAML = `kind: Dash0GenerativeAiEvaluation
metadata:
  name: answer-relevance
spec:
  display:
    name: Answer relevance
  enabled: true
  judge:
    model: standard
    output:
      kind: boolean
    systemPrompt: You judge relevance.
    userPrompt: "{{ gen_ai.input.messages }}"
  match:
    filters:
      - key: service.name
        operator: is
        value: checkout
      - key: gen_ai.operation.name
        operator: is_set
  sample:
    rate: 1
`

func TestParseGenerativeAiEvaluation(t *testing.T) {
	requiredFilter := "      - key: gen_ai.operation.name\n        operator: is_set\n"
	tests := []struct {
		name    string
		old     string
		new     string
		wantErr string
	}{
		{name: "valid, required filter after another filter"},
		{name: "missing required filter", old: requiredFilter, new: "", wantErr: "requires the filter {key: gen_ai.operation.name, operator: is_set}"},
		{name: "required key with another operator", old: "operator: is_set", new: "operator: is_not_set", wantErr: "requires the filter"},
		{name: "is_set filter with values", old: "operator: is_set\n", new: "operator: is_set\n        values: [chat]\n", wantErr: "requires the filter"},
		{name: "is_set filter with an empty values list is allowed", old: "operator: is_set\n", new: "operator: is_set\n        values: []\n"},
		{name: "missing metadata.name", old: "  name: answer-relevance\n", new: "", wantErr: "requires metadata.name"},
		{name: "missing display name is allowed", old: "    name: Answer relevance\n", new: ""},
		{name: "missing system prompt is allowed", old: "    systemPrompt: You judge relevance.\n", new: ""},
		{name: "missing output kind", old: "    output:\n      kind: boolean\n", new: "", wantErr: "requires spec.judge.output.kind"},
		{name: "blank user prompt", old: `"{{ gen_ai.input.messages }}"`, new: `" "`, wantErr: "requires spec.judge.userPrompt"},
		{name: "rate above 1", old: "rate: 1\n", new: "rate: 1.5\n", wantErr: "spec.sample.rate must be between 0 and 1"},
		{name: "rate 0 is allowed", old: "rate: 1\n", new: "rate: 0\n"},
		{name: "missing sample rate", old: "    rate: 1\n", new: "", wantErr: "requires spec.sample.rate"},
		{name: "missing enabled", old: "  enabled: true\n", new: "", wantErr: "requires spec.enabled"},
		{name: "enabled false is allowed", old: "enabled: true", new: "enabled: false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := validEvalYAML
			if tt.old != "" {
				require.Contains(t, doc, tt.old)
				doc = strings.Replace(doc, tt.old, tt.new, 1)
			}
			_, err := ParseGenerativeAiEvaluation([]byte(doc))
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}

	// A document with only a kind fails on the first required field.
	_, err := ParseGenerativeAiEvaluation([]byte("kind: Dash0GenerativeAiEvaluation\n"))
	assert.EqualError(t, err, "eval definition requires metadata.name")
}
