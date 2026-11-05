package debuginfo

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/dash0hq/dash0-cli/internal/output"
	debuginfopb "github.com/parca-dev/parca/gen/proto/go/parca/debuginfo/v1alpha1"
	"github.com/spf13/cobra"
)

const typeSourceMap = "source-map"

// Every type the Dash0 debuginfo service stores, so that the flag can grow without a new command.
var debuginfoTypes = map[string]debuginfopb.DebuginfoType{
	"debuginfo":   debuginfopb.DebuginfoType_DEBUGINFO_TYPE_DEBUGINFO_UNSPECIFIED,
	"executable":  debuginfopb.DebuginfoType_DEBUGINFO_TYPE_EXECUTABLE,
	"sources":     debuginfopb.DebuginfoType_DEBUGINFO_TYPE_SOURCES,
	typeSourceMap: debuginfopb.DebuginfoType_DEBUGINFO_TYPE_SOURCE_MAP,
}

const debuginfoTypeNames = "debuginfo, executable, sources, source-map"

// NewDebuginfoCmd creates the debuginfo parent command.
func NewDebuginfoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "debuginfo",
		Short: "[experimental] Upload debug information so Dash0 can symbolize profiles",
		Long: `Upload debug information to Dash0.
Dash0 uses it to resolve the frames of profiles to function names, files, and lines when you read them.`,
	}
	cmd.AddCommand(newUploadCmd())
	return cmd
}

type uploadFlags struct {
	Type      string
	BaseURL   string
	ScriptURL string
	SourceMap string
	Endpoint  string
	OtlpUrl   string
	AuthToken string
	DryRun    bool
	Force     bool
}

