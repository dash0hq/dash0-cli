---
date: 2026-06-11
topic: otlp-proxy-local-dev
---

# Local-dev OTLP proxy — `dash0 otlp proxy`

> **Companion brief:** `dash0 logs send --tail` and `dash0 spans send --tail` were scoped alongside this command and live at [`docs/brainstorms/2026-06-11-send-tail-local-dev-requirements.md`](2026-06-11-send-tail-local-dev-requirements.md).
> The two briefs share several decisions (`-X` gating, not a Collector replacement, fail-loud, OTLP/JSON agent-mode events, no `dash0 dev` umbrella); each is duplicated where load-bearing so this brief stands alone for planning.

## Summary

A new long-running CLI command, gated `-X` initially, that lets a developer get OpenTelemetry signals into Dash0 from a local machine without standing up an OpenTelemetry Collector.
`dash0 otlp proxy` listens on both standard OTLP ports (4318 for HTTP and 4317 for gRPC) and forwards inbound OTLP traffic to Dash0 using the active profile's credentials.
gRPC payloads are transcoded to OTLP/HTTP/JSON outbound, since `dash0-api-client-go` speaks HTTP/JSON.

## Problem Frame

A developer who wants to see their locally-running application's telemetry in Dash0 has two options today.
They can run an OpenTelemetry Collector on their laptop with a bearer-token YAML config — roughly 25 lines across receivers, exporters, extensions, and pipelines — or they can skip Dash0 in local dev and use a different debugging path.
The OTel 2026 community survey reports 63 percent of Collector users cite configuration management as their top pain point, and no major observability vendor ships a "proxy" subcommand in their CLI today: Datadog requires its full agent, Honeycomb and Grafana require Collector configs.

The dash0-cli explicitly does not aim to replace the OTel Collector (`STRATEGY.md`, "Not working on").
The proxy is a dev-shortcut, not a production forwarder, and lives in the "Non-IaC workflows" track that aims to lower the bar for OpenTelemetry adoption through Dash0.

## Key Decisions

**KD1. The command ships behind `-X` initially.**
Long-running daemons are novel command shapes for the CLI — every existing command terminates after one operation.
Gating behind the experimental flag follows the precedent set by the `api` command (`internal/rawapi/`) and the project's adding-commands guide.
Promotion to stable follows `docs/promoting-commands-to-stable.md` once the shape stabilizes.

**KD2. The proxy is a local-dev shortcut, not a Collector replacement.**
`STRATEGY.md` is clear: replacing the OTel Collector is not a goal.
This decision shapes every behavioral choice downstream — no durable buffering, no non-OTLP protocols (Zipkin, Jaeger, statsd), no pipeline processors, no sampling configuration.
The proxy speaks OTLP and only OTLP, in both wire forms the spec defines (HTTP and gRPC), because both are how OTel SDKs talk by default; that is what "OTLP-only" means at the wire level, not "HTTP-only."
Production reliability is the Collector's job; this command is for the inner dev loop.

**KD3. Async-forward per OTLP spec; backpressure as HTTP 503 + Retry-After.**
The proxy follows the OTLP spec's two-hop reliability model: HTTP 200 / gRPC OK means "accepted at this node," not "delivered to Dash0" ([OTLP spec §5](https://opentelemetry.io/docs/specs/otlp/) — "this protocol is concerned with the reliability of delivery between one pair of client/server nodes ... acknowledgements ... do not span intermediary nodes").
The listener returns 200 after pdata decode + queueing; worker goroutines perform the outbound forward asynchronously.
This matches the OTel Collector's `otlpreceiver` → `batchprocessor` boundary exactly.

When the per-signal queue saturates, the listener returns HTTP 503 / gRPC `UNAVAILABLE` with `Retry-After` so the SDK's mandated exponential backoff (OTLP spec §4.4) applies — backpressure is explicit and SDK-visible.
Upstream failures (Dash0 returns 5xx) are observed in stats + agent-mode error events, not surfaced to the SDK, because the SDK already received 200 on its hop.
The proxy does not buffer to disk.
This is strictly more reliable than the OTel Collector's default exporter-queue (which drops on overflow); a local-dev shortcut needs nothing more.

