package tracing

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/dash0hq/dash0-cli/internal/otlp"
	"github.com/dash0hq/dash0-cli/internal/version"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type sendFlags struct {
	OtlpUrl            string
	AuthToken          string
	Dataset            string
	Name               string
	Kind               string
	StatusCode         string
	StatusMessage      string
	StartTime          string
	EndTime            string
	Duration           string
	TraceID            string
	SpanID             string
	ParentSpanID       string
	ResourceAttributes []string
	SpanAttributes     []string
	SpanLinks          []string
	ScopeName          string
	ScopeVersion       string
	ScopeAttributes    []string
}

func newSendCmd() *cobra.Command {
	flags := &sendFlags{}

	cmd := &cobra.Command{
		Use:   "send",
		Short: "[experimental] Send a span to Dash0",
		Long:  `Send a span to Dash0 via OTLP.` + internal.CONFIG_HINT,
		Example: `  # Send a simple span
  dash0 --experimental spans send --name "my-operation"

  # Send a server span with duration
  dash0 --experimental spans send --name "GET /api/users" \
      --kind SERVER --status-code OK --duration 100ms \
      --resource-attribute service.name=my-service

  # Send a span with explicit start and end times
  dash0 --experimental spans send --name "batch-job" \
      --kind INTERNAL \
      --start-time 2024-03-15T10:30:00Z \
      --end-time 2024-03-15T10:30:01.500Z

  # Send a span with a link to another trace
  dash0 --experimental spans send --name "process-message" \
      --kind CONSUMER \
      --span-link 0af7651916cd43dd8448eb211c80319c:b7ad6b7169203331

  # Send a span with parent
  dash0 --experimental spans send --name "db-query" \
      --kind CLIENT \
      --trace-id 0af7651916cd43dd8448eb211c80319c \
      --parent-span-id b7ad6b7169203331`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runSend(cmd, flags)
		},
	}

	cmd.Flags().StringVar(&flags.OtlpUrl, "otlp-url", "", "OTLP endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().StringVar(&flags.Dataset, "dataset", "", "Dataset name")
	cmd.Flags().StringVar(&flags.Name, "name", "", "Span name (required)")
	cmd.MarkFlagRequired("name")
	cmd.Flags().StringVar(&flags.Kind, "kind", "INTERNAL", "Span kind: INTERNAL, SERVER, CLIENT, PRODUCER, CONSUMER")
	cmd.Flags().StringVar(&flags.StatusCode, "status-code", "UNSET", "Status code: UNSET, OK, ERROR")
	cmd.Flags().StringVar(&flags.StatusMessage, "status-message", "", "Status message (typically for ERROR status)")
	cmd.Flags().StringVar(&flags.StartTime, "start-time", "", "Start timestamp in RFC3339 format; defaults to now")
	cmd.Flags().StringVar(&flags.EndTime, "end-time", "", "End timestamp in RFC3339 format; mutually exclusive with --duration")
	cmd.Flags().StringVar(&flags.Duration, "duration", "", "Span duration (e.g. '100ms', '1.5s'); mutually exclusive with --end-time")
	cmd.Flags().StringVar(&flags.TraceID, "trace-id", "", "Trace ID (32 hex characters); auto-generated if omitted")
	cmd.Flags().StringVar(&flags.SpanID, "span-id", "", "Span ID (16 hex characters); auto-generated if omitted")
	cmd.Flags().StringVar(&flags.ParentSpanID, "parent-span-id", "", "Parent span ID (16 hex characters)")
	cmd.Flags().StringArrayVar(&flags.ResourceAttributes, "resource-attribute", nil, "Resource attribute as 'key=value' (repeatable)")
	cmd.Flags().StringArrayVar(&flags.SpanAttributes, "span-attribute", nil, "Span attribute as 'key=value' (repeatable)")
	cmd.Flags().StringArrayVar(&flags.SpanLinks, "span-link", nil, "Span link as 'trace-id:span-id[,key=value,...]' (repeatable)")
	cmd.Flags().StringVar(&flags.ScopeName, "scope-name", otlp.DefaultScopeName, "Instrumentation scope name; defaults to 'dash0-cli'")
	cmd.Flags().StringVar(&flags.ScopeVersion, "scope-version", version.Version, "Instrumentation scope version; defaults to the dash0 CLI version")
	cmd.Flags().StringArrayVar(&flags.ScopeAttributes, "scope-attribute", nil, "Instrumentation scope attribute as 'key=value' (repeatable)")

	return cmd
}

