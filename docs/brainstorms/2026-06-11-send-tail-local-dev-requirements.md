---
date: 2026-06-11
topic: send-tail-local-dev
---

# Local-dev signal streaming — `logs send --tail` and `spans send --tail`

> **Companion brief:** `dash0 otlp proxy` was scoped alongside these commands and lives at [`docs/brainstorms/2026-06-11-otlp-proxy-local-dev-requirements.md`](2026-06-11-otlp-proxy-local-dev-requirements.md).
> The briefs share several decisions (`-X` gating, not a Collector replacement, fail-loud, OTLP/JSON agent-mode events, no `dash0 dev` umbrella, `stats → stderr · --tail → stdout` stream routing); each is duplicated where load-bearing so this brief stands alone for planning.

## Summary

Two new `--tail` flags on the existing signal-sender commands, both gated `-X` initially, that turn the one-shot senders into streaming forwarders.
`dash0 logs send --tail` reads log lines from stdin or files and forwards each as an OTLP log.
`dash0 spans send --tail` watches files of newline-delimited JSON span records and forwards each as an OTLP span.
Both commands echo every forwarded input to stdout (`tee`-style) so the surrounding pipeline stays intact for `grep`, `jq`, or further `tee`-ing.
The existing one-shot forms (positional `<body>` for `logs send`, flag-driven for `spans send`) are unchanged — the `send-log-event` GitHub Action and all current call sites keep working.

## Problem Frame

