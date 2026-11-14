package diff

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dash0hq/dash0-cli/internal/asset"
)

// keyRecordingClient answers every read with 404 and records the key of each
// read, so a test sees every key a plan tries, fallbacks included.
type keyRecordingClient struct {
	dash0api.Client
	reads []string
}

func (c *keyRecordingClient) notFound(method, key string) error {
	c.reads = append(c.reads, method+"("+key+")")
	return &dash0api.APIError{StatusCode: http.StatusNotFound, Status: "404 Not Found"}
}

func (c *keyRecordingClient) GetDashboard(_ context.Context, key string, _ *string) (*dash0api.DashboardDefinition, error) {
	return nil, c.notFound("GetDashboard", key)
}

func (c *keyRecordingClient) GetCheckRule(_ context.Context, key string, _ *string) (*dash0api.PrometheusAlertRule, error) {
	return nil, c.notFound("GetCheckRule", key)
}

func (c *keyRecordingClient) GetSyntheticCheck(_ context.Context, key string, _ *string) (*dash0api.SyntheticCheckDefinition, error) {
	return nil, c.notFound("GetSyntheticCheck", key)
}

func (c *keyRecordingClient) GetSLO(_ context.Context, key string, _ *string) (*dash0api.SloDefinition, error) {
	return nil, c.notFound("GetSLO", key)
}

func (c *keyRecordingClient) GetDataset(_ context.Context, key string) (*dash0api.DatasetDefinition, error) {
	return nil, c.notFound("GetDataset", key)
}

func (c *keyRecordingClient) GetAwsMetricScrapeConfig(_ context.Context, key string) (*dash0api.AwsMetricScrapeConfigDefinition, error) {
	return nil, c.notFound("GetAwsMetricScrapeConfig", key)
}

func (c *keyRecordingClient) GetView(_ context.Context, key string, _ *string) (*dash0api.ViewDefinition, error) {
	return nil, c.notFound("GetView", key)
}

func (c *keyRecordingClient) GetTimeSeriesAggregation(_ context.Context, key string, _ *string) (*dash0api.TimeSeriesAggregationDefinition, error) {
	return nil, c.notFound("GetTimeSeriesAggregation", key)
}

func (c *keyRecordingClient) GetTeam(_ context.Context, key string) (*dash0api.TeamDefinitionV1Alpha1, error) {
	return nil, c.notFound("GetTeam", key)
}

func (c *keyRecordingClient) GetRecordingRule(_ context.Context, key string, _ *string) (*dash0api.RecordingRule, error) {
	return nil, c.notFound("GetRecordingRule", key)
}

func (c *keyRecordingClient) GetCustomSkill(_ context.Context, key string) (*dash0api.SkillDefinition, error) {
	return nil, c.notFound("GetCustomSkill", key)
}

func (c *keyRecordingClient) GetNotificationChannel(_ context.Context, key string) (*dash0api.NotificationChannelDefinition, error) {
	return nil, c.notFound("GetNotificationChannel", key)
}

func (c *keyRecordingClient) GetSignalToMetrics(_ context.Context, key string, _ *string) (*dash0api.SignalToMetricsDefinition, error) {
	return nil, c.notFound("GetSignalToMetrics", key)
}

func (c *keyRecordingClient) GetGenerativeAiEvaluation(_ context.Context, key string, _ *string) (*dash0api.GenerativeAiEvaluationDefinition, error) {
	return nil, c.notFound("GetGenerativeAiEvaluation", key)
}

func (c *keyRecordingClient) GetSpamFilter(_ context.Context, key string, _ *string) (dash0api.SpamFilterObject, error) {
	return nil, c.notFound("GetSpamFilter", key)
}

// identifierVariant is which identifiers a characterization document carries.
type identifierVariant string

const (
	withNeither identifierVariant = "neither"
	withOrigin  identifierVariant = "origin"
	withID      identifierVariant = "id"
	withBoth    identifierVariant = "both"
)

var identifierVariants = []identifierVariant{withNeither, withOrigin, withID, withBoth}

