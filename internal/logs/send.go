package logs

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/version"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
)

type createFlags struct {
	OtlpUrl                        string
	AuthToken                      string
	Dataset                        string
	ResourceAttributes             []string
	LogAttributes                  []string
	SeverityNumber                 int
	SeverityText                   string
	Time                           string
	ObservedTime                   string
	TraceID                        string
	SpanID                         string
	EventName                      string
	Flags                          uint32
	ScopeName                      string
	ScopeVersion                   string
	ScopeAttributes                []string
	ResourceDroppedAttributesCount uint32
	ScopeDroppedAttributesCount    uint32
	LogDroppedAttributesCount      uint32
}

func newSendCmd() *cobra.Command {
	flags := &createFlags{}

	cmd := &cobra.Command{
		Use:     "send <body>",
		Aliases: []string{"create"},
		Short: "Send a log record to Dash0",
		Long: `Send a log record to Dash0 via OTLP.` + internal.CONFIG_HINT,
		Example: `  # Send a simple log message
  dash0 logs send "Application started"

  # Send with severity and service name
  dash0 logs send "Deployment completed" \
      --severity-text INFO --severity-number 9 \
      --resource-attribute service.name=my-service

  # Send a deployment event with attributes
  dash0 logs send "Deployment v2.1.0" \
      --event-name dash0.deployment \
      --severity-number 9 \
      --resource-attribute service.name=my-service \
      --log-attribute deployment.status=succeeded

  # Send with trace context
  dash0 logs send "Request processed" \
      --trace-id 0af7651916cd43dd8448eb211c80319c \
      --span-id b7ad6b7169203331`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd, args[0], flags)
		},
	}

	cmd.Flags().StringVar(&flags.OtlpUrl, "otlp-url", "", "OTLP endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().StringVar(&flags.Dataset, "dataset", "", "Dataset name")
	cmd.Flags().StringArrayVar(&flags.ResourceAttributes, "resource-attribute", nil, "Resource attribute as 'key=value' (repeatable)")
	cmd.Flags().StringArrayVar(&flags.LogAttributes, "log-attribute", nil, "Log record attribute as 'key=value' (repeatable)")
	cmd.Flags().IntVar(&flags.SeverityNumber, "severity-number", 0, "Severity number (1-24, see OpenTelemetry specification); this is used in Dash0 to caulculate the severity range and is separate from severity-text")
	cmd.Flags().StringVar(&flags.SeverityText, "severity-text", "", "Severity text (e.g., INFO, WARN, ERROR); this is separate from severity-number and can be used to provide custom severity levels the way logging libraries do")
	cmd.Flags().StringVar(&flags.Time, "time", "", "Log record timestamp in RFC3339 format, e.g. '2024-03-15T10:30:00.123456789Z'; defaults to now")
	cmd.Flags().StringVar(&flags.ObservedTime, "observed-time", "", "Observed timestamp in RFC3339 format, e.g. '2024-03-15T10:30:00.123456789Z'; defaults to now")
	cmd.Flags().StringVar(&flags.TraceID, "trace-id", "", "Trace ID (32 hex characters)")
	cmd.Flags().StringVar(&flags.SpanID, "span-id", "", "Span ID (16 hex characters)")
	cmd.Flags().StringVar(&flags.EventName, "event-name", "", "Event name")
	cmd.Flags().Uint32Var(&flags.Flags, "flags", 0, "Log record flags")
	cmd.Flags().StringVar(&flags.ScopeName, "scope-name", "dash0-cli", "Instrumentation scope name; defaults to 'dash0-cli'")
	cmd.Flags().StringVar(&flags.ScopeVersion, "scope-version", version.Version, "Instrumentation scope version; defaults to the dash0 CLI version")
	cmd.Flags().StringArrayVar(&flags.ScopeAttributes, "scope-attribute", nil, "Instrumentation scope attribute as 'key=value' (repeatable)")
	cmd.Flags().Uint32Var(&flags.ResourceDroppedAttributesCount, "resource-dropped-attributes-count", 0, "Number of dropped resource attributes")
	cmd.Flags().Uint32Var(&flags.ScopeDroppedAttributesCount, "scope-dropped-attributes-count", 0, "Number of dropped instrumentation scope attributes")
	cmd.Flags().Uint32Var(&flags.LogDroppedAttributesCount, "log-dropped-attributes-count", 0, "Number of dropped log record attributes")

	return cmd
}