func newUploadCmd() *cobra.Command {
	flags := &uploadFlags{}
	cmd := &cobra.Command{
		Use:   "upload <path>",
		Short: "[experimental] Upload debug information to Dash0",
		Long: `Upload debug information to Dash0.

--type selects what to upload: ` + debuginfoTypeNames + `.
Only source-map is supported yet.

For source maps, <path> is a script or a build output directory.
In a directory, every .js, .mjs, and .cjs file with a source map is uploaded. The source map is the
file named by the //# sourceMappingURL= comment of the script, or <file>.map when it has none.
Dash0 matches a source map to a browser profile frame by the URL the script is served from, so pass
--base-url with the public URL of the directory, or --script-url with the full URL of a single script.
Files that Dash0 already has are skipped; pass --force to replace them.` + internal.CONFIG_HINT,
		Example: `  # Preview the source maps of a Vite build served from the site root
  dash0 -X debuginfo upload --type source-map dist --base-url https://app.example.com/ --dry-run

  # Upload the source maps of a Next.js build
  dash0 -X debuginfo upload --type source-map .next/static --base-url https://app.example.com/_next/static/

  # Upload one script and its source map
  dash0 -X debuginfo upload --type source-map dist/assets/app.js --script-url https://app.example.com/assets/app.js

  # Replace a source map Dash0 already has, for a script served from a URL that does not change between releases
  dash0 -X debuginfo upload --type source-map dist --base-url https://app.example.com/ --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runUpload(cmd, args[0], flags)
		},
	}

	cmd.Flags().StringVar(&flags.Type, "type", "", "Type of debug information: "+debuginfoTypeNames+" (required; only source-map is supported yet)")
	cmd.Flags().StringVar(&flags.BaseURL, "base-url", "", "Public URL that <path> is served from; the URL of each script is this URL plus its path relative to <path>")
	cmd.Flags().StringVar(&flags.ScriptURL, "script-url", "", "Public URL of the script, when <path> is a single script")
	cmd.Flags().StringVar(&flags.SourceMap, "source-map", "", "Source map of the script, when <path> is a single script (default: the file its sourceMappingURL comment names, or <path>.map)")
	cmd.Flags().StringVar(&flags.Endpoint, "endpoint", "", "URL of the Dash0 debuginfo gRPC service (default: the host of the OTLP URL on port 4317)")
	cmd.Flags().StringVar(&flags.OtlpUrl, "otlp-url", "", "OTLP endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile); it must grant ingest:debuginfo")
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false, "List what would be uploaded without uploading")
	cmd.Flags().BoolVar(&flags.Force, "force", false, "Upload even when Dash0 already has the debug information")
	return cmd
}

type uploadResult struct {
	sourceMapArtifact
	Status uploadOutcome `json:"status"`
	Reason string        `json:"reason,omitempty"`
}

func runUpload(cmd *cobra.Command, inputPath string, flags *uploadFlags) error {
	debuginfoType, known := debuginfoTypes[flags.Type]
	switch {
	case flags.Type == "":
		return fmt.Errorf("--type is required; one of: %s", debuginfoTypeNames)
	case !known:
		return fmt.Errorf("unknown --type %q; one of: %s", flags.Type, debuginfoTypeNames)
	case flags.Type != typeSourceMap:
		return fmt.Errorf("--type %s is not supported yet; only source-map can be uploaded", flags.Type)
	}

	artifacts, err := resolveSourceMaps(inputPath, flags)
	if err != nil {
		return err
	}

	results := make([]uploadResult, 0, len(artifacts))
	if flags.DryRun {
		for _, artifact := range artifacts {
			results = append(results, uploadResult{sourceMapArtifact: artifact, Status: outcomePending})
		}
		return printDryRun(results)
	}

	otlpURLOverride := flags.OtlpUrl
	if flags.Endpoint != "" {
		otlpURLOverride = flags.Endpoint
	}
	otlpURL, authToken, err := client.ResolveOtlpCredentials(cmd.Context(), otlpURLOverride, flags.AuthToken)
	if err != nil {
		return err
	}
	target, transport, err := debuginfoTarget(otlpURL, flags.Endpoint != "")
	if err != nil {
		return err
	}
	debuginfoUploader, closeConnection, err := dialUploader(target, transport, authToken, flags.Force)
	if err != nil {
		return err
	}
	defer func() { _ = closeConnection() }()

	for _, artifact := range artifacts {
		body, err := artifact.readBundle()
		if err != nil {
			return err
		}
		outcome, reason, err := debuginfoUploader.upload(cmd.Context(), artifact.BuildID, debuginfoType, body)
		if err != nil {
			return fmt.Errorf("source map %q (%s):\n  %w", artifact.BuildID, artifact.Script, err)
		}
		result := uploadResult{sourceMapArtifact: artifact, Status: outcome, Reason: reason}
		results = append(results, result)
		if !agentmode.Enabled {
			printResultLine(result)
		}
	}
	if agentmode.Enabled {
		return printJSON(results)
	}
	return nil
}

func resolveSourceMaps(inputPath string, flags *uploadFlags) ([]sourceMapArtifact, error) {
	info, err := os.Stat(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s:\n  %w", inputPath, err)
	}

	if info.IsDir() {
		if flags.ScriptURL != "" || flags.SourceMap != "" {
			return nil, fmt.Errorf("--script-url and --source-map apply to a single script; use --base-url for a directory")
		}
		if flags.BaseURL == "" {
			return nil, fmt.Errorf("--base-url is required when <path> is a directory")
		}
		artifacts, skipped, err := discoverSourceMaps(inputPath, flags.BaseURL)
		if err != nil {
			return nil, err
		}
		for _, script := range skipped {
			fmt.Fprintf(os.Stderr, "Skipped %s: %s\n", script.Script, script.Reason)
		}
		if len(skipped) > 0 {
			fmt.Fprintf(os.Stderr, "Skipped %s without a source map to upload\n", pluralize(len(skipped), "script"))
		}
		if len(artifacts) == 0 {
			return nil, fmt.Errorf("no script with a source map found in %s", inputPath)
		}
		return artifacts, nil
	}

	if (flags.BaseURL == "") == (flags.ScriptURL == "") {
		return nil, fmt.Errorf("pass exactly one of --base-url and --script-url for a single script")
	}
	scriptURL := flags.ScriptURL
	if scriptURL == "" {
		if scriptURL, err = joinBaseURL(flags.BaseURL, filepath.Base(inputPath)); err != nil {
			return nil, err
		}
	}
	buildID, err := sourceMapBuildID(scriptURL)
	if err != nil {
		return nil, err
	}
	mapPath := flags.SourceMap
	if mapPath == "" {
		located, skipReason, err := locateSourceMap(inputPath)
		if err != nil {
			return nil, err
		}
		if skipReason != "" {
			return nil, fmt.Errorf("%s: %s; pass --source-map to name the map", inputPath, skipReason)
		}
		mapPath = located
	}
	return []sourceMapArtifact{{BuildID: buildID, ScriptURL: scriptURL, Script: inputPath, SourceMap: mapPath}}, nil
}

func printDryRun(results []uploadResult) error {
	if agentmode.Enabled {
		return printJSON(results)
	}
	fmt.Printf("Dry run: %s found\n", pluralize(len(results), "source map"))
	for _, result := range results {
		fmt.Printf("  %s <- %s, %s\n", result.BuildID, result.Script, result.SourceMap)
	}
	return nil
}

func printResultLine(result uploadResult) {
	if result.Status == outcomeSkipped {
		fmt.Printf("Source map %q skipped: %s\n", result.BuildID, result.Reason)
		return
	}
	fmt.Printf("Source map %q uploaded\n", result.BuildID)
}

func printJSON(results []uploadResult) error {
	return output.NewFormatter(output.FormatJSON, os.Stdout).PrintJSON(results)
}

func pluralize(count int, noun string) string {
	if count == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", count, noun)
}
