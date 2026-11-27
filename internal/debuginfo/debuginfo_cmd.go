package debuginfo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/asset"
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
Dash0 uses it to resolve the frames of profiles and browser errors to function names, files, and lines when you read them.`,
	}
	cmd.AddCommand(newUploadCmd())
	return cmd
}

type uploadFlags struct {
	Type        string
	BaseURL     string
	ScriptURL   string
	SourceMap   string
	Endpoint    string
	OtlpUrl     string
	AuthToken   string
	Output      string
	DryRun      bool
	Force       bool
	Concurrency int
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
In a directory, every .js, .mjs, and .cjs file with a source map is uploaded, outside node_modules.
The source map is the file named by the //# sourceMappingURL= comment of the script, or <file>.map
when it has none.

Dash0 finds the source map of a frame in one of two ways:
  - By a debug ID read from the end of the script. This is the default, and works for Node.js profiles
    and browser errors. Scripts without a debug ID get one: the command writes it into the script and
    its source map, so run it after the build and before you package or deploy the build output.
  - By the URL the script is served from. This works for browser profiles and browser errors. Pass
    --base-url with the public URL of the directory, or --script-url with the full URL of a single
    script. No file is changed.
Files that Dash0 already has are skipped; pass --force to replace them.` + internal.CONFIG_HINT,
		Example: `  # Add debug IDs to a Node.js build and upload its source maps
  dash0 -X debuginfo upload --type source-map dist

  # Preview the source maps of a Vite build served from the site root
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
	cmd.Flags().StringVarP(&flags.Output, "output", "o", "", "Output format: table, json (default: table; json in agent mode)")
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false, "List what would be uploaded without changing files or uploading")
	cmd.Flags().BoolVar(&flags.Force, "force", false, "Upload even when Dash0 already has the debug information")
	cmd.Flags().IntVar(&flags.Concurrency, "concurrency", 8, "Maximum number of uploads in flight at the same time")
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
	if flags.Concurrency < 1 {
		return fmt.Errorf("--concurrency must be at least 1, got %d", flags.Concurrency)
	}

	jsonOutput, err := parseOutputFormat(flags.Output)
	if err != nil {
		return err
	}

	artifacts, err := resolveSourceMaps(inputPath, flags)
	if err != nil {
		return err
	}

	if err := checkSharedSourceMaps(artifacts); err != nil {
		return err
	}

	var writes []fileWrite
	injected := 0
	for _, artifact := range artifacts {
		writes = append(writes, artifact.writes...)
		if artifact.Injected {
			injected++
		}
	}

	results := make([]uploadResult, 0, len(artifacts))
	if flags.DryRun {
		// The dry run fails on a read-only build output, like the real run.
		if err := checkWritable(writes); err != nil {
			return err
		}
		for _, artifact := range artifacts {
			results = append(results, uploadResult{sourceMapArtifact: artifact, Status: outcomePending})
		}
		return printDryRun(results, jsonOutput)
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
	maxRetries, err := client.ResolveMaxRetries(cmd.Context())
	if err != nil {
		return err
	}
	debuginfoUploader, closeConnection, err := dialUploader(target, transport, authToken, flags.Force, maxRetries)
	if err != nil {
		return err
	}
	defer func() { _ = closeConnection() }()

	// Credentials resolve first, so that a missing token fails before any file changes.
	if err := checkWritable(writes); err != nil {
		return err
	}
	for _, write := range writes {
		if err := writeFileAtomically(write); err != nil {
			return err
		}
	}
	if injected > 0 && !jsonOutput {
		fmt.Printf("Injected debug IDs into %s\n", asset.Pluralize(injected, "script"))
	}

	upload := func(artifact sourceMapArtifact) (uploadOutcome, string, error) {
		body, err := artifact.readBundle()
		if err != nil {
			return "", "", err
		}
		status, reason, err := debuginfoUploader.upload(cmd.Context(), artifact.BuildID, debuginfoType, artifact.buildIDType(), body)
		if err != nil {
			return "", "", fmt.Errorf("source map %q (%s):\n  %w", artifact.BuildID, artifact.Script, err)
		}
		return status, reason, nil
	}
	report := func(result uploadResult) {
		results = append(results, result)
		if !jsonOutput {
			printResultLine(result)
		}
	}
	uploadErr := uploadInOrder(artifacts, flags.Concurrency, upload, report)
	if jsonOutput {
		if err := printJSON(results); err != nil && uploadErr == nil {
			return err
		}
	}
	return uploadErr
}

// uploadInOrder uploads up to concurrency artifacts at a time and reports each result in the order of artifacts.
// Artifacts that share a build ID are uploaded once. After an upload fails, no new upload starts, the uploads in
// flight finish and are reported, and the errors of all failed uploads are returned in the order of artifacts.
func uploadInOrder(artifacts []sourceMapArtifact, concurrency int, upload func(sourceMapArtifact) (uploadOutcome, string, error), report func(uploadResult)) error {
	results := make([]uploadResult, len(artifacts))
	errs := make([]error, len(artifacts))
	finished := make([]bool, len(artifacts))
	firstByBuildID := map[string]int{}
	for index, artifact := range artifacts {
		if first, seen := firstByBuildID[artifact.BuildID]; seen {
			results[index] = uploadResult{sourceMapArtifact: artifact, Status: outcomeSkipped, Reason: "same source map as " + artifacts[first].Script}
		} else {
			firstByBuildID[artifact.BuildID] = index
		}
	}

	var mutex sync.Mutex
	failed := false
	nextToReport := 0
	reportFinished := func() {
		for ; nextToReport < len(artifacts); nextToReport++ {
			first := firstByBuildID[artifacts[nextToReport].BuildID]
			if !finished[first] {
				return
			}
			if errs[first] == nil {
				report(results[nextToReport])
			}
		}
	}

	slots := make(chan struct{}, concurrency)
	var running sync.WaitGroup
	for index, artifact := range artifacts {
		if firstByBuildID[artifact.BuildID] != index {
			continue
		}
		slots <- struct{}{}
		mutex.Lock()
		stop := failed
		mutex.Unlock()
		if stop {
			break
		}
		running.Go(func() {
			defer func() { <-slots }()
			status, reason, err := upload(artifact)
			mutex.Lock()
			defer mutex.Unlock()
			results[index] = uploadResult{sourceMapArtifact: artifact, Status: status, Reason: reason}
			errs[index] = err
			finished[index] = true
			failed = failed || err != nil
			reportFinished()
		})
	}
	running.Wait()
	return errors.Join(errs...)
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
			if _, err := os.Stat(filepath.Join(inputPath, "index.html")); err == nil {
				fmt.Fprintf(os.Stderr, "Warning: %s has an index.html, so it looks like a browser build. Without --base-url, debug IDs are written into the scripts, which resolves browser errors but not browser profiles; pass --base-url to cover both.\n", inputPath)
			}
		}
		artifacts, skipped, err := discoverSourceMaps(inputPath, flags.BaseURL)
		if err != nil {
			return nil, err
		}
		for _, script := range skipped {
			fmt.Fprintf(os.Stderr, "Skipped %s: %s\n", script.Script, script.Reason)
		}
		if len(skipped) > 0 {
			fmt.Fprintf(os.Stderr, "Skipped %s\n", asset.Pluralize(len(skipped), "path"))
		}
		if len(artifacts) == 0 {
			return nil, fmt.Errorf("no script with a source map found in %s", inputPath)
		}
		return artifacts, nil
	}

	if flags.BaseURL != "" && flags.ScriptURL != "" {
		return nil, fmt.Errorf("pass only one of --base-url and --script-url for a single script")
	}
	scriptURL := flags.ScriptURL
	if flags.BaseURL != "" {
		if scriptURL, err = joinBaseURL(flags.BaseURL, filepath.Base(inputPath)); err != nil {
			return nil, err
		}
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
	artifact, err := newSourceMapArtifact(inputPath, mapPath, scriptURL)
	if err != nil {
		return nil, err
	}
	return []sourceMapArtifact{artifact}, nil
}

func printDryRun(results []uploadResult, jsonOutput bool) error {
	if jsonOutput {
		return printJSON(results)
	}
	fmt.Printf("Dry run: %s found\n", asset.Pluralize(len(results), "source map"))
	for _, result := range results {
		note := ""
		if result.Injected {
			note = " (debug ID would be injected)"
		}
		fmt.Printf("  %s <- %s, %s%s\n", result.BuildID, result.Script, result.SourceMap, note)
	}
	return nil
}

// parseOutputFormat reports whether results print as JSON.
func parseOutputFormat(format string) (bool, error) {
	switch strings.ToLower(format) {
	case "":
		return agentmode.Enabled, nil
	case "table":
		return false, nil
	case "json":
		return true, nil
	}
	return false, fmt.Errorf("unknown --output %q; one of: table, json", format)
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