func runCreate(cmd *cobra.Command, body string, flags *createFlags) error {
	ctx := cmd.Context()

	resolveScopeDefaults(cmd, flags)

	resourceAttrs, err := parseKeyValuePairs(flags.ResourceAttributes)
	if err != nil {
		return fmt.Errorf("invalid resource attribute: %w", err)
	}

	logAttrs, err := parseKeyValuePairs(flags.LogAttributes)
	if err != nil {
		return fmt.Errorf("invalid log attribute: %w", err)
	}

	scopeAttrs, err := parseKeyValuePairs(flags.ScopeAttributes)
	if err != nil {
		return fmt.Errorf("invalid scope attribute: %w", err)
	}

	now := time.Now()

	logTimestamp := now
	if flags.Time != "" {
		logTimestamp, err = time.Parse(time.RFC3339Nano, flags.Time)
		if err != nil {
			return fmt.Errorf("invalid time format (expected RFC3339 with optional nanoseconds, e.g. '2024-03-15T10:30:00.123456789Z'): %w", err)
		}
	}

	observedTimestamp := now
	if flags.ObservedTime != "" {
		observedTimestamp, err = time.Parse(time.RFC3339Nano, flags.ObservedTime)
		if err != nil {
			return fmt.Errorf("invalid observed-time format (expected RFC3339 with optional nanoseconds, e.g. '2024-03-15T10:30:00.123456789Z'): %w", err)
		}
	}

	// Build plog.Logs
	logs := plog.NewLogs()
	rl := logs.ResourceLogs().AppendEmpty()

	// Set resource attributes
	resource := rl.Resource()
	for k, v := range resourceAttrs {
		resource.Attributes().PutStr(k, v)
	}
	if flags.ResourceDroppedAttributesCount != 0 {
		resource.SetDroppedAttributesCount(flags.ResourceDroppedAttributesCount)
	}

	// Set scope
	sl := rl.ScopeLogs().AppendEmpty()
	scope := sl.Scope()
	scope.SetName(flags.ScopeName)
	scope.SetVersion(flags.ScopeVersion)
	for k, v := range scopeAttrs {
		scope.Attributes().PutStr(k, v)
	}
	if flags.ScopeDroppedAttributesCount != 0 {
		scope.SetDroppedAttributesCount(flags.ScopeDroppedAttributesCount)
	}

	// Build log record
	lr := sl.LogRecords().AppendEmpty()
	lr.Body().SetStr(body)
	lr.SetTimestamp(pcommon.NewTimestampFromTime(logTimestamp))
	lr.SetObservedTimestamp(pcommon.NewTimestampFromTime(observedTimestamp))

	if flags.SeverityNumber != 0 {
		lr.SetSeverityNumber(plog.SeverityNumber(flags.SeverityNumber))
	}
	if flags.SeverityText != "" {
		lr.SetSeverityText(flags.SeverityText)
	}

	for k, v := range logAttrs {
		lr.Attributes().PutStr(k, v)
	}

	if (flags.TraceID != "") != (flags.SpanID != "") {
		return fmt.Errorf("both --trace-id and --span-id must be specified together")
	}

	if flags.TraceID != "" {
		traceID, err := parseTraceID(flags.TraceID)
		if err != nil {
			return err
		}
		lr.SetTraceID(traceID)
	}

	if flags.SpanID != "" {
		spanID, err := parseSpanID(flags.SpanID)
		if err != nil {
			return err
		}
		lr.SetSpanID(spanID)
	}

	if flags.EventName != "" {
		lr.SetEventName(flags.EventName)
	}

	if flags.Flags != 0 {
		lr.SetFlags(plog.LogRecordFlags(flags.Flags))
	}

	if flags.LogDroppedAttributesCount != 0 {
		lr.SetDroppedAttributesCount(flags.LogDroppedAttributesCount)
	}

	// Create OTLP client and send
	apiClient, err := client.NewOtlpClientFromContext(ctx, flags.OtlpUrl, flags.AuthToken)
	if err != nil {
		return err
	}
	defer apiClient.Close(ctx)

	if err := apiClient.SendLogs(ctx, logs, client.ResolveDataset(ctx, flags.Dataset)); err != nil {
		return fmt.Errorf("failed to send log record: %w", err)
	}

	fmt.Println("Log record sent successfully")
	return nil
}

// parseKeyValuePairs parses a slice of "key=value" strings into a map.
func parseKeyValuePairs(pairs []string) (map[string]string, error) {
	result := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		k, v, ok := strings.Cut(pair, "=")
		if !ok {
			return nil, fmt.Errorf("expected key=value format, got %q", pair)
		}
		if k == "" {
			return nil, fmt.Errorf("empty key in %q", pair)
		}
		result[k] = v
	}
	return result, nil
}

// parseTraceID parses a 32 hex character string into a pcommon.TraceID.
func parseTraceID(s string) (pcommon.TraceID, error) {
	if len(s) != 32 {
		return pcommon.TraceID{}, fmt.Errorf("trace-id must be 32 hex characters, got %d", len(s))
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return pcommon.TraceID{}, fmt.Errorf("trace-id must be valid hex: %w", err)
	}
	var tid pcommon.TraceID
	copy(tid[:], b)
	return tid, nil
}

// parseSpanID parses a 16 hex character string into a pcommon.SpanID.
func parseSpanID(s string) (pcommon.SpanID, error) {
	if len(s) != 16 {
		return pcommon.SpanID{}, fmt.Errorf("span-id must be 16 hex characters, got %d", len(s))
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return pcommon.SpanID{}, fmt.Errorf("span-id must be valid hex: %w", err)
	}
	var sid pcommon.SpanID
	copy(sid[:], b)
	return sid, nil
}

// resolveScopeDefaults clears the default value for scope-name or scope-version
// when only the other flag is explicitly set. This avoids pairing a custom scope
// name with the dash0-cli version (or vice versa).
func resolveScopeDefaults(cmd *cobra.Command, flags *createFlags) {
	scopeNameChanged := cmd.Flags().Changed("scope-name")
	scopeVersionChanged := cmd.Flags().Changed("scope-version")
	if scopeNameChanged && !scopeVersionChanged {
		flags.ScopeVersion = ""
	} else if scopeVersionChanged && !scopeNameChanged {
		flags.ScopeName = ""
	}
}
