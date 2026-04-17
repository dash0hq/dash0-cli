// Package rawapi implements the experimental `dash0 api` command, a raw HTTP
// passthrough that reuses the active profile's connection settings.
package rawapi

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/spf13/cobra"
)

type apiFlags struct {
	ApiUrl    string
	AuthToken string
	Dataset   string
	File      string
	Headers   []string
	Verbose   bool
}

// NewAPICmd creates the experimental `api` command.
func NewAPICmd() *cobra.Command {
	flags := &apiFlags{}

	cmd := &cobra.Command{
		Use:   "api [METHOD] <path>",
		Short: "[experimental] Call a Dash0 API endpoint directly",
		Long: `Call a Dash0 API endpoint directly, reusing the active profile's API URL,
authentication token, and (by default) dataset.

Useful for endpoints that do not yet have a dedicated subcommand.

Relative paths must start with /api/ and are resolved against the profile's
api-url. Absolute URLs (http:// or https://) are passed through verbatim.

Query parameters are baked into the path. Headers are set with -H.
Request bodies are read from a file or stdin with -f.`,
		Example: `  # GET — dataset auto-injected from the active profile
  dash0 -X api /api/signal-to-metrics/configs

  # GET against an organization-level endpoint that does not take a dataset
  dash0 -X api /api/organization/settings --dataset ""

  # GET with query parameters baked into the path
  dash0 -X api "/api/signal-to-metrics/configs?limit=50&enabled=true"

  # POST with a payload from a file
  dash0 -X api POST /api/signal-to-metrics/configs -f config.json

  # POST with a payload from stdin and a custom header
  echo '{"name":"my-config"}' | dash0 -X api POST /api/signal-to-metrics/configs -f - -H 'X-Request-Id: abc123'

  # Debug a failing request
  dash0 -X api POST /api/signal-to-metrics/configs -f config.json -v`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runAPI(cmd, args, flags)
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().StringVar(&flags.Dataset, "dataset", "", `Dataset query parameter (overrides active profile; pass "" to skip injection)`)
	cmd.Flags().StringVarP(&flags.File, "file", "f", "", `Request body from file, or "-" for stdin`)
	cmd.Flags().StringArrayVarP(&flags.Headers, "header", "H", nil, "Request header in 'Key: Value' form (repeatable)")
	cmd.Flags().BoolVarP(&flags.Verbose, "verbose", "v", false, "Print request and response details to stderr")

	return cmd
}

func runAPI(cmd *cobra.Command, args []string, flags *apiFlags) error {
	ctx := cmd.Context()

	cfg, err := client.NewRawHTTPConfig(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	method, path, err := parseMethodAndPath(args)
	if err != nil {
		return err
	}

	body, err := readBody(flags.File, cmd.InOrStdin())
	if err != nil {
		return err
	}

	if method == http.MethodGet && body != nil {
		return fmt.Errorf("cannot combine GET with --file; use an explicit body-bearing method (POST, PUT, PATCH)")
	}

	headers, err := parseHeaders(flags.Headers)
	if err != nil {
		return err
	}

	req, err := buildRequest(buildRequestInput{
		BaseURL:           cfg.ApiUrl,
		Path:              path,
		Method:            method,
		ProfileDataset:    cfg.Dataset,
		FlagDataset:       flags.Dataset,
		DatasetFlagSet:    cmd.Flags().Changed("dataset"),
		Body:              body,
		Headers:           headers,
		AuthToken:         cfg.AuthToken,
		AuthTokenFromFlag: flags.AuthToken != "",
		UserAgent:         cfg.UserAgent,
	})
	if err != nil {
		return err
	}

	if flags.Verbose {
		printRequest(os.Stderr, req, body)
	}

	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if flags.Verbose {
		printResponseHead(os.Stderr, resp)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if len(respBody) > 0 {
		if _, err := os.Stdout.Write(respBody); err != nil {
			return fmt.Errorf("failed to write response: %w", err)
		}
		if respBody[len(respBody)-1] != '\n' {
			fmt.Fprintln(os.Stdout)
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("request failed with HTTP %s", resp.Status)
	}

	return nil
}

// printRequest writes the outbound request line, headers (with Authorization
// redacted) and body to w.
func printRequest(w io.Writer, req *http.Request, body []byte) {
	fmt.Fprintf(w, "> %s %s\n", req.Method, req.URL.String())
	writeHeaders(w, ">", req.Header, true)
	if len(body) > 0 {
		fmt.Fprintln(w, ">")
		_, _ = w.Write(body)
		if len(body) > 0 && body[len(body)-1] != '\n' {
			fmt.Fprintln(w)
		}
	}
}

// printResponseHead writes the response status line and headers to w.
func printResponseHead(w io.Writer, resp *http.Response) {
	fmt.Fprintf(w, "< %s\n", resp.Status)
	writeHeaders(w, "<", resp.Header, false)
	fmt.Fprintln(w, "<")
}

func writeHeaders(w io.Writer, prefix string, h http.Header, redactAuth bool) {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		seen := make(map[string]struct{})
		for _, v := range h.Values(key) {
			if redactAuth && strings.EqualFold(key, "Authorization") {
				v = "<redacted>"
			}
			if _, dup := seen[v]; dup {
				continue
			}
			seen[v] = struct{}{}
			fmt.Fprintf(w, "%s %s: %s\n", prefix, key, v)
		}
	}
}
