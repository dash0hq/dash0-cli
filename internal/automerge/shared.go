package automerge

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/client"
)

// displayKind is the display name used in success messages and diffs.
const displayKind = "AutoMerge settings"

type connectionFlags struct {
	ApiUrl    string
	AuthToken string
}

// repositoryRef is how a command names a repository: an 'owner/repo' slug or an id.
type repositoryRef struct {
	ID   string
	Slug string
}

func (r repositoryRef) String() string {
	if r.Slug != "" {
		return r.Slug
	}
	return r.ID
}

// parseRepositoryRef reads a slug when the value has a slash. Repository ids never do.
func parseRepositoryRef(value string) repositoryRef {
	value = strings.TrimSpace(value)
	if strings.Contains(value, "/") {
		return repositoryRef{Slug: value}
	}
	return repositoryRef{ID: value}
}

// resolveRepositoryID returns the id of the repository ref names, looking a slug up.
func resolveRepositoryID(ctx context.Context, apiClient dash0api.Client, ref repositoryRef) (string, error) {
	if ref.ID != "" {
		return ref.ID, nil
	}
	repository, err := apiClient.FindDarkplaneRepository(ctx, ref.Slug)
	if err != nil {
		return "", handleError(err, ref)
	}
	return dash0api.GetDarkplaneRepositoryID(repository), nil
}

// handleError explains the superseded-version rejection, which the API reports as a plain 400.
func handleError(err error, ref repositoryRef) error {
	var apiErr *dash0api.APIError
	if errors.As(err, &apiErr) && strings.Contains(strings.ToLower(apiErr.Message+apiErr.Body), "superseded") {
		return fmt.Errorf("the AutoMerge settings of %s changed since they were read: run 'get -o yaml' again and redo your edit, or remove metadata.labels[\"dash0.com/version\"] to overwrite them", ref)
	}
	return client.HandleAPIError(err, client.ErrorContext{AssetType: "repository", AssetID: ref.String()})
}

func printWarnings(warnings []string) {
	for _, warning := range warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
	}
}