// characterizationDocument returns a minimal document of the kind that carries
// the origin "o" and the id "i" as the variant says. The id goes in the field
// the kind's own id lives in.
func characterizationDocument(kind string, variant identifierVariant) string {
	hasOrigin := variant == withOrigin || variant == withBoth
	hasID := variant == withID || variant == withBoth

	var labels []string
	if hasOrigin {
		labels = append(labels, "    dash0.com/origin: o")
	}
	idInLabel := kind != "dashboard" && kind != "ephemeraldashboard" && kind != "checkrule"
	if hasID && idInLabel {
		labels = append(labels, "    dash0.com/id: i")
	}
	labelBlock := ""
	if len(labels) > 0 {
		labelBlock = "  labels:\n" + strings.Join(labels, "\n") + "\n"
	}

	switch kind {
	case "dashboard", "ephemeraldashboard":
		extensions := ""
		if hasID {
			extensions = "  dash0Extensions:\n    id: i\n"
		}
		if kind == "ephemeraldashboard" {
			return "kind: EphemeralDashboard\nmetadata:\n  name: n\n" + labelBlock + extensions + "spec:\n  ttl: 1d\n  display:\n    name: n\n"
		}
		return "apiVersion: dash0.com/v1alpha1\nkind: Dashboard\nmetadata:\n  name: n\n" + labelBlock + extensions + "spec:\n  display:\n    name: n\n"
	case "persesdashboard":
		return "apiVersion: perses.dev/v1alpha1\nkind: PersesDashboard\nmetadata:\n  name: n\n" + labelBlock + "spec:\n  display:\n    name: n\n  duration: 5m\n  panels: {}\n"
	case "checkrule":
		id := ""
		if hasID {
			id = "id: i\n"
		}
		return "apiVersion: dash0.com/v1alpha1\nkind: CheckRule\n" + id + "name: n\nexpression: up == 0\nmetadata:\n" + labelBlock
	case "prometheusrule":
		return "apiVersion: monitoring.coreos.com/v1\nkind: PrometheusRule\nmetadata:\n  name: n\n" + labelBlock +
			"spec:\n  groups:\n    - name: g\n      rules:\n        - alert: a\n          expr: up == 0\n        - record: r\n          expr: up\n"
	case "syntheticcheck":
		return "apiVersion: dash0.com/v1alpha1\nkind: SyntheticCheck\nmetadata:\n  name: n\n" + labelBlock + "spec: {}\n"
	case "slo":
		return "apiVersion: openslo.com/v1\nkind: SLO\nmetadata:\n  name: n\n" + labelBlock + "spec: {}\n"
	case "view":
		return "apiVersion: dash0.com/v1alpha1\nkind: View\nmetadata:\n  name: n\n" + labelBlock + "spec: {}\n"
	case "spamfilter":
		return "apiVersion: v1alpha1\nkind: Dash0SpamFilter\nmetadata:\n  name: n\n" + labelBlock + "spec:\n  contexts: [log]\n  filter: []\n"
	case "notificationchannel":
		return "apiVersion: dash0.com/v1alpha1\nkind: Dash0NotificationChannel\nmetadata:\n  name: n\n" + labelBlock + "spec:\n  type: slack\n"
	case "team":
		return "apiVersion: dash0.com/v1alpha1\nkind: Dash0Team\nmetadata:\n  name: n\n" + labelBlock + "spec:\n  display:\n    name: n\n"
	case "timeseriesaggregation":
		return "apiVersion: dash0.com/v1alpha1\nkind: Dash0TimeSeriesAggregation\nmetadata:\n  name: n\n" + labelBlock + "spec: {}\n"
	case "dataset":
		return "apiVersion: dash0.com/v1alpha1\nkind: Dash0Dataset\nmetadata:\n  name: n\n" + labelBlock + "spec: {}\n"
	case "skill":
		return "kind: Dash0Skill\nmetadata:\n" + labelBlock + "spec:\n  files:\n    - relativePath: SKILL.md\n      content: |\n        ---\n        name: n\n        description: d\n        ---\n"
	case "generativeaievaluation":
		return "kind: Dash0GenerativeAiEvaluation\nmetadata:\n  name: n\n" + labelBlock +
			"spec:\n  display:\n    name: n\n  enabled: true\n  judge:\n    model: standard\n    output:\n      kind: boolean\n    systemPrompt: s\n    userPrompt: u\n  match:\n    filters:\n      - key: gen_ai.operation.name\n        operator: is_set\n  sample:\n    rate: 0.25\n"
	case "signaltometrics":
		return "kind: Dash0SignalToMetrics\nmetadata:\n  name: n\n" + labelBlock +
			"spec:\n  display:\n    name: n\n  enabled: true\n  match:\n    signal: logs\n    filters: []\n  output:\n    name: m\n    interval: 1m\n"
	case "awsmetricscrapeconfig":
		return "apiVersion: dash0.com/v1alpha1\nkind: Dash0AwsMetricScrapeConfig\nmetadata:\n" + labelBlock + "spec:\n  namespaces: []\n"
	}
	panic("no characterization document for kind " + kind)
}