**KD4. The proxy is a credential broker, not a transparent forwarder.**
Inbound OTLP from localhost carries no `Authorization` header — that is the de-facto local-dev expectation, since the dev should not put bearer tokens into their app's environment.
The proxy strips any inbound `Authorization` and injects the bearer from the active profile on the outbound hop.
This follows the precedent set by `internal/rawapi/`, where the CLI is authoritative for outbound auth.

**KD5. Proxy the profile.**
One running instance proxies exactly one profile to exactly one dataset.
Dataset resolves from the active profile (overridable via the standard `--dataset` flag and `DASH0_DATASET` env var, same precedence as every other CLI command).
Per-request dataset routing — via `X-Dash0-Dataset` header, `service.namespace` attribute, or any other mechanism — is not a thing the proxy will do, ever.
Developers who need two datasets run two proxies; the CLI's profile mechanism is the routing primitive, not request metadata.

**KD6. Default to the standard OTLP ports so default-config SDKs just work.**
The proxy binds two listeners by default: 4318 for OTLP/HTTP and 4317 for OTLP/gRPC.
An app configured with the OTel SDK's default endpoint connects with no env-var change regardless of which OTLP protocol the SDK picked.
Each listener falls back to an OS-assigned port independently when its default port is in use (printing a clear notice naming the actual port chosen), or when the user explicitly overrides via the corresponding flag or env var.
On explicit override the user owns the choice: if the override value collides, the command fails for that listener rather than silently rebinding.
A failure on one listener does not block the other from starting; both are reported in the start banner.

**KD7. Make telemetry visible — kill the "is it flowing or lost?" FUD.**
The biggest source of friction in local OTel work is the silence between "my app emitted a span" and "I can find it in the backend."
The proxy is uniquely positioned to answer this because it sits in the middle of the stream.
Three layers, all on by default:

1. **Per-signal live stats.**
   The proxy tracks two numbers per signal — rate (records per second over the last interval) and running total (records forwarded since the proxy started).
   Three signals: logs, spans (traces), metrics.
   Profiles are explicitly skipped — the OTLP Profiles signal is not stable and adding it now would lock the proxy to a moving target.
   In TTY mode the stats live-update in place on stderr (single line, redrawn each interval), shaped like `spans: 5/s · 1234 total · logs: 12/s · 8431 total · metrics: 0/s · 0 total`.
   When the terminal is wide enough and a TTY is attached, the line is preceded by a compact per-signal **timeline sparkline** — the rate for each of the last N intervals rendered as Unicode block characters (`▁▂▃▄▅▆▇█`), oldest interval on the left, newest on the right.
   The shape makes "is it flowing? when did it stop?" land at a glance: `spans ▁▂▅▇▆▃▁ 5/s · logs ▃▅▇█▇▆▅ 12/s · metrics ▁▁▁▁▁▁▁ 0/s`.
   Sparklines are degraded to text-only when stderr is not a TTY, when the terminal lacks Unicode support, or when the terminal width can't accommodate them; the text counts remain.
   Feasibility: well-trodden — Go has both first-party Unicode-block approaches (~50 lines) and small libraries (e.g., `asciigraph`); the cost is implementation polish, not unknowns.
   In agent mode (`DASH0_AGENT_MODE=true`) the stats append to stdout, one NDJSON event per interval, in the OTLP/JSON shape defined by KD8 — no sparklines, because the consumer is a structured-data parser, not a human terminal.
2. **Lifecycle and error events.**
   Start banner, listener fallback notices, outbound failures, and shutdown print one-shot lines to stderr in TTY mode (above the live-updating stats area).
   In agent mode they append as OTLP/JSON log records (KD8).
