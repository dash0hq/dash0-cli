package awsmetricscrapeconfigs

import (
	"fmt"
	"os"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/client"
)

// displayKind is the display name used in success messages and diffs.
const displayKind = "AWS metric scrape config"

type connectionFlags struct {
	ApiUrl    string
	AuthToken string
}

// errorContext names the integration, because a 404 means the integration does
// not exist: every existing integration has a config, even an empty one.
func errorContext(integrationID string) client.ErrorContext {
	return client.ErrorContext{AssetType: "AWS integration", AssetID: integrationID}
}

// requireConfig guards against a 200 whose body the client could not decode,
// which yields a nil config and a nil error.
func requireConfig(cfg *dash0api.AwsMetricScrapeConfigDefinition, integrationID string) error {
	if cfg == nil {
		return fmt.Errorf("the API returned no AWS metric scrape config for integration %q", integrationID)
	}
	return nil
}

func printWarnings(warnings []string) {
	for _, warning := range warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
	}
}