// upsertKeyCharacterization is what every reader of a document's upsert key
// returns for one document today.
type upsertKeyCharacterization struct {
	extractIdentifier string
	headerID          string
	planReads         string
}

func characterize(t *testing.T, kind string, variant identifierVariant) upsertKeyCharacterization {
	t.Helper()
	raw := []byte(characterizationDocument(kind, variant))

	extracted, err := asset.ExtractIdentifier(raw)
	require.NoError(t, err)

	docKind, _, headerID, err := asset.ParseDocumentHeader(raw)
	require.NoError(t, err)

	apiClient := &keyRecordingClient{}
	_, planErr := planDocument(context.Background(), apiClient, asset.Document{Kind: docKind, ID: headerID, Raw: raw}, nil, false)
	reads := apiClient.reads
	if planErr != nil {
		reads = append(reads, "error: "+planErr.Error())
	}

	return upsertKeyCharacterization{
		extractIdentifier: extracted,
		headerID:          headerID,
		planReads:         strings.Join(reads, " "),
	}
}

// TestUpsertKey_Characterization pins the upsert key every kind uses today, so
// moving the key choice into one rule cannot change which asset a document
// replaces. Change this table only together with a user-facing change.
func TestUpsertKey_Characterization(t *testing.T) {
	want := map[string]map[identifierVariant]upsertKeyCharacterization{
		"dashboard": {
			withNeither: {"", "", ""},
			withOrigin:  {"", "", ""},
			withID:      {"i", "i", "GetDashboard(i)"},
			withBoth:    {"i", "i", "GetDashboard(i)"},
		},
		"ephemeraldashboard": {
			withNeither: {"", "", ""},
			withOrigin:  {"", "", ""},
			withID:      {"i", "i", "GetDashboard(i)"},
			withBoth:    {"i", "i", "GetDashboard(i)"},
		},
		"checkrule": {
			withNeither: {"", "", ""},
			withOrigin:  {"", "", ""},
			withID:      {"i", "i", "GetCheckRule(i)"},
			withBoth:    {"i", "i", "GetCheckRule(i)"},
		},
		"syntheticcheck": {
			withNeither: {"", "", ""},
			withOrigin:  {"", "", ""},
			withID:      {"i", "i", "GetSyntheticCheck(i)"},
			withBoth:    {"i", "i", "GetSyntheticCheck(i)"},
		},
		"slo": {
			withNeither: {"", "", ""},
			withOrigin:  {"o", "o", "GetSLO(o)"},
			withID:      {"i", "i", "GetSLO(i)"},
			withBoth:    {"o", "o", "GetSLO(o)"},
		},
		"view": {
			withNeither: {"", "", ""},
			withOrigin:  {"", "", ""},
			withID:      {"i", "i", "GetView(i)"},
			withBoth:    {"i", "i", "GetView(i)"},
		},
		"prometheusrule": {
			withNeither: {"", "", ""},
			withOrigin:  {"", "", ""},
			withID:      {"i", "i", "GetCheckRule(i) GetRecordingRule(i)"},
			withBoth:    {"i", "i", "GetCheckRule(i) GetRecordingRule(i)"},
		},
		"persesdashboard": {
			withNeither: {"", "", ""},
			withOrigin:  {"", "", ""},
			withID:      {"i", "i", "GetDashboard(i)"},
			withBoth:    {"i", "i", "GetDashboard(i)"},
		},
		"spamfilter": {
			withNeither: {"", "", ""},
			withOrigin:  {"o", "", "GetSpamFilter(o)"},
			withID:      {"i", "i", "GetSpamFilter(i)"},
			withBoth:    {"o", "i", "GetSpamFilter(o)"},
		},
		"notificationchannel": {
			withNeither: {"", "", ""},
			withOrigin:  {"o", "o", "GetNotificationChannel(o)"},
			withID:      {"", "i", ""},
			withBoth:    {"o", "i", "GetNotificationChannel(o)"},
		},
		"team": {
			withNeither: {"", "", ""},
			withOrigin:  {"o", "o", "GetTeam(o)"},
			withID:      {"i", "i", "GetTeam(i)"},
			withBoth:    {"o", "o", "GetTeam(o) GetTeam(i)"},
		},
		"timeseriesaggregation": {
			withNeither: {"", "", "error: Dash0TimeSeriesAggregation requires metadata.labels[\"dash0.com/origin\"]: the Dash0 API rejects a time series aggregation without an origin, and the origin is the key `apply` upserts by"},
			withOrigin:  {"o", "o", "GetTimeSeriesAggregation(o)"},
			withID:      {"", "", "error: Dash0TimeSeriesAggregation requires metadata.labels[\"dash0.com/origin\"]: the Dash0 API rejects a time series aggregation without an origin, and the origin is the key `apply` upserts by"},
			withBoth:    {"o", "o", "GetTimeSeriesAggregation(o)"},
		},
		"dataset": {
			withNeither: {"", "", "error: dataset definition requires a slug in spec.slug or metadata.labels[\"dash0.com/id\"]"},
			withOrigin:  {"", "", "error: dataset definition requires a slug in spec.slug or metadata.labels[\"dash0.com/id\"]"},
			withID:      {"i", "i", "GetDataset(i)"},
			withBoth:    {"i", "i", "GetDataset(i)"},
		},
		"skill": {
			withNeither: {"", "", ""},
			withOrigin:  {"o", "o", "GetCustomSkill(o)"},
			withID:      {"i", "i", "GetCustomSkill(i) error: Agent0 skill with id \"i\" not found\nHint: dash0.com/id only addresses an existing skill; set metadata.labels[\"dash0.com/origin\"] to create the skill and manage it as code"},
			withBoth:    {"o", "o", "GetCustomSkill(o)"},
		},
		"awsmetricscrapeconfig": {
			withNeither: {"", "", "GetAwsMetricScrapeConfig() error: AWS integration not found (status: 404)"},
			withOrigin:  {"", "", "GetAwsMetricScrapeConfig() error: AWS integration not found (status: 404)"},
			withID:      {"i", "i", "GetAwsMetricScrapeConfig(i) error: AWS integration \"i\" not found (status: 404)"},
			withBoth:    {"i", "i", "GetAwsMetricScrapeConfig(i) error: AWS integration \"i\" not found (status: 404)"},
		},
		"generativeaievaluation": {
			withNeither: {"", "", ""},
			withOrigin:  {"o", "o", "GetGenerativeAiEvaluation(o)"},
			withID:      {"i", "i", "GetGenerativeAiEvaluation(i)"},
			withBoth:    {"o", "o", "GetGenerativeAiEvaluation(o)"},
		},
		"signaltometrics": {
			withNeither: {"", "", ""},
			withOrigin:  {"o", "o", "GetSignalToMetrics(o)"},
			withID:      {"i", "i", "GetSignalToMetrics(i)"},
			withBoth:    {"o", "o", "GetSignalToMetrics(o)"},
		},
	}

	for _, kind := range asset.ValidKinds {
		for _, variant := range identifierVariants {
			t.Run(fmt.Sprintf("%s/%s", kind, variant), func(t *testing.T) {
				got := characterize(t, kind, variant)
				expected, ok := want[kind][variant]
				require.True(t, ok, "no characterization for this kind; add one")
				assert.Equal(t, expected, got)
			})
		}
	}
}