A developer who wants to ship locally-running telemetry to Dash0 has two options today.
They can stand up an OpenTelemetry Collector (filelog-receiver for logs; no equivalent for shell-emitted spans), or they can skip Dash0 in local dev entirely.
The OTel filelog-receiver is the documented complexity outlier of the tail-and-forward category: its multiline configuration is described in [issue #35162](https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/35162) as "the most mysterious log parser," it provides no observability into which files are being harvested ([issue #31256](https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/31256)), and its default `start_at: end` surprises new users.
Fluent Bit, Vector, and Promtail have converged on a simpler UX (glob paths, SQLite checkpoints, explicit start-position control, inline multiline) — a convergence the OTel surface doesn't reflect.

For spans, the dev-loop gap is different.
CI pipelines, test runners, deployment scripts, and Makefile targets often want to emit one span per step (build, test, lint, release) but instrumenting each script with an OTel SDK is heavy.
Today the alternative is calling `dash0 spans send` once per span, which carries connection overhead per call.
Honeycomb's `buildevents` solves this for CI by writing structured events to disk and forwarding them; a `spans send --tail '*.jsonl'` shape lets any script emit a JSON span per line into a file and have them all forwarded with auto-batching.

The dash0-cli explicitly does not aim to replace the OTel Collector (`STRATEGY.md`, "Not working on").
These additions are dev-shortcuts for the most common local-signal shapes — "stream my app's stdout to Dash0" and "watch a directory of span events from my CI scripts" — and live in the "Non-IaC workflows" track that aims to lower the bar for OpenTelemetry adoption through Dash0.

## Key Decisions

**KD1. Adding `--tail` to existing `send` commands, not new subcommands.**
The streaming form is a different *mode* of the same operation — "put records into Dash0" — so a flag keeps the verb stable.
`logs send --tail` and `spans send --tail` carry parallel meaning.
Existing one-shot forms (`logs send "<body>"`, `spans send --name <name> ...`) are unchanged and the positional/flag-driven one-shot args are mutually exclusive with `--tail`.

**KD2. Both commands ship behind `-X` initially.**
Long-running daemons are novel command shapes for the CLI — every existing command terminates after one operation.
`--tail` is the introduction of that shape under both `send` verbs.
Gating follows `docs/promoting-commands-to-stable.md` for later promotion; the one-shot forms remain stable and ungated.

**KD3. Tee semantics: forward AND echo to stdout.**
Every input record forwarded to Dash0 also writes to stdout, unchanged, so the surrounding pipeline can fan out through the command without losing data.
For `logs send --tail`, "input record" is the raw line read from stdin or a file.
For `spans send --tail`, "input record" is the raw JSON line read from a file (each line one span).
`--quiet` / `-q` suppresses the tee echo when only forwarding is wanted.
In `DASH0_AGENT_MODE` the tee is auto-suppressed to keep stdout parseable as the NDJSON event stream (KD8); `--tee-stderr` reroutes the echo to stderr when both are wanted.

**KD4. Local-dev shortcut, not a Collector replacement.**
`STRATEGY.md` is clear: replacing the OTel Collector is not a goal.
This shapes downstream choices — no durable buffering, no routing rules, no pipeline processors beyond per-line format auto-detection (for logs), no multiple destinations.

**KD5. Fail-loud on Dash0 unreachable.**
When the outbound hop to Dash0 fails, both commands print a clear stderr line and continue reading.
Neither buffers to disk.
The tee echo to stdout continues unaffected — even when Dash0 is unreachable, the downstream pipeline sees the line.

**KD6. Per-signal input shape: logs is pipe-first with auto-detect; spans is file-only with structured JSON.**
`logs send --tail -` reads stdin; `logs send --tail '<glob>'` reads files; per line, auto-detection picks JSON / logfmt / plain.
`spans send --tail '<glob>'` reads files only; per line, the content must parse as a JSON object describing one span; **stdin is not supported for spans in v1**.
Two reasons for the asymmetry.
First, structured-JSON-per-line is awkward to pipe — the natural producer is a script or test runner writing into a file, not a pipe stage.
Second, span emission frequency is bursty and file-shaped (one JSON-line per step in a multi-step process is the canonical pattern), where pipe streaming offers no advantage.
A future stdin mode for spans is not architecturally precluded but is not v1 scope.

**KD7. Make telemetry visible — kill the "is it flowing or lost?" FUD.**
The biggest source of friction in local OTel work is the silence between "my app/script emitted a record" and "I can find it in the backend."
Three layers, on by default (the verbose mode is opt-in):

1. **Live stats.**
   Each command tracks two numbers — rate (records per second over the last interval) and running total (records forwarded since start).
   In TTY mode the stats live-update in place on stderr with a Unicode timeline sparkline: `logs ▃▅▇█▇▆▅ 12/s · 8431 total` (or `spans ▁▂▅▇▆ 5/s · 234 total`).
   When stderr is not a TTY, Unicode is unsupported, or the terminal is too narrow, the sparkline degrades to text-only counts.
   In agent mode the stats append to stdout as one NDJSON event per interval per KD8.
2. **Lifecycle and error events.**
   Start banner (active profile + dataset + input source), restart-from-checkpoint notices, outbound failures, shutdown — all print as one-shot stderr lines in TTY mode.
   In agent mode they append as OTLP/JSON log records per KD8.
3. **`--verbose` (`-v`) prints the parsed OTLP record details.**
   Beyond the raw tee echo on stdout (KD3), each forwarded record renders on stderr in the style of the OTel Collector's debug exporter — resource attributes, scope, then severity/body/attributes for logs or trace+span IDs/name/kind/status/attributes for spans.
   Stderr (not stdout) because the tee echo already owns stdout for the raw stream; mixing them would corrupt downstream pipes.
   The flag name matches the existing `api` command, where `--verbose` / `-v` already means "print wire-level details to stderr."

**Stream routing is uniform and explicit:** statistics go to **stderr** (always), the `--tail` view (raw-line tee echo) goes to **stdout** (always), `--verbose` adds a stderr-side layer.
The same `stats → stderr · --tail → stdout` rule holds for `dash0 otlp proxy --tail` in the companion brief.
No TUI — `STRATEGY.md` non-goal.

**KD8. Agent-mode events are OTLP/JSON, not a bespoke schema.**
In `DASH0_AGENT_MODE`, each command appends NDJSON to stdout where each line is a valid OTLP `ResourceLogs` record describing one event in the command's own lifecycle.
Common attributes: `service.name = "dash0-cli"`, `service.instance.id = "<process-uuid>"`, `event.name = "dash0.cli.<command>.<event>"` (`dash0.cli.logs_send_tail.started`, `dash0.cli.spans_send_tail.forwarded`, etc.), plus event-specific attributes (`source`, `dataset`, `count`, `bytes`, `reason`, `code`).
Per-interval stats updates emit as `dash0.cli.<command>.stats` records.
v1 does not forward these self-events to Dash0; they exist on stdout for the agent in the same process tree.
Shared shape with `dash0 otlp proxy` so the agent uses one OTLP decoder for everything.

**KD9. Span input shape: a structured JSON object per line.**
Each line in a file watched by `spans send --tail` parses as a JSON object whose fields mirror `spans send`'s existing flags — `name` (required), `kind`, `status_code`, `status_message`, `start_time`, `end_time` (or `duration`), `trace_id`, `span_id`, `parent_span_id`, `resource_attributes`, `span_attributes`, `span_links`.
Missing IDs are auto-generated (same as one-shot `spans send`).
Spans sharing a `trace_id` within a single forwarded batch are grouped into one OTLP `ResourceSpans` so the trace assembles correctly server-side.
OTLP-wire-format span JSON is not accepted in v1; if a process emits full OTLP, the proxy is the right tool, not this command.

## Actors

A1. **Developer (primary).**
SRE or developer running Dash0 as their observability platform, working locally on an application, CI script, or shell pipeline.
Hires `logs send --tail` to ship lines from a pipe or log files; hires `spans send --tail` to ship JSON-per-line span records from files produced by scripts or test runners.

A2. **Text-producing process or shell pipeline (logs).**
Any process emitting text-shaped log lines.
Writes to stdout (piped into `logs send --tail -`) or to a file (watched by `logs send --tail <glob>`).

A3. **JSON-emitting script or runner (spans).**
A CI step, test runner, deployment script, or any process that writes one JSON span object per line into a file.
Examples: a Makefile rule that appends `{"name":"build","status_code":"OK","duration_ms":3041}` to `~/.dash0/build-trace.jsonl` per step; a `pytest` plugin that writes one span per test.

A4. **Dash0 backend.**
Receives OTLP/HTTP logs and traces via the existing `dash0-api-client-go`.
Technically the same outbound code path would work against any OTLP/HTTP-accepting backend, but supporting non-Dash0 backends is not a goal — the active profile's `otlp-url` is the only outbound target.

A5. **AI coding agent (secondary).**
Consumes the NDJSON-OTLP/JSON event stream when invoked under `DASH0_AGENT_MODE`.
Humans-first design rule applies as elsewhere in the CLI.

## Requirements

### General behavior (both commands)

R1. The `--tail` flag is gated behind `-X` (`--experimental`) initially. The existing one-shot forms of both `send` commands are unaffected by the `-X` gate.

R2. Each command resolves `otlp-url`, `auth-token`, and `dataset` from the standard env > flag > profile precedence.

R3. Each command surfaces a per-second rate and running total continuously while running, per KD7. In TTY mode the counters live-update in place on **stderr** with a Unicode timeline sparkline; in agent mode they append to **stdout** as OTLP/JSON `dash0.cli.<command>.stats` events per KD8. Stream routing is fixed: statistics always on stderr (TTY mode), `--tail` raw-line echo always on stdout (TTY mode).

R4. Each command exits cleanly on `SIGINT` and `SIGTERM`, flushing in-flight requests and any open multiline buffer (logs only) with a bounded timeout of approximately 5 seconds before terminating, and emits a final stats line plus a shutdown event.

R5. Each command prints the active profile, resolved dataset, and input source(s) on start.

R6. When the outbound hop to Dash0 fails, each command prints a stderr line in TTY mode or an `error` event in agent mode and continues reading. Neither buffers to disk. The tee echo to stdout continues unaffected.

R7. A `--verbose` / `-v` flag enables per-record printing of forwarded telemetry in the style of the OTel Collector's debug exporter, on **stderr**. Off by default.

### Input modes — `logs send --tail`

R8. `logs send --tail -` reads from stdin line-by-line. `logs send --tail '<glob>'` (one or more glob arguments) watches matching files.

R9. Each line becomes one OTLP log record. Auto-detection per line: a line that parses as a JSON object lifts its fields to log attributes (with `body` unset); a logfmt line lifts fields to log attributes; otherwise the line is plain text and becomes `otel.log.body`.

R10. `--format json|logfmt|plain` overrides auto-detection.

### Input modes — `spans send --tail`

R11. `spans send --tail '<glob>'` (one or more glob arguments) watches matching files. **Stdin is not supported for spans in v1** (KD6) — `spans send --tail -` fails with a clear usage error pointing the user at either `dash0 otlp proxy` (for live span streams from an instrumented app) or a file-based source.

R12. Each line in a watched file is parsed as a JSON object describing one span, with field names mirroring `spans send`'s existing flags (KD9). Lines that fail to parse as JSON or that fail span-shape validation (e.g., missing required `name`) generate one stderr error line per failure and are skipped; the command continues reading.

R13. Spans sharing a `trace_id` within a single forwarded batch are grouped into one OTLP `ResourceSpans` request so the trace assembles correctly server-side.

### Tee semantics (both commands)

R14. By default, every input record read is echoed unchanged to stdout after being parsed and queued for forwarding. The echo order matches the input order and is not delayed by Dash0 flush timing — the downstream pipeline never blocks on the network.

R15. `--quiet` / `-q` suppresses the tee echo entirely. Each command still forwards to Dash0; it just doesn't pass through.

R16. In `DASH0_AGENT_MODE`, the tee echo is suppressed by default (so the NDJSON event stream on stdout stays parseable). `--tee-stderr` routes the tee echo to stderr.

### File mode (both commands)

R17. File mode persists per-file checkpoints (offset plus content fingerprint) at `~/.dash0/tail-state/` so a restart resumes where it stopped. Default on first run is `--from-beginning`.

R18. File rotation is handled by content fingerprinting (CRC of the first N bytes), not inode tracking.

R19. `--multiline-start "<regex>"` (logs only) coalesces multiline log records — lines that don't match the regex are appended to the previous record's body. Spans do not multiline-coalesce: each line must be a self-contained JSON object (if the producer needs multi-line JSON, it pre-processes with `jq -c` or equivalent).

### Compatibility

R20. The existing one-shot forms of `logs send` (positional `<body>`) and `spans send` (`--name` plus flags) are unchanged. Passing `--tail` together with the positional body (logs) or `--name` (spans) fails with a clear usage error.

## Key Flows

### F1. Tee a process's stdout into Dash0 while keeping the pipeline (logs)

**Trigger:** developer wants to ship their app's stdout to Dash0 without instrumenting it, but also wants to keep piping output to `grep` / `jq`.

Developer runs `myapp 2>&1 | dash0 -X logs send --tail | grep ERROR`.
The command prints the active profile and dataset on stderr; the rate sparkline starts on stderr.
Each line is parsed (auto-detected JSON, logfmt, or plain), wrapped in an OTLP log record, forwarded to Dash0, and echoed unchanged on stdout for `grep` to consume.
Logs appear in Dash0 within seconds AND the local terminal investigation continues.
Ctrl-C drains in-flight requests and exits.

**Covers R1-R10, R14-R16, R20.**

### F2. Tail rotated log files across restarts

**Trigger:** developer is debugging an app whose logs live in a rotating file at `/var/log/myapp/*.log` and wants persistent streaming across restarts.

Developer runs `dash0 -X logs send --tail '/var/log/myapp/*.log'`.
On first run, the command reads from the beginning, records checkpoints with content fingerprints, and forwards.
Each line also echoes to stdout — the user can pipe to `grep` or redirect for local inspection.
After Ctrl-C and a later restart with the same command, the command picks up where it left off, including rotation detected by CRC fingerprint.

**Covers R8, R14, R17-R19.**

### F3. Forward spans emitted by a CI pipeline into Dash0

**Trigger:** developer has wired their CI to append one JSON span per step into `ci-spans.jsonl` and wants the resulting trace in Dash0.

Each CI step runs something like `printf '%s\n' '{"name":"build","trace_id":"...","duration_ms":3041,"status_code":"OK","span_attributes":{"ci.job":"build"}}' >> ci-spans.jsonl`.
A long-running `dash0 -X spans send --tail ci-spans.jsonl` (started once at job start) reads each new line, validates the span shape, groups spans by `trace_id`, and forwards them as OTLP `ResourceSpans`.
The CI dashboard query in Dash0 sees a complete trace for the build, with one span per step, no SDK instrumentation in the CI scripts.

**Covers R1-R7, R11-R20.**

### F4. Agent-driven signal streaming

**Trigger:** an AI coding agent has started one of the commands to monitor signals during an automated investigation.

Agent runs `dash0 --agent-mode -X logs send --tail '/var/log/myapp/*.log'` (or `spans send --tail '/tmp/ci/*.jsonl'`) and parses NDJSON-OTLP/JSON from stdout.
First event is a log record with `event.name = "dash0.cli.logs_send_tail.started"` (or `dash0.cli.spans_send_tail.started`) carrying `source` and `dataset`.
`dash0.cli.<command>.stats` events tick once per interval with rate and total counts.
The tee echo is suppressed automatically.
If the outbound hop fails, a `dash0.cli.<command>.error` event surfaces the reason.
On SIGTERM the agent receives a `dash0.cli.<command>.shutdown` event.

**Covers R3, R4, R16.**

## Acceptance Examples

AE1. **Logs auto-detection picks the right shape (R9).**
Input line `{"level":"error","msg":"db timeout","duration_ms":3041}` → log record with `body` unset, attributes `level=error`, `msg=db timeout`, `duration_ms=3041`, severity inferred from `level`.
Input line `level=info msg="started" port=4318` → log record with logfmt fields lifted to attributes.
Input line `2026-06-11T10:00:00 starting app` → log record with `body="2026-06-11T10:00:00 starting app"`.

AE2. **Tee echo preserves the pipeline (R14).**
Running `printf 'one\ntwo\nthree\n' | dash0 -X logs send --tail` writes `one`, `two`, `three` to stdout in order, byte-for-byte unchanged.
`dash0 -X logs send --tail | wc -l` reports `3`.
Three OTLP log records are forwarded to Dash0.

AE3. **`--quiet` suppresses tee but not forwarding (R15).**
Running `printf 'one\ntwo\n' | dash0 -X logs send --tail --quiet` writes nothing to stdout. Two records still reach Dash0.

AE4. **Body argument and `--tail` are mutually exclusive (R20).**
`dash0 -X logs send "hello" --tail -` fails immediately with a clear usage error naming the conflict. Exit code non-zero. No records sent.
`dash0 -X spans send --name foo --tail ci-spans.jsonl` fails the same way.

AE5. **Multi-line stack traces coalesce — logs only (R19).**
With `--multiline-start "^\\d{4}-\\d{2}-\\d{2}T"`, three lines — `2026-06-11T10:00:00 ERROR Exception:`, `  at Foo.bar`, `  at Baz.qux` — coalesce into one log record whose body includes the timestamp line plus the indented frames.
The tee echo emits the three original lines unchanged.

AE6. **Fail-loud on Dash0 outage (KD5, R6).**
With Dash0's OTLP endpoint returning HTTP 503, each command prints a single stderr line per failed flush, drops the affected batch, and continues reading.
The tee echo on stdout continues unaffected.

AE7. **Restart resumes from checkpoint (R17, R18).**
With `dash0 -X logs send --tail '/var/log/myapp/app.log'` running, the developer hits Ctrl-C after 100 lines have been forwarded.
Restarting picks up from line 101, including rotation-via-rename detected by CRC fingerprint.

AE8. **Agent mode keeps stdout parseable (R16, KD8).**
Running `dash0 --agent-mode -X logs send --tail` produces only NDJSON-OTLP/JSON event records on stdout.
The original input lines do not interleave.
Adding `--tee-stderr` routes the input lines to stderr; stdout remains NDJSON-only and `jq -c .` over the stream parses every line cleanly.

AE9. **Span JSON shape mirrors `spans send` flags (R12, KD9).**
A file containing `{"name":"build","kind":"INTERNAL","status_code":"OK","duration_ms":3041,"span_attributes":{"ci.job":"build"}}` is read line-by-line by `dash0 -X spans send --tail`.
Each line becomes one span with the expected fields populated and missing IDs auto-generated.
A malformed line (e.g., `{"name":` — truncated JSON) prints one stderr error line and is skipped; subsequent lines continue normally.

AE10. **Spans grouped by `trace_id` (R13).**
A file with three lines sharing `"trace_id":"abc..."` and one line with a different `trace_id` produces two OTLP `ResourceSpans` requests in one batch — one carrying three spans, one carrying one — so the trace assembles correctly server-side.

AE11. **Stdin rejected for spans (R11, KD6).**
`echo '{"name":"foo"}' | dash0 -X spans send --tail -` fails with a clear usage error explaining that stdin is not supported for span streaming, pointing the user at `dash0 otlp proxy` or a file-based source.

## Success Criteria

- A developer pipes their app's stdout into `dash0 -X logs send --tail | <downstream>` and sees logs in Dash0 within 30 seconds while `<downstream>` continues to receive lines.
- A CI script writing JSON spans to `ci-spans.jsonl` produces a complete Dash0 trace via `dash0 -X spans send --tail ci-spans.jsonl`, with one span per step.
- The `send-log-event` GitHub Action and any existing scripts calling `spans send --name ...` keep working — the one-shot forms are unchanged.
- Auto-detection lands correctly on the common log shapes (JSON-lines, logfmt, plain text) without `--format` in everyday use.
- File-mode tailing survives one full rotation cycle for logs (rename + new file) without data loss in the verified case.
- Agent mode emits well-formed NDJSON-OTLP/JSON for the full lifecycle, parsable by the same OTLP decoder the agent uses for ingested data.
- Both commands exit cleanly within 5 seconds of SIGINT/SIGTERM, with no stale checkpoint locks or orphaned file handles.
- Roundtrip integration tests under `test/roundtrip/` cover send-and-query for both signals from file inputs (and stdin for logs), including the tee-stdout-preserved check.

## Scope Boundaries

### In scope (v1)

- `--tail` flag on `dash0 logs send` with stdin (`-`) and glob file modes.
- `--tail` flag on `dash0 spans send` with glob file mode only (no stdin per KD6).
- Tee semantics: input echoed to stdout by default; `--quiet` suppresses; agent mode auto-suppresses; `--tee-stderr` routes to stderr.
- Auto-detection of JSON-lines, logfmt, plain text for logs, with `--format` opt-out.
- Structured-JSON span shape mirroring `spans send` flag names.
- File-mode SQLite checkpoints + content-fingerprint rotation handling (both commands).
- `--multiline-start` single-flag multiline coalescing (logs only).
- Per-signal live stats with Unicode sparkline on TTY stderr; NDJSON-OTLP/JSON events on stdout in agent mode.
- `--verbose` / `-v` per-record verbose mode (collector debug-exporter style) on stderr (both commands).
- Trace correlation: spans sharing `trace_id` grouped into one OTLP `ResourceSpans` per batch.
- `-X` gating plus the agent-mode OTLP/JSON schema in KD8.
- Integration tests plus roundtrip tests for both commands.
- Documentation in `docs/commands.md`, `README.md`, plus changelog entries per `docs/changelog-maintenance.md`.

### Deferred for later

- Stdin streaming for `spans send --tail` (KD6 — not architecturally precluded; deferred to v2 if demand surfaces).
- Full OTLP/JSON wire-format span input (use `dash0 otlp proxy` if a process emits OTLP).
- Inactivity-flush timeout for in-progress multiline records (see OQ3).
- Per-line transformation pipeline (drop, sample, rewrite).
- Durable disk-backed buffering on outage.
- File-mode glob refresh — watching for new files matching the glob that didn't exist at start.
- OAuth-on-first-run for users without an active profile (composes with `feat/oauth-login` separately).
- Equivalent `--tail` for `metrics send` — metrics samples are not naturally line-shaped, so streaming isn't an obvious fit.

### Outside this product's identity

- Replacing the OpenTelemetry Collector (`STRATEGY.md` non-goal).
- TUI or local-inspection UI (`STRATEGY.md` non-goal).
- Plugin system for parsers, transforms, or sinks (`STRATEGY.md` non-goal).
- Tailing arbitrary remote sources (journald over SSH, Kubernetes pod logs across a cluster) — Collector territory.
- A `dash0 dev` umbrella for local-dev commands.
- Pointing the forwarder at non-Dash0 OTLP backends. The active profile's `otlp-url` is the only outbound target.

## Dependencies / Assumptions

- The user has an active Dash0 profile with a valid `otlp-url` and `auth-token`. v1 does not invoke OAuth-on-first-run.
- `dash0-api-client-go.SendLogs` and `SendTraces` continue to support OTLP/HTTP/JSON.
- Cobra and the existing flag, config, and agent-mode plumbing carry through unchanged.
- The CLI's existing `--max-retries` plumbing (default 3, max 5) applies to each outbound batch.
- File mode targets POSIX-like filesystems with reliable `fsnotify` semantics; Windows is best-effort and not a v1 success-criteria target.
- **Package layout:** the `--tail` logic for each command lives alongside the existing one-shot file — `internal/logging/send.go` gains a `tail.go` (or sub-file split per planning), `internal/tracing/spans_send.go` gains the same. Both reuse `internal/otlp/` for parsing utilities. No new `internal/ingest/` shared abstraction with the companion `dash0 otlp proxy` — that proxy extends `internal/otlp/` separately.

## Outstanding Questions

### Resolve before planning

OQ1. **`--verbose` detail level.**
The Collector debug exporter has `verbosity: basic | normal | detailed`. Pick one default for v1 (likely `detailed` — that's the FUD-killer) and decide whether `-v` / `-vv` / `-vvv` (or `--verbosity=<level>`) is a v1 flag or a later add. Should match whatever `otlp proxy --tail` lands on (companion brief OQ2).

OQ2. **Live-stats update interval.**
Same question as the proxy brief — 1 second is the obvious starting point but burns redraw budget on idle streams; 5 seconds is calmer. Resolve once across both briefs.

OQ3. **Span timestamp resolution rules.**
When a span JSON line provides `start_time` and `end_time` (or `start_time` and `duration`), use those. When neither is provided, what's the fallback? Use `start_time = now - duration` if duration is present; if neither is, error or auto-fill with `now`? The one-shot `spans send` has clear flag-level rules — we need to mirror them in the JSON shape.

### Deferred to planning

OQ4. Multiline state-machine subtleties (logs) — when does an open multiline record flush in the absence of new lines (inactivity timeout, EOF, SIGINT)? Maximum coalesced size before forcing a flush? Behavior when a new file matches the glob after start?

OQ5. CRC fingerprint window size and the on-disk checkpoint database schema (SQLite vs append-only file, lock granularity).

OQ6. Internal organization within `internal/logging/` and `internal/tracing/` — file split for tail-versus-one-shot, test seams, how file watcher and pipe reader (logs only) share lifecycle code.

## Sources / Research

- `docs/ideation/2026-06-11-otlp-proxy-local-dev-ideation.html` — the ideation artifact that surfaced these additions (idea I3, originally framed as `logs tail`) along with `dash0 otlp proxy` (idea I2).
- `docs/brainstorms/2026-06-11-otlp-proxy-local-dev-requirements.md` — companion brief for the `otlp proxy` command.
- `STRATEGY.md` — "Non-IaC workflows" track and "Not working on: Replacing the OpenTelemetry Collector" constraint.
- `internal/logging/send.go`, `internal/tracing/spans_send.go` — existing one-shot commands that `--tail` extends.
- `internal/otlp/` — shared OTLP utilities to reuse.
- `docs/commands.md` raw HTTP `api` section — precedent for `--verbose` / `-v` semantics on a long-running operation.
- `docs/commands.md` `spans send` section — flag names that the span JSON shape mirrors (KD9).
- `docs/adding-commands.md`, `docs/promoting-commands-to-stable.md` — the project's adding/promoting checklists.
- OTel filelog-receiver issues [#35162](https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/35162), [#31256](https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/31256) — documented pain points the logs v1 design routes around.
- [Fluent Bit `tail` input](https://docs.fluentbit.io/manual/data-pipeline/inputs/tail), [Vector `file` source](https://vector.dev/docs/reference/configuration/sources/file/), [Promtail `pipeline_stages`](https://grafana.com/docs/loki/latest/send-data/promtail/stages/) — convergent UX prior art for log tailing.
- [Honeycomb `buildevents`](https://github.com/honeycombio/buildevents) — CI-telemetry prior art for the span-streaming use case (F3).
- [OTel Collector debug exporter](https://github.com/open-telemetry/opentelemetry-collector/tree/main/exporter/debugexporter) — visual format for `--verbose` per-record printing.
- `tee(1)` — structural model for `--tail`'s pass-through-while-forwarding behavior.
