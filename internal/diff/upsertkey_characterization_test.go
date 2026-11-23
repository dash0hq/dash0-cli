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

func (c *keyRecordingClient) GetSamplingRule(_ context.Context, key string, _ *string) (*dash0api.SamplingDefinition, error) {
	return nil, c.notFound("GetSamplingRule", key)
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
	case "sampling":
		// Disabled, so planning reads only the rule and never lists the
		// dataset for the turn-on warning.
		return "kind: Dash0Sampling\nmetadata:\n  name: n\n" + labelBlock +
			"spec:\n  display:\n    name: n\n  enabled: false\n  conditions:\n    kind: error\n    spec: {}\n"
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
			withNeither: {"", "", "GetDashboard(dash0-cli_8c3789eb30701c9e4d02c5b21660f40d)"},
			withOrigin:  {"o", "o", "GetDashboard(o)"},
			withID:      {"i", "i", "GetDashboard(i)"},
			withBoth:    {"o", "o", `error: metadata.labels["dash0.com/origin"] ("o") and metadata.dash0Extensions.id ("i") must be equal; set only metadata.labels["dash0.com/origin"], and move "i" to metadata.labels["dash0.com/id"] if it is the id of an existing dashboard`},
		},
		"ephemeraldashboard": {
			withNeither: {"", "", "GetDashboard(dash0-cli_9abe74d15ec59f186f017630ff7c797d)"},
			withOrigin:  {"o", "o", "GetDashboard(o)"},
			withID:      {"i", "i", "GetDashboard(i)"},
			withBoth:    {"o", "o", `error: metadata.labels["dash0.com/origin"] ("o") and metadata.dash0Extensions.id ("i") must be equal; set only metadata.labels["dash0.com/origin"], and move "i" to metadata.labels["dash0.com/id"] if it is the id of an existing dashboard`},
		},
		"checkrule": {
			// The test document puts the origin in metadata.labels; a check rule reads it from its top-level labels.
			withNeither: {"", "", "GetCheckRule(dash0-cli_eda043ab83329c77b4ed7f465caea040)"},
			withOrigin:  {"", "", "GetCheckRule(dash0-cli_eda043ab83329c77b4ed7f465caea040)"},
			withID:      {"i", "i", "GetCheckRule(i)"},
			withBoth:    {"i", "i", "GetCheckRule(i)"},
		},
		"syntheticcheck": {
			withNeither: {"", "", "GetSyntheticCheck(dash0-cli_5d001689d30e63de937bf0bd6185d1e5)"},
			withOrigin:  {"o", "o", "GetSyntheticCheck(o)"},
			withID:      {"i", "i", "GetSyntheticCheck(i) GetSyntheticCheck(dash0-cli_5d001689d30e63de937bf0bd6185d1e5)"},
			withBoth:    {"i", "i", "GetSyntheticCheck(i) GetSyntheticCheck(o)"},
		},
		"slo": {
			withNeither: {"", "", "GetSLO(dash0-cli_ccd8d2fa6938a28d0b9fe487b335cfd2)"},
			withOrigin:  {"o", "o", "GetSLO(o)"},
			withID:      {"i", "i", "GetSLO(i) GetSLO(dash0-cli_ccd8d2fa6938a28d0b9fe487b335cfd2)"},
			withBoth:    {"i", "o", "GetSLO(i) GetSLO(o)"},
		},
		"view": {
			withNeither: {"", "", "GetView(dash0-cli_c2cd79b14cacc84c35f2891bf455de45)"},
			withOrigin:  {"o", "o", "GetView(o)"},
			withID:      {"i", "i", "GetView(i) GetView(dash0-cli_c2cd79b14cacc84c35f2891bf455de45)"},
			withBoth:    {"i", "i", "GetView(i) GetView(o)"},
		},
		"prometheusrule": {
			withNeither: {"", "", "GetCheckRule(dash0-cli_c8b5205e526cbeb9cb5fc77fad4e09f6) GetRecordingRule(dash0-cli_87a69fbd04c8b6bb51abb4e7465c67f6)"},
			withOrigin:  {"o", "o", "GetCheckRule(o) GetRecordingRule(o)"},
			withID:      {"i", "i", "GetCheckRule(i) GetRecordingRule(i) GetRecordingRule(dash0-cli_87a69fbd04c8b6bb51abb4e7465c67f6)"},
			withBoth:    {"i", "i", `error: the check rule sets id "i" and dash0.com/origin "o", which differ, so only the first apply would succeed; remove dash0.com/origin or the id`},
		},
		"persesdashboard": {
			withNeither: {"", "", "GetDashboard(dash0-cli_8c3789eb30701c9e4d02c5b21660f40d)"},
			withOrigin:  {"o", "o", "GetDashboard(o)"},
			withID:      {"i", "i", "GetDashboard(i) GetDashboard(dash0-cli_8c3789eb30701c9e4d02c5b21660f40d)"},
			withBoth:    {"i", "i", "GetDashboard(i) GetDashboard(o)"},
		},
		"spamfilter": {
			withNeither: {"", "", "GetSpamFilter(dash0-cli_c5d82d7d40200dc8059d794895be053f)"},
			withOrigin:  {"o", "", "GetSpamFilter(o)"},
			withID:      {"i", "i", "GetSpamFilter(i) GetSpamFilter(dash0-cli_c5d82d7d40200dc8059d794895be053f)"},
			withBoth:    {"i", "i", "GetSpamFilter(i) GetSpamFilter(o)"},
		},
		"notificationchannel": {
			withNeither: {"", "", "GetNotificationChannel(dash0-cli_42126ca28b4cc4a14dba5c2566ad1eec)"},
			withOrigin:  {"o", "o", "GetNotificationChannel(o)"},
			withID:      {"i", "i", "GetNotificationChannel(i) GetNotificationChannel(dash0-cli_42126ca28b4cc4a14dba5c2566ad1eec)"},
			withBoth:    {"i", "i", "GetNotificationChannel(i) GetNotificationChannel(o)"},
		},
		"team": {
			withNeither: {"", "", "GetTeam(dash0-cli_8083a7be84554318ca00370205d9cbc5)"},
			withOrigin:  {"o", "o", "GetTeam(o)"},
			withID:      {"i", "i", "GetTeam(i) GetTeam(dash0-cli_8083a7be84554318ca00370205d9cbc5)"},
			withBoth:    {"i", "o", "GetTeam(i) GetTeam(o)"},
		},
		"timeseriesaggregation": {
			withNeither: {"", "", "GetTimeSeriesAggregation(dash0-cli_921056c7dd8df46007acec262a6a50d3)"},
			withOrigin:  {"o", "o", "GetTimeSeriesAggregation(o)"},
			withID:      {"i", "", "GetTimeSeriesAggregation(i) GetTimeSeriesAggregation(dash0-cli_921056c7dd8df46007acec262a6a50d3)"},
			withBoth:    {"i", "o", "GetTimeSeriesAggregation(i) GetTimeSeriesAggregation(o)"},
		},
		"dataset": {
			withNeither: {"", "", "error: dataset definition requires a slug in spec.slug or metadata.labels[\"dash0.com/id\"]"},
			withOrigin:  {"", "", "error: dataset definition requires a slug in spec.slug or metadata.labels[\"dash0.com/id\"]"},
			withID:      {"i", "i", "GetDataset(i) GetDataset(dash0-cli_b7e674b12c0387a6c726538139d1b7d5)"},
			withBoth:    {"i", "i", "GetDataset(i) GetDataset(o)"},
		},
		"skill": {
			withNeither: {"", "", "GetCustomSkill(dash0-cli_b0d12560acda94b0cbbc694b1a9679c6)"},
			withOrigin:  {"o", "o", "GetCustomSkill(o)"},
			withID:      {"i", "i", "GetCustomSkill(i) GetCustomSkill(dash0-cli_b0d12560acda94b0cbbc694b1a9679c6)"},
			withBoth:    {"i", "o", "GetCustomSkill(i) GetCustomSkill(o)"},
		},
		"awsmetricscrapeconfig": {
			withNeither: {"", "", "error: Dash0AwsMetricScrapeConfig requires metadata.labels[\"dash0.com/id\"]: it is the id of the AWS integration whose scrape config the document replaces"},
			withOrigin:  {"", "", "error: Dash0AwsMetricScrapeConfig requires metadata.labels[\"dash0.com/id\"]: it is the id of the AWS integration whose scrape config the document replaces"},
			withID:      {"i", "i", "GetAwsMetricScrapeConfig(i) error: AWS integration \"i\" not found (status: 404)"},
			withBoth:    {"i", "i", "GetAwsMetricScrapeConfig(i) error: AWS integration \"i\" not found (status: 404)"},
		},
		"generativeaievaluation": {
			withNeither: {"", "", "GetGenerativeAiEvaluation(dash0-cli_d480f2fd11254a8ae007e52fc33719e4)"},
			withOrigin:  {"o", "o", "GetGenerativeAiEvaluation(o)"},
			withID:      {"i", "i", "GetGenerativeAiEvaluation(i) GetGenerativeAiEvaluation(dash0-cli_d480f2fd11254a8ae007e52fc33719e4)"},
			withBoth:    {"i", "o", "GetGenerativeAiEvaluation(i) GetGenerativeAiEvaluation(o)"},
		},
		"signaltometrics": {
			withNeither: {"", "", "GetSignalToMetrics(dash0-cli_c13dd324a80fe512832f0459609b21bc)"},
			withOrigin:  {"o", "o", "GetSignalToMetrics(o)"},
			withID:      {"i", "i", "GetSignalToMetrics(i) GetSignalToMetrics(dash0-cli_c13dd324a80fe512832f0459609b21bc)"},
			withBoth:    {"i", "o", "GetSignalToMetrics(i) GetSignalToMetrics(o)"},
		},
		"sampling": {
			withNeither: {"", "", "GetSamplingRule(dash0-cli_55f1e8303d29484bd22cfcfebb98fc98)"},
			withOrigin:  {"o", "o", "GetSamplingRule(o)"},
			withID:      {"i", "i", "GetSamplingRule(i) GetSamplingRule(dash0-cli_55f1e8303d29484bd22cfcfebb98fc98)"},
			withBoth:    {"i", "o", "GetSamplingRule(i) GetSamplingRule(o)"},
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