3. **`--tail` prints the actual telemetry.**
   When enabled, the proxy prints each forwarded record in the style of the OTel Collector's debug exporter — resource attributes, scope, then per-record details (trace and span IDs, span name, status, log severity and body, metric data points).
   Answers "what is my app actually sending?" without needing a separate sink or a round-trip through the Dash0 UI.

   **Stream routing is uniform across this command's modes:** statistics always go to stderr (so they stay out of the way of pipelines), and `--tail` output always goes to stdout (so it can be piped, redirected, or grep'd).
   This mirrors how `dash0 logs send --tail` routes its tee output, and it matches the shared meaning of `--tail` across both commands: "make in-flight telemetry visible to me."
   The proxy implementation naturally differs from `logs send --tail` — the proxy is a boolean opt-in (no input source argument), while `logs send --tail` reads lines from one — but a user familiar with one finds the other immediately.
   In agent mode (`DASH0_AGENT_MODE=true`), `--tail` without `--tail-stderr` is a **hard error at startup** with an actionable message ("`--tail` output conflicts with agent-mode stdout; add `--tail-stderr` to route to stderr"). The hard-error choice matches how the CLI surfaces other flag conflicts (e.g., `--column` with `-o json`) and tells a deliberate user that their `--tail` request would otherwise be invisible.
   Defaults to off because high-volume sessions become unreadable; explicit opt-in is the right shape.

No TUI — `STRATEGY.md` non-goal.

**KD8. Agent-mode events are OTLP/JSON, not a bespoke schema.**
In `DASH0_AGENT_MODE`, the proxy appends NDJSON to stdout where each line is a valid OTLP `ResourceLogs` record describing one event in the proxy's own lifecycle.
Common attributes: `service.name = "dash0-cli"`, `service.instance.id = "<process-uuid>"`, `event.name = "dash0.cli.otlp_proxy.<event>"` (`started`, `forwarded`, `error`, `shutdown`), plus event-specific attributes (`endpoint`, `port`, `dataset`, `signal`, `count`, `bytes`, `reason`, `code`).
Two consequences: the agent reading proxy output uses the same OTLP parsing it already has for ingested data, and the events are self-describing without a separate schema doc.
Per-interval stats updates are emitted as `event.name = "dash0.cli.otlp_proxy.stats"` log records.
v1 does not forward these self-events to Dash0; they exist on stdout for the agent in the same process tree.

## Actors

A1. **Developer (primary).**
SRE or developer running Dash0 as their observability platform, working locally on an instrumented application.
Hires the proxy to send OTLP from their app to Dash0 with no Collector setup.

A2. **OTLP-generating process.**
Any process emitting OTLP that the developer wants forwarded to Dash0.
Most commonly an OTel-instrumented application, but the proxy treats the source generically — an upstream OTel Collector daisy-chained into the proxy as its `otlp` exporter is equally valid, as is `dash0 logs send`, `dash0 spans send`, or anything else that speaks OTLP/HTTP or OTLP/gRPC.
Talks to the proxy on `localhost:4318` (HTTP) or `localhost:4317` (gRPC), or the fallback port reported in the start banner.

A3. **Dash0 backend.**
Receives OTLP/HTTP via the existing `dash0-api-client-go`.
Technically the same outbound code path would work against any OTLP/HTTP-accepting backend, but supporting non-Dash0 backends is not a goal — the proxy is part of `dash0-cli` and the active profile's `otlp-url` is what it talks to.
Treated as a fixed external dependency.

A4. **AI coding agent (secondary).**
Consumes the NDJSON event stream when invoked under `DASH0_AGENT_MODE`.
Humans-first design rule applies as elsewhere in the CLI.

## Requirements

### General behavior

R1. The command is gated behind `-X` (`--experimental`) initially, following `docs/promoting-commands-to-stable.md` for later promotion.

R2. The command resolves `otlp-url`, `auth-token`, and `dataset` from the standard env > flag > profile precedence.

