# Dash0 CLI Development Guide

This repository provides a CLI utility to perform various tasks related with Dash0, enabling users via terminal,
AI agents and CI/CD workflows to perform tasks like creating, updating and deleting a number of assets in Dash0 like dashboards, alerting rules, views and more.

## Commands
- Build: `make build`
- Test all: `make test`
- Test unit only: `make test-unit`
- Test integration only: `make test-integration`
- Roundtrip tests only: `make test-roundtrip`
- Test specific: `go test -v ./internal/config -run TestServiceAddContext`
- Run locally: `./dash0 [command]`

## Development guidelines

Detailed guidelines are split into focused documents:

- @docs/adding-commands.md — step-by-step guide for adding a new command
- @docs/promoting-commands-to-stable.md — how to remove the experimental gate from a command
- @docs/commands.md — full command reference with flags, outputs, and examples
- @docs/cli-naming-conventions.md — command naming, aliases, asset kind display names
- @docs/code-style.md — Go style, dependencies, error handling
- @docs/project-structure.md — directory layout, package responsibilities
- @docs/documentation.md — prose rules, attribute keys in examples, validation
- @docs/testing.md — test strategies, integration tests, fixtures, mock server, roundtrip tests
- @docs/github-actions-maintenance.md — keeping the setup and send-log-event composite actions in sync with CLI changes, plus how the setup action is tested
- @docs/changelog-maintenance.md — when and how to create changelog entries
- @docs/agent-skill-maintenance.md — keeping the dash0-cli Agent Skill bundle (`internal/skill/`) in sync with `docs/commands.md`

## Hard rules

- **Adding support for an asset type (or a new schema version of an existing one)  must follow @docs/adding-commands.md and @docs/cli-naming-conventions.md in full.**
  That document is the authoritative checklist: every numbered step and every requirement are required, not optional.

- **When editing @README.md's top-of-file positioning or its Installation section, mirror the changes into @docs/about.md or @docs/installation.md respectively (and vice versa).**
  Both docs are synced to dash0.com/docs on every release — drift ships as stale website content. See the "Docs synced to dash0.com/docs" section of @docs/documentation.md for the specifics.

## Key Dash0 API concepts

### Origin vs ID

**Origin** and **ID** are distinct concepts — do not conflate them.

- **Origin** identifies which system is the authoritative source of truth for an asset (e.g., `dash0-cli`, `terraform`, `ui`).
  It records provenance, and the CLI never changes a stored origin.
  It is stored as the label `dash0.com/origin`; check rules keep it in their top-level `labels`.
  For the routed kinds below, the origin is also the key a create writes by, so the CLI sends it on a create and clears it before an update at the id.

- **ID** is the user-defined external identifier used for upsert (create-or-replace) operations.
  `apply` and `create` read an asset by its ID first. Every kind `apply` accepts is written by PUT; only `auth-tokens create` and the flag-based `teams create <name>` still POST.