func runSend(cmd *cobra.Command, flags *sendFlags) error {
	ctx := cmd.Context()

	otlp.ResolveScopeDefaults(cmd, &flags.ScopeName, &flags.ScopeVersion)

	if flags.EndTime != "" && flags.Duration != "" {
		return fmt.Errorf("--end-time and --duration are mutually exclusive")
	}

	kind, err := ParseSpanKind(flags.Kind)
	if err != nil {
		return err
	}

	statusCode, err := ParseSpanStatusCode(flags.StatusCode)
	if err != nil {
		return err
	}

	resourceAttrs, err := otlp.ParseKeyValuePairs(flags.ResourceAttributes)
	if err != nil {
		return fmt.Errorf("invalid resource attribute: %w", err)
	}

	spanAttrs, err := otlp.ParseKeyValuePairs(flags.SpanAttributes)
	if err != nil {
		return fmt.Errorf("invalid span attribute: %w", err)
	}

	scopeAttrs, err := otlp.ParseKeyValuePairs(flags.ScopeAttributes)
	if err != nil {
		return fmt.Errorf("invalid scope attribute: %w", err)
	}

	now := time.Now()

	startTime := now
	if flags.StartTime != "" {
		startTime, err = time.Parse(time.RFC3339Nano, flags.StartTime)
		if err != nil {
			return fmt.Errorf("invalid start-time format (expected RFC3339): %w", err)
		}
	}

	endTime := startTime
	if flags.EndTime != "" {
		endTime, err = time.Parse(time.RFC3339Nano, flags.EndTime)
		if err != nil {
			return fmt.Errorf("invalid end-time format (expected RFC3339): %w", err)
		}
	} else if flags.Duration != "" {
		d, err := ParseDuration(flags.Duration)
		if err != nil {
			return err
		}
		endTime = startTime.Add(d)
	}

	// Generate or parse trace ID
	var traceID pcommon.TraceID
	traceIDHex := flags.TraceID
	if traceIDHex == "" {
		traceID = generateTraceID()
		traceIDHex = hex.EncodeToString(traceID[:])
	} else {
		traceID, err = otlp.ParseTraceID(traceIDHex)
		if err != nil {
			return err
		}
	}

	// Generate or parse span ID
	var spanID pcommon.SpanID
	spanIDHex := flags.SpanID
	if spanIDHex == "" {
		spanID = generateSpanID()
		spanIDHex = hex.EncodeToString(spanID[:])
	} else {
		spanID, err = otlp.ParseSpanID(spanIDHex)
		if err != nil {
			return err
		}
	}

	// Parse span links
	links, err := parseSpanLinks(flags.SpanLinks)
	if err != nil {
		return err
	}

	// Build ptrace.Traces
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()

	resource := rs.Resource()
	for k, v := range resourceAttrs {
		resource.Attributes().PutStr(k, v)
	}

	ss := rs.ScopeSpans().AppendEmpty()
	scope := ss.Scope()
	scope.SetName(flags.ScopeName)
	scope.SetVersion(flags.ScopeVersion)
	for k, v := range scopeAttrs {
		scope.Attributes().PutStr(k, v)
	}

	s := ss.Spans().AppendEmpty()
	s.SetName(flags.Name)
	s.SetKind(ptrace.SpanKind(kind))
	s.Status().SetCode(ptrace.StatusCode(statusCode))
	if flags.StatusMessage != "" {
		s.Status().SetMessage(flags.StatusMessage)
	}
	s.SetStartTimestamp(pcommon.NewTimestampFromTime(startTime))
	s.SetEndTimestamp(pcommon.NewTimestampFromTime(endTime))
	s.SetTraceID(traceID)
	s.SetSpanID(spanID)

	if flags.ParentSpanID != "" {
		parentSpanID, err := otlp.ParseSpanID(flags.ParentSpanID)
		if err != nil {
			return fmt.Errorf("invalid parent-span-id: %w", err)
		}
		s.SetParentSpanID(parentSpanID)
	}

	for k, v := range spanAttrs {
		s.Attributes().PutStr(k, v)
	}

	for _, link := range links {
		l := s.Links().AppendEmpty()
		l.SetTraceID(link.traceID)
		l.SetSpanID(link.spanID)
		for k, v := range link.attributes {
			l.Attributes().PutStr(k, v)
		}
	}

	// Create OTLP client and send
	apiClient, err := client.NewOtlpClientFromContext(ctx, flags.OtlpUrl, flags.AuthToken)
	if err != nil {
		return err
	}
	defer apiClient.Close(ctx)

	if err := apiClient.SendTraces(ctx, traces, client.ResolveDataset(ctx, flags.Dataset)); err != nil {
		return fmt.Errorf("failed to send span: %w", err)
	}

	fmt.Printf("Span sent successfully (trace-id: %s, span-id: %s)\n", traceIDHex, spanIDHex)
	return nil
}

type parsedSpanLink struct {
	traceID    pcommon.TraceID
	spanID     pcommon.SpanID
	attributes map[string]string
}

// parseSpanLinks parses span link flags in the format "trace-id:span-id[,key=value,...]".
func parseSpanLinks(links []string) ([]parsedSpanLink, error) {
	var result []parsedSpanLink
	for _, link := range links {
		parsed, err := parseSpanLink(link)
		if err != nil {
			return nil, fmt.Errorf("invalid span-link %q: %w", link, err)
		}
		result = append(result, parsed)
	}
	return result, nil
}

func parseSpanLink(s string) (parsedSpanLink, error) {
	// Split on first comma to separate IDs from attributes
	idPart := s
	var attrPart string
	if idx := strings.Index(s, ","); idx != -1 {
		idPart = s[:idx]
		attrPart = s[idx+1:]
	}

	// Split trace-id:span-id
	parts := strings.SplitN(idPart, ":", 2)
	if len(parts) != 2 {
		return parsedSpanLink{}, fmt.Errorf("expected format 'trace-id:span-id[,key=value,...]'")
	}

	traceID, err := otlp.ParseTraceID(parts[0])
	if err != nil {
		return parsedSpanLink{}, err
	}

	spanID, err := otlp.ParseSpanID(parts[1])
	if err != nil {
		return parsedSpanLink{}, err
	}

	var attrs map[string]string
	if attrPart != "" {
		kvPairs := strings.Split(attrPart, ",")
		attrs, err = otlp.ParseKeyValuePairs(kvPairs)
		if err != nil {
			return parsedSpanLink{}, fmt.Errorf("invalid link attributes: %w", err)
		}
	}

	return parsedSpanLink{
		traceID:    traceID,
		spanID:     spanID,
		attributes: attrs,
	}, nil
}

func generateTraceID() pcommon.TraceID {
	var tid pcommon.TraceID
	rand.Read(tid[:])
	return tid
}

func generateSpanID() pcommon.SpanID {
	var sid pcommon.SpanID
	rand.Read(sid[:])
	return sid
}
