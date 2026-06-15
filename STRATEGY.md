---
name: Dash0 CLI
last_updated: 2026-06-11
---

# Dash0 CLI Strategy

## Target problem

SREs and developers who run Dash0 as their observability platform need to manage Dash0 assets (dashboards, views, check rules, and so on) and send signals from scripts, CI pipelines, and ad-hoc terminal sessions.
The existing paths are too heavyweight for this work.
The Dash0 Terraform provider and the Dash0 operator for Kubernetes are too ceremonious for shell scripting and one-off ops, and a full OpenTelemetry pipeline (apps plus collectors) is overkill for sending a single deployment event or test signal.
Agentic AI that needs to drive Dash0 hits the same gap, with no self-discoverable, structured surface to work against.

## Our approach

Be the *kubectl for Dash0*: a fast, scriptable, terminal-native surface that covers both declarative IaC (`apply -f`, typed YAML, and CRDs like PersesDashboard and PrometheusRule) **and** imperative operations (sending OTLP signals, querying logs, spans, and metrics, login, raw API passthrough).
Agent ergonomics ride on top via `--agent-mode`, designed humans-first: when human and agent ergonomics conflict, humans win.

The bet against the Dash0 Terraform provider and the Dash0 operator for Kubernetes is not "we are better at IaC".
It is that neither of them will ever achieve universal adoption or feel quick to script against, and neither can carry the imperative half at all.

We will know this is working when scripting against `dash0` becomes the default way users automate Dash0 from a terminal or CI runner, and when agentic AI reaches for it instead of the raw API.

## Who it's for

**Primary:** SREs and developers (DevOps) who run Dash0 as their observability platform.
They are hiring `dash0` to interact with Dash0 from the terminal — to script in shell or CI, or simply because they prefer the terminal over the UI.

**Secondary:** agentic AI, served via `--agent-mode` (JSON output, JSON help, JSON errors, prompt suppression).
Humans-first is an explicit design rule.

## Tracks

### Asset coverage parity

Keep the CLI at coverage parity with Dash0 itself, the Dash0 Terraform provider, and the Dash0 operator for Kubernetes.
Every new asset type or schema version those surfaces support is a candidate for the CLI on the same release cadence.

_Why it serves the approach:_ without parity the CLI is a second-class IaC surface, which kills the "kubectl for Dash0" promise.
Users would fall back to the heavier tools the CLI exists to complement.

### Agent-native ergonomics

`--agent-mode`, structured JSON help, errors, and output, auto-detection of agent environments, agent-friendly defaults (skipped prompts, no color, stable output shapes).
The investment is in the contract that lets agents self-discover and drive `dash0` reliably.

_Why it serves the approach:_ the secondary persona only exists if the agent surface is first-class.
Without this track, agents fall back to the raw Dash0 API and the CLI's discoverability promise collapses.

### Non-IaC workflows

Telemetry sending (`logs send`, `spans send`, future signal types), query commands (`logs query`, `spans query`, `traces get`, `metrics instant`), and lowering the bar for OpenTelemetry adoption through Dash0 — so the CLI carries imperative work, not just declarative state.

_Why it serves the approach:_ half of the bet against the Dash0 Terraform provider and the Dash0 operator for Kubernetes is that imperative ops belong in the same tool as `apply`.
Without this track the CLI is just a faster Terraform; with it, it is the terminal-native surface for Dash0 end to end.

## Not working on

- A TUI.
- A plugin system.
- Replacing other Dash0 IaC tools (Terraform provider for Dash0, Dash0 operator for Kubernetes).
- Replacing the OpenTelemetry Collector.
