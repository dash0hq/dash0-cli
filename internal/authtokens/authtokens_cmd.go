package authtokens

import (
	"fmt"
	"slices"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/spf13/cobra"
)

const (
	assetType    = "auth token"
	apiVersion   = "dash0.com/v1alpha1"
	allActions   = dash0api.AuthTokenAction("*")
	allowAllFlag = "allow-all-actions"
	// originPrefix makes the server report the token's source as dash0-cli.
	originPrefix = "dash0-cli_"
)

// NewAuthTokensCmd creates the auth-tokens parent command.
func NewAuthTokensCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth-tokens",
		Short: "[experimental] Manage auth tokens",
		Long: `Create, list, get, update, and delete the auth tokens of your Dash0 organization.

The API returns a token's secret only once, when the token is created.
'create' prints it on an interactive terminal, or writes it to --secret-file. No other command shows it.`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}

func validateAPIVersion(token *dash0api.AuthTokenDefinition) error {
	switch {
	case token.ApiVersion == "":
		return fmt.Errorf("auth token definition requires apiVersion: %s", apiVersion)
	case token.ApiVersion != apiVersion:
		return fmt.Errorf("unsupported auth token apiVersion %q (supported: %s)", token.ApiVersion, apiVersion)
	}
	return nil
}

// requireAllowAll refuses a document that grants every action unless the user
// passed --allow-all-actions. --force and agent mode never stand in for it,
// because such a token can manage every other token, including yours.
func requireAllowAll(token *dash0api.AuthTokenDefinition, allowAll bool) error {
	if allowAll || token.Spec.Actions == nil || !slices.Contains(*token.Spec.Actions, allActions) {
		return nil
	}
	return fmt.Errorf("the definition grants the '*' action, which gives the token full access to the organization, including its auth tokens\n"+
		"Hint: pass --%s to grant it, or list only the actions the token needs", allowAllFlag)
}

func originOf(token *dash0api.AuthTokenMetadata) string {
	if token.Labels != nil && token.Labels.Dash0Comorigin != nil {
		return *token.Labels.Dash0Comorigin
	}
	return ""
}

func idOf(token *dash0api.AuthTokenMetadata) string {
	if token.Labels != nil && token.Labels.Dash0Comid != nil {
		return token.Labels.Dash0Comid.String()
	}
	return ""
}

func actionsOf(token *dash0api.AuthTokenResponseDefinition) string {
	if token.Spec.Actions == nil {
		return ""
	}
	actions := make([]string, 0, len(*token.Spec.Actions))
	for _, action := range *token.Spec.Actions {
		actions = append(actions, string(action))
	}
	return strings.Join(actions, ",")
}