R3. The command surfaces per-signal rate and running total counts continuously while running, per KD7. In TTY mode the counters live-update in place on **stderr** (with Unicode timeline sparkline when stderr is a TTY); in agent mode the counters append to **stdout** as OTLP/JSON `dash0.cli.otlp_proxy.stats` events per KD8. Stream routing is fixed: statistics always on stderr in TTY mode, `--tail` output always on stdout in TTY mode (R6a) — see KD7.

R4. The command exits cleanly on `SIGINT` and `SIGTERM`, draining in-flight requests with a bounded timeout of approximately 5 seconds before terminating, and emits a final stats line plus a shutdown event.

R5. The command prints the active profile and resolved dataset on start so the developer can confirm before traffic flows.

R6. The proxy implements OTLP's two-hop reliability model (KD3): listener returns 200 to the SDK after pdata decode + queueing, workers forward asynchronously. Upstream failures (Dash0 returns 5xx) surface as a stderr line in TTY mode or a `dash0.cli.otlp_proxy.error` event in agent mode — not as an SDK-visible status code, because the SDK already received its 200. Queue saturation surfaces to the SDK as HTTP 503 / gRPC `UNAVAILABLE` with `Retry-After`, triggering the SDK's mandated exponential backoff. The command does not buffer to disk.

R6a. A `--tail` flag enables per-record printing of forwarded telemetry in the style of the OTel Collector's debug exporter (resource attributes, scope, per-record details for spans / logs / metrics). Off by default; explicit opt-in (KD7). Per-record output goes to stdout; statistics remain on stderr (R3). The flag name matches `dash0 logs send --tail` for cross-command consistency. In `DASH0_AGENT_MODE`, `--tail` without `--tail-stderr` is a hard error at startup; pass `--tail-stderr` to route `--tail` output to stderr while keeping stdout parseable as the NDJSON event stream.

### Listener behavior

R7. The proxy binds two listeners by default: OTLP/HTTP on port 4318 and OTLP/gRPC on port 4317, matching the OTel SDK default endpoints so an SDK at default configuration connects without any env-var change regardless of which OTLP wire form it uses. If a default port is already in use, that listener falls back to an OS-assigned port, prints a clear notice naming the actual port chosen, and proceeds; the other listener is unaffected. `--http-port <n>`, `--grpc-port <n>`, `DASH0_OTLP_PROXY_HTTP_PORT`, and `DASH0_OTLP_PROXY_GRPC_PORT` override the defaults; on explicit override the value is honored, and the affected listener fails if it collides. Setting either override to `0` requests an OS-assigned port for that listener.

R8. The HTTP listener exposes the three OTLP/HTTP spec paths: `POST /v1/logs`, `POST /v1/traces`, `POST /v1/metrics`. Other paths return 404. The gRPC listener exposes the three OTLP services: `opentelemetry.proto.collector.logs.v1.LogsService`, `opentelemetry.proto.collector.trace.v1.TraceService`, `opentelemetry.proto.collector.metrics.v1.MetricsService`. Other services return gRPC `UNIMPLEMENTED`.

R9. The HTTP listener accepts `application/x-protobuf` and `application/json` content types and returns 415 for others. The gRPC listener accepts the standard OTLP/gRPC framing only.

R10. Inbound payloads from either listener are normalized to in-memory pdata (`go.opentelemetry.io/collector/pdata`), then sent through `dash0-api-client-go` as OTLP/HTTP/JSON. Inbound `Authorization` headers and gRPC `authorization` metadata are stripped; the proxy injects the active profile's bearer on the outbound hop. Routing-style metadata (e.g., `X-Dash0-Dataset` header, `dash0-dataset` gRPC metadata) is ignored — the proxy routes by profile per KD5.

R11. On start, the proxy prints one line per active listener (e.g., `OTLP/HTTP listening on http://127.0.0.1:4318`, `OTLP/gRPC listening on 127.0.0.1:4317`). When a default port is in use and the fallback fires, the printed endpoint reflects the OS-assigned port plus a one-line notice naming the relevant env-var override (e.g., `OTEL_EXPORTER_OTLP_ENDPOINT` for the HTTP fallback, `OTEL_EXPORTER_OTLP_ENDPOINT` plus the protocol hint for gRPC). When both defaults bind cleanly, no override hint is printed — the SDK's defaults already point at the proxy.