- **One routing rule** (`asset.RouteDocument` in `internal/asset/router.go`) serves dashboards, ephemeral dashboards, PersesDashboards, check rules, PrometheusRules (alerting and recording rules), synthetic checks, views, teams, notification channels, time series aggregations, spam filters, SLOs, evals, signal-to-metrics rules, sampling rules, Agent0 skills, and datasets.
  AWS metric scrape configs do not use `RouteDocument`. They have their own rule (`asset.PlanAwsMetricScrapeConfig`): read at the integration id, and fail on 404, because an unknown integration cannot be created.
  A document with `dash0.com/id` is read by that id; on hit the CLI PUTs at the id, and fails when the stored origin differs from an explicit origin in the file.
  Views, synthetic checks, and recording rules send the stored origin back on an update at that origin (`asset.UpdateOrigin`), because Dash0 echoes the body's labels on a read by id; an update at the id cannot send it, since the API rejects a body origin there.
  On 404, or without an id, the CLI PUTs by the file's origin or the inferred one, never by the id and never with POST.
  Check rules and PrometheusRule alerting rules are the exception: an id that is not found is created at that id (`Identity.CreatesAtID`), so the file keeps its id.
  There is no lookup by name.
  The origin is sent on create; an update at the id clears it from the body.
  The inferred origin is `dash0-cli_` plus a hash of the kind, `metadata.name`, and the dataset (`asset.InferOrigin`), with no clock, randomness, or file path.
  Two documents without an id that would write the same origin fail validation (`asset.CheckInferredOriginCollisions`): two with the same inferred origin, or one whose explicit origin is the other's inferred origin.
  A document whose id is not found fails at write time when another document of the run already wrote its inferred origin (`asset.InferredOriginWrites`).
  `apply --since` and `diff --since` key a document without an id or an origin by its inferred origin for the run's `--dataset` (`asset.ExtractInferredKeys`, passed to `gitutil.ComputeSincePlan`), so a removed or renamed one is deleted there; a PrometheusRule expands to one key per alert and one for its recording rules.
  Only datasets without a slug and AWS metric scrape configs without an id still fail `--since` without an identifier.
  `TestRouter_EveryKindFollowsTheSameOrder` checks every kind in `ValidKinds`. Datasets, whose id is required, and AWS metric scrape configs, with their own rule, run their own case tables there (`routedKind.cases`).
  The ID field varies by asset type:
  - Check rules: routed. The top-level `id` field (`PrometheusAlertRule.Id`), then `labels["dash0.com/origin"]` in the top-level labels. The inferred origin hashes `name`. The top-level labels are the alerts' Prometheus labels, so the CLI reads the origin from the file into `asset.CheckRule.Origin` and never sends it in a body; the path carries it. The GET returns no origin label, so the stored-origin check reads the stored origin through `asset.CheckRuleStoredOrigin`. A rule whose id and origin differ fails validation and `RouteDocument` (`asset.IDOriginConflictError`), because it is created at the id and a later apply could not match the origin. An id that is not found is created at that id; only a rule without an id writes by its origin or inferred origin. `check-rules update -f` fails on an id that is not found, and `apply --since` deletes at the id.
  - Views: routed. `metadata.labels["dash0.com/id"]` (`ViewLabels.Dash0Comid`), then `metadata.labels["dash0.com/origin"]`. The server rejects a body origin that differs from the path, which is why an update never sends it.
  - Synthetic checks: routed. `metadata.labels["dash0.com/id"]`, then `metadata.labels["dash0.com/origin"]`.
  - Dashboards: routed. The ID is `metadata.labels["dash0.com/id"]` (the server UUID); the origin is `metadata.labels["dash0.com/origin"]`, or `metadata.dash0Extensions.id` when the label is absent (`asset.DashboardIdentity`). Read them before `StripDashboardServerFields`, which clears `dash0.com/id`. An origin that differs from `dash0Extensions.id` fails validation. A UUID origin is not set as the origin label on create, because the API stores a UUID key as the id. `asset.DashboardKeys` and `asset.DashboardAlias` still serve `update`, `preview`, and `apply --since`.
  - PersesDashboard CRD: routed as a dashboard. `metadata.labels["dash0.com/id"]`, then `metadata.labels["dash0.com/origin"]`. The library's conversion drops both labels, so `asset.ParseDashboard` copies them back (`keepPersesIdentity`). The inferred origin hashes kind `dashboard`, as `apply` converts the CRD first, and `asset.InferKindOrigin` maps `persesdashboard` to it so `--since` deletes at the same key.
  - PrometheusRule CRD: routed per asset. Each alert reads its id (the CRD's `dash0.com/id`, or `asset.DeriveAlertCheckRuleID` of it for two or more alerts), then its origin: the CRD's `dash0.com/origin` derived the same way, or an inferred origin that hashes kind `prometheusrule` and `metadata.name` NUL `<group> - <alert>`. The recording rules are one asset: the CRD's id, then its origin, or an inferred origin that hashes kind `recordingrule` and `metadata.name`. An alert whose id is not found is created at that id, as for check rules. Collision checks and `--since` identify the CRD by its own labels and `metadata.name`.
  - Spam filters (v1alpha1 and v1alpha2): routed. `metadata.labels["dash0.com/id"]`, then `metadata.labels["dash0.com/origin"]`.
  - Notification channels: routed. `metadata.labels["dash0.com/id"]`, then `metadata.labels["dash0.com/origin"]`; organization-level, so the inferred origin uses no dataset.
  - SLOs: routed. `metadata.labels["dash0.com/id"]`, then `metadata.labels["dash0.com/origin"]`. IDs are server-assigned (`slo_<ulid>`), so a hand-authored document pins `dash0.com/origin` or relies on the inferred one. An id from another organization is not found, so the SLO is created by its origin, never by POST. `StripSLOServerFields` clears the origin, because the path carries it.
  - Datasets: routed. The id is the slug: `spec.slug`, or `metadata.labels["dash0.com/id"]` when `spec.slug` is absent — required and immutable. Then `metadata.labels["dash0.com/origin"]`; organization-level, and the inferred origin hashes the slug in place of `metadata.name` (`asset.DatasetIdentity`). A PUT to an origin creates a missing dataset, with the slug from the body, and restores one deleted in the last 30 days, so there is no POST. A document whose origin belongs to a dataset with another slug fails before the write (`asset.DatasetSlugChangeError`), because the slug cannot change. `apply` and `diff` leave datasets out of `documentIdentity`, so `--since` keeps deleting them at the slug. `apply --since` deletes a dataset removed from git like any other kind, as a 30-day soft delete; in agent mode it needs `--force`.
  - AWS metric scrape configs: id only. `metadata.labels["dash0.com/id"]` is the AWS integration id and the only key; a 404 fails, because the CLI cannot create an integration. There is one config per integration, PUT replaces it, and DELETE clears it while keeping the integration. The first write after none or a clear is the create: it sends `dash0.com/origin`, or the origin inferred from the integration id, and Dash0 ignores the origin on later writes, so an update never sends it. A stored origin that differs from the file's fails. `spec.namespaces` is required, because the client sends a missing list as `[]`, which stops all scraping.
  - Evals (`Dash0GenerativeAiEvaluation`): routed. `metadata.labels["dash0.com/id"]`, then `metadata.labels["dash0.com/origin"]`. Read them before `StripGenerativeAiEvaluationServerFields`, which drops `dash0.com/id` and `dash0.com/version` (never send the version: without it, PUT overwrites unconditionally) but, unlike `StripSLOServerFields`, keeps `dash0.com/origin` in the body. So the plan sets the routed origin on a create, and the import clears it before an update.
  - Agent0 skills (`Dash0Skill`): routed. `metadata.labels["dash0.com/id"]`, then `metadata.labels["dash0.com/origin"]`; organization-level, so the inferred origin uses no dataset. The inferred origin hashes the skill name from the `SKILL.md` frontmatter (`asset.CustomSkillName`), not `metadata.name`, because the frontmatter name is what the server keys a skill by. Read the origin and ID before `StripCustomSkillServerFields`, which drops `spec.source`, `metadata.name`, and the per-user `dash0.com/enabled` annotation that a write must never send.
  - Signal-to-metrics rules (`Dash0SignalToMetrics`): routed. `metadata.labels["dash0.com/id"]`, then `metadata.labels["dash0.com/origin"]`. The CLI clears `dash0.com/id` from the body of a PUT that is not at that id. The GET ignores the dataset, so the CLI compares the stored rule's dataset with `--dataset` before the write (`asset.CheckSignalToMetricsRuleDataset`).
  - Sampling rules (`Dash0Sampling`): routed, with the same keys as signal-to-metrics rules, through `GET /api/sampling-rules/{id}`. IDs are unique across organizations, so an unknown id is never PUT; the rule is created by its origin instead. Read the origin and ID before `StripSamplingRuleServerFields`, which also drops the `dash0.com/dataset` label: on writes that label wins over the `dataset` query parameter. Unlike signal-to-metrics rules, the GET honors the dataset, so a rule in another dataset fails the read with a 400, which is not "not found" and which `delete --force` must not treat as already deleted. Deletes are permanent, so there is no `restored` action.
  - Time series aggregations: routed. `metadata.labels["dash0.com/id"]`, then `metadata.labels["dash0.com/origin"]`; a document without either is written at its inferred origin, so the origin is no longer required.
  - Teams: routed. `metadata.labels["dash0.com/id"]`, then `metadata.labels["dash0.com/origin"]`; organization-level, so the inferred origin uses no dataset. An exported SSO/SCIM team updates at its id even though its `saml://...` origin does not resolve.

  The user-facing documentation of these fields, including the rationale for idempotent upsert, lives in the [Asset identifiers and idempotent upsert](docs/commands.md#asset-identifiers-and-idempotent-upsert) section of `docs/commands.md`.

The `ORIGIN` column in `list -o wide` output shows the `origin` provenance field, not the ID.

For dashboards, the ID must be cleared from the request body before PUT (update) calls.
The ID is already passed as the URL path parameter (`originOrId`); sending it in both places causes the server to reject the request when the ID is a UUID that matches the dashboard's own server-assigned ID.
Use `dash0api.ClearDashboardID()` before calling `UpdateDashboard`.
This applies to both `ImportDashboard` (used by `apply`) and `dashboards update`.

### Asset annotations

Assets (dashboards, views, synthetic checks) have typed annotation structs with these fields:

| Annotation | User-settable | Behavior |
|------------|---------------|----------|
| `dash0.com/folder-path` | Yes | Fully user-controlled, round-trips through the API unchanged |
| `dash0.com/source` | Yes (dashboards only) | User-settable, but the server overrides it based on the API path used (e.g., `api`, `ui`) |
| `dash0.com/sharing` | Write-only | The API parses it into typed `Permissions` objects in `spec.permissions`, then strips the annotation before persisting. It is never returned in GET responses. Format: `"role:basic_member,team:team_123,user:alice@example.com"` |
| `dash0.com/deleted-at` | No | Server-managed soft-delete timestamp |

**`Strip*ServerFields` functions** (`dash0-api-client-go`) remove server-managed noise before sending assets to the API and for diff rendering.
They must only strip truly server-managed fields (`deleted-at`, `createdAt`, `updatedAt`, `version`, `dataset`, `origin`).
User-settable annotations (`folder-path`, `sharing`, `source`) and `spec.permissions` must be preserved.

**`spec.permissions`** on views and synthetic checks holds typed permission objects (role + actions).
These are user-provided and must not be stripped.

## GitHub issues

When creating GitHub issues, describe **what** and **why** — not **how**.
Issues should only contain the problem statement, user-facing behavior, and acceptance criteria.

Every pull request links to a GitHub issue, and the branch and the changelog entry use its number (see the "Issues, branches, and pull requests" section of CONTRIBUTING.md).
