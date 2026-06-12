package otlp

import "github.com/spf13/cobra"

// NewOtlpCmd creates the otlp parent command. The proxy subcommand exposes a
// local OTLP forwarder that brokers traffic from local OpenTelemetry SDKs to
// the active Dash0 profile.
func NewOtlpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "otlp",
		Short: "[experimental] Local-dev OTLP forwarding for Dash0",
		Long: `OTLP commands for local development against Dash0.

The proxy subcommand exposes the standard OTLP/HTTP and OTLP/gRPC endpoints
on the loopback interface, brokers credentials from the active Dash0
profile, and forwards inbound telemetry to Dash0. It is a local-dev
shortcut, not a replacement for the OpenTelemetry Collector.`,
	}

	cmd.AddCommand(newProxyCmd())

	return cmd
}
