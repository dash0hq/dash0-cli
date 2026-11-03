package awsmetricscrapeconfigs

import (
	"fmt"
	"os"

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

func printWarnings(warnings []string) {
	for _, warning := range warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
	}
}