R12. Both OTLP wire forms are first-class in v1. Inbound OTLP/gRPC payloads are decoded with the standard OTel collector protobuf definitions and routed through the same outbound code path as HTTP inbound. There is no v1 case where an SDK at OTLP defaults gets a connection refused.

## Key Flows

### F1. Send spans from a locally-running instrumented application

**Trigger:** developer wants to see their app's spans in Dash0 while iterating on code.

Developer runs `dash0 -X otlp proxy` in one terminal.
The proxy binds both default ports, prints `OTLP/HTTP listening on http://127.0.0.1:4318` and `OTLP/gRPC listening on 127.0.0.1:4317`, and prints the active profile plus the dataset it will route to.
Developer starts the app in another terminal with no env-var changes — SDK defaults already point at the right port regardless of whether the SDK chose HTTP or gRPC.
Spans flow: app → proxy (strips inbound auth, decodes / normalizes via pdata if inbound was gRPC, injects bearer from profile) → Dash0 (active profile's dataset).
The per-signal live-stats line on the proxy's stderr updates as spans forward; the developer sees them in Dash0 within seconds.
Adding `--tail` to the proxy command prints each forwarded record to stdout in collector-debug-exporter style — answering "what is my app actually sending?" without leaving the terminal, and pipeable to `grep` / `jq` if the developer wants to filter.
Ctrl-C drains in-flight requests and exits.

**Covers R1-R12, R6a.**

### F2. Agent-driven monitoring of a long-running proxy

**Trigger:** an AI coding agent has started the proxy in the background to verify a customer's instrumentation is reachable.

Agent runs `dash0 --agent-mode -X otlp proxy` and parses NDJSON-OTLP/JSON from stdout.
First event is a log record with `event.name = "dash0.cli.otlp_proxy.started"` carrying `endpoint`, `port`, `dataset`.
`dash0.cli.otlp_proxy.stats` events tick once per interval with per-signal counts and bytes.
If the outbound hop fails, a `dash0.cli.otlp_proxy.error` event surfaces the reason.
On SIGTERM the agent receives a `dash0.cli.otlp_proxy.shutdown` event before the process exits.
Because every line is a valid OTLP `ResourceLogs` record, the agent reuses the same OTLP parsing it already has for ingested data.

**Covers R3, R4.**

## Acceptance Examples

AE1. **Inbound auth is stripped (R10).**
A request to `POST /v1/logs` carrying `Authorization: Bearer some-leaked-token` is forwarded to Dash0 with `Authorization: Bearer <active-profile-bearer>`, regardless of what the inbound value was.
The inbound value never appears in any log line or outbound request.

AE2. **Fail-loud on Dash0 outage (KD3, R6).**
With Dash0's OTLP endpoint returning HTTP 503, the proxy responds to inbound `POST /v1/traces` with HTTP 502 carrying a body like `{"error":"upstream Dash0 OTLP endpoint returned 503", ...}`, prints a single stderr line, and does not retry beyond the standard `--max-retries` budget.

AE3. **Default port collision falls back cleanly (R7).**
With port 4318 already bound by another process, `dash0 -X otlp proxy` starts on an OS-assigned port (e.g., 41723), prints `port 4318 unavailable — fell back to 41723; export OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:41723`, and proceeds.
With `--port 9999` explicitly set and 9999 also in use, the command exits non-zero with a clear error rather than falling back silently.

AE4. **Default port works without env-var setup (R7, KD6).**
With nothing else bound to 4318, `dash0 -X otlp proxy` starts on 4318, prints `OTLP/HTTP listening on http://127.0.0.1:4318`, and an app started in the same shell with no `OTEL_EXPORTER_OTLP_ENDPOINT` set sends spans successfully — the SDK's default endpoint already points at the proxy.

AE5. **gRPC default-port works without env-var setup (R7, R12, KD6).**
With nothing else bound to 4317, an app configured to use OTLP/gRPC at default `localhost:4317` connects successfully.
The proxy decodes inbound gRPC, normalizes to pdata, and forwards as OTLP/HTTP/JSON to Dash0.
The proxy's start banner reports both listeners: `OTLP/HTTP listening on http://127.0.0.1:4318` and `OTLP/gRPC listening on 127.0.0.1:4317`.

AE6. **One listener falls back independently of the other (R7).**
With port 4317 already bound by another process but 4318 free, `dash0 -X otlp proxy` starts the HTTP listener cleanly on 4318 and falls back to an OS-assigned port for gRPC, naming both endpoints in the banner.
With `--grpc-port 9999` explicitly set and 9999 also in use, the gRPC listener fails and exits non-zero for gRPC while the HTTP listener still starts on 4318.

- A developer with an instrumented app and an active Dash0 profile sees spans in Dash0 within 60 seconds of `dash0 -X otlp proxy` starting, with no env-var or config-file change when ports 4318 and 4317 are free — regardless of whether the SDK chose OTLP/HTTP or OTLP/gRPC by default.
- A developer asking "is my app actually sending telemetry?" can answer it without leaving the terminal by reading the live stats line, and can answer "what exactly is my app sending?" by adding `--debug`.
- Agent mode emits well-formed NDJSON-OTLP/JSON for the full lifecycle of the command, parsable line-by-line by the same OTLP decoder the agent already uses for ingested data.
- The command exits cleanly within 5 seconds of SIGINT/SIGTERM, with no orphaned listeners on either port.
- Roundtrip integration tests under `test/roundtrip/` cover send-and-query for spans, logs, and metrics through both the HTTP listener and the gRPC listener.

## Scope Boundaries

### In scope (v1)

- `dash0 otlp proxy` listening on both 4318 (HTTP) and 4317 (gRPC) with the independent fallback policy in R7.
- gRPC ingest transcoded to OTLP/HTTP/JSON outbound via pdata.
- `-X` gating plus the agent-mode NDJSON schema in KD8.
- Per-signal live counters (rate + total for logs / spans / metrics) with Unicode sparklines when stderr is a TTY, per KD7. NDJSON stats events in agent mode.
- Optional `--debug` mode that prints inbound records in collector-debug-exporter style.
- Integration tests plus a roundtrip test covering both wire forms.
- Documentation in `docs/commands.md`, `README.md`, plus a changelog entry per `docs/changelog-maintenance.md`.

### Deferred for later

- Durable disk-backed buffering on outage (revisit only if customer pain emerges; a small in-memory ring buffer is the cheaper upgrade if needed).
- `dash0 exec -- <cmd>` subprocess wrapper.
- OAuth-on-first-run for users without an active profile (composes with `feat/oauth-login` separately).

### Outside this product's identity

- Replacing the OpenTelemetry Collector (`STRATEGY.md` non-goal).
- TUI or local-inspection UI (`STRATEGY.md` non-goal).
- Plugin system for receivers, exporters, or processors (`STRATEGY.md` non-goal).
- Non-OTLP wire formats (Zipkin, Jaeger, statsd) — Collector territory.
- Per-request dataset routing (multi-tenant proxy). The proxy's identity is one running instance proxies one profile — see KD5.
- Pointing the proxy at non-Dash0 OTLP backends. The outbound code path technically could, but adding a `--upstream-url` flag or similar would invite supporting backends whose semantics, auth model, and behavior `dash0-cli` cannot speak for. The active profile's `otlp-url` is the only outbound target.
- A `dash0 dev` umbrella for local-dev commands. The local-dev surface stays at natural per-signal locations (`dash0 otlp proxy`, `dash0 logs tail`, etc.).

## Dependencies / Assumptions

- The user has an active Dash0 profile with a valid `otlp-url` and `auth-token`. v1 does not invoke OAuth-on-first-run; that path composes with the in-flight `feat/oauth-login` work separately.
- `dash0-api-client-go` continues to support `SendLogs` / `SendTraces` / `SendMetrics` over HTTP/JSON. gRPC outbound is not assumed; inbound gRPC is transcoded to HTTP/JSON.
- `go.opentelemetry.io/collector/pdata` (already in use, see `internal/otlp/`) is the canonical in-memory representation for normalizing between inbound formats and outbound HTTP/JSON.
- `google.golang.org/grpc` plus the OTLP service protobuf definitions (`go.opentelemetry.io/proto/otlp`) are new direct dependencies for the gRPC listener. Both have permissive licenses compatible with the project's Apache 2.0.
- Cobra and the existing flag, config, and agent-mode plumbing carry through unchanged.
- The CLI's existing `--max-retries` plumbing (default 3, max 5) applies to each outbound request.
- **Package layout:** the proxy extends `internal/otlp/` (reusing the existing shared OTLP utilities — `ParseKeyValuePairs`, `ResolveScopeDefaults`, trace/span ID parsing). It does not introduce a new top-level `internal/otlpproxy/` package, and it does not motivate a shared `internal/ingest/` abstraction with the companion `logs send --tail` (each command lives in its own existing domain package).

## Outstanding Questions

### Resolve before planning

OQ1. **Live-stats update interval.**
The stats line updates every N seconds in TTY mode and emits an NDJSON stats event every N seconds in agent mode. 1 second is the obvious starting point but burns a redraw budget on idle proxies; 5 seconds smooths the visible rate but feels laggy when an app first connects. A non-fixed cadence (faster early, slower at steady state) is a third option. Worth a call before implementation.

OQ2. **`--tail` detail level.**
KD7 says `--tail` prints "in the style of the OTel Collector's debug exporter." The Collector exporter has a `verbosity` setting (`basic` / `normal` / `detailed`) — basic prints record counts only, normal prints names and key attributes, detailed prints everything. Pick one default for v1 (likely `detailed` — that's the FUD-killer) and decide whether `--tail-detail=<level>` (or repeating the flag, e.g. `--tail --tail`) is a v1 flag or a later add. Should match whatever `logs send --tail` lands on.

### Deferred to planning

OQ3. Internal organization within `internal/otlp/` — how the listener, transcoder, and forwarder are split into files, what test seams exist, how the dual-listener lifecycle is coordinated. The package home is settled (see Dependencies); the internal shape is a planning call.

## Sources / Research

- `docs/ideation/2026-06-11-otlp-proxy-local-dev-ideation.html` — the ideation artifact that surfaced this command (idea I2) along with `logs send --tail` + `spans send --tail` (idea I3, originally framed as `logs tail`).
- `docs/brainstorms/2026-06-11-send-tail-local-dev-requirements.md` — companion brief for `logs send --tail` and `spans send --tail`, scoped alongside this one.
- `STRATEGY.md` — "Non-IaC workflows" track and "Not working on: Replacing the OpenTelemetry Collector" constraint.
- `internal/rawapi/` — precedent for `-X` experimental commands and CLI-as-authoritative-for-outbound-auth.
- `internal/logging/send.go`, `internal/tracing/spans_send.go`, `internal/otlp/` — existing OTLP-related code to reuse and extend.
- `docs/adding-commands.md` — the project's adding-commands checklist.
- `docs/promoting-commands-to-stable.md` — the path from `-X` to stable.
- `kubectl proxy`, `mkcert`, `gcloud auth application-default login` — cross-domain analogies for the proxy's UX.
- [OTel Collector debug exporter](https://github.com/open-telemetry/opentelemetry-collector/tree/main/exporter/debugexporter) — the visual format for `--debug` per-record printing.
- [`go.opentelemetry.io/proto/otlp`](https://github.com/open-telemetry/opentelemetry-proto-go) — OTLP service protobuf definitions for the gRPC listener.
- [OTel Collector pdata](https://github.com/open-telemetry/opentelemetry-collector/tree/main/pdata) — in-memory normalization across HTTP and gRPC inbound formats.
