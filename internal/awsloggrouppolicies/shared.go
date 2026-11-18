package awsloggrouppolicies

import (
	"fmt"
	"os"

	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
)

const displayKind = asset.AwsLogGroupPolicyDisplayKind

type connectionFlags struct {
	ApiUrl    string
	AuthToken string
}

func errorContext(integrationID string) client.ErrorContext {
	return client.ErrorContext{AssetType: displayKind, AssetID: integrationID}
}

func printWarnings(warnings []string) {
	for _, warning := range warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
	}
}
