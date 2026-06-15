# OAuth access tokens have narrower server-side scope than static `auth_*` tokens

**Filed by:** dash0-cli, feat/oauth-login branch
**Affects:** Dash0 backend (API and OTLP ingress)
**Environment reproduced:** `api.eu-west-1.aws.dash0-dev.com` / `ingress.eu-west-1.aws.dash0-dev.com`
**Date:** 2026-06-12

## TL;DR

The OAuth 2.0 access tokens (`dash0_at_*`) issued via the new `dash0 login` flow are accepted by GET endpoints but rejected (or behave differently) on several WRITE endpoints, where the equivalent static `auth_*` token of the same user succeeds.
The two-cell matrix below was reproduced against the same dash0-dev environment, same dash0-cli binary, same user, varying only the auth token type.

| Operation | Static token (`auth_*`) | OAuth access token (`dash0_at_*`) |
|---|---|---|
| `GET /api/dashboards/:id` | ✅ 200 | ✅ 200 |
| `PUT /api/dashboards/:id` (mutate then delete then re-`PUT`) | ✅ 200 (restore-via-PUT works) | ❌ 404 |
| `PUT /api/views/:id` (any modification) | ✅ 200 | ❌ 404 |
| `PUT /api/recording-rules/...` (apply after a same-name create) | ✅ 200 | ❌ 403 "access denied; check your permissions" |
| `POST /v1/logs` (OTLP/JSON ingest) | ✅ 200 | ❌ 401 "invalid authentication token starting with 'at_…'" |

The CLI side has been verified — same code path, only `Authorization: Bearer <token>` differs.

## Who is affected

Anyone who authenticates via `dash0 login` (i.e. the new OAuth-based flow) and then runs any of:
- `dash0 logs send` / `dash0 spans send` (OTLP-bound write)
- `dash0 dashboards apply` / `update` for assets that follow a "create, second apply (no-op), delete, re-apply" pattern
- `dash0 views update` / `apply` for any modification
- `dash0 recording-rules apply` / `update` after the first apply

Existing users on static tokens are unaffected. CI (which uses a static `auth_*` token from `secrets.DASH0_AUTH_TOKEN`) has therefore never surfaced this gap.

## How this was missed until now

OAuth login is a brand-new feature shipping on the `feat/oauth-login` branch.
This is the first end-to-end exercise of the asset-mutation surface and OTLP ingress with a `dash0_at_*` token.
The existing roundtrip suite drives static tokens only.

A CI matrix change is being made on the same branch to run the roundtrip suite against both auth modes going forward.

## Reproduction setup

- CLI: `feat/oauth-login` HEAD (any recent commit; behavior is identical on `main` with an OAuth-authenticated profile).
- User: `user_2VYwDEhFTbOxPib8PP0lRNkyoND` (same user has both a static and an OAuth profile on the same Dash0 organization on dev).
- Static profile: `dash0-dev-cli-test` (static `auth_*` token; same org, same dataset `default`).
- OAuth profile: `minecraft` (OAuth-active, refresh-token issued via `dash0 login`, current access token format `dash0_at_*`).
- All requests issue `Authorization: Bearer <token>` via the same `dash0-api-client-go` library.

## Reproductions

### 1. Dashboard PUT after no-op-update → 404 only under OAuth

```bash
# Bisected operation sequence:
# apply, delete, apply              → PASS under both auth types
# apply, apply (no-change), delete, apply → PASS for static, FAIL for OAuth
```

CLI session (OAuth profile):
```
$ dash0 apply -f /tmp/dash.yaml
Dashboard "Trace Capture Dashboard" (trace-cap-a38a7d9aca1d44a08895f32aca6b662b) created

$ dash0 apply -f /tmp/dash.yaml
Dashboard "Trace Capture Dashboard": no changes

$ dash0 dashboards delete trace-cap-a38a7d9aca1d44a08895f32aca6b662b --force
Dashboard "trace-cap-a38a7d9aca1d44a08895f32aca6b662b" deleted

$ dash0 apply -f /tmp/dash.yaml
Error: document 1 (Dashboard): dashboard "Trace Capture Dashboard" not found
  (status: 404, trace_id: 76aa8d09c34b528adbdd5db8274435d7)
  Not Found: The requested dashboard does not exist or is inaccessible to you.
```

The exact same sequence with `--profile dash0-dev-cli-test` (static token) succeeds with `Dashboard "Trace Capture Dashboard": no changes` (restore-via-PUT works).

The dashboard is still present in the soft-deleted state — `GET /api/dashboards/:id` returns 200 with `metadata.annotations["dash0.com/deleted-at"]` populated.
PUT to the same path, same body, with a static token restores it.
PUT with an OAuth token returns 404.

**Trace IDs to investigate (OAuth):** `76aa8d09c34b528adbdd5db8274435d7`, `21e835a3fb56852ea316003bb8e230a8`

### 2. View update (any modification) → 404 under OAuth

The CLI-side path for views had a separate, real CLI bug (the `dash0.com/origin` label was being echoed back in the PUT body, causing a 400 "origin mismatch").
That bug is fixed in this branch (`internal/views/update.go` now calls `StripViewServerFields` before PUT, matching the apply code path).

After that fix, OAuth view updates surface a clean 404 from the server while the same code path on a static token returns 200:

```
$ dash0 views update -f exported-view.yaml      # OAuth profile
Error: view "Roundtrip Fix Test MODIFIED" not found (status: 404, trace_id: ...)

$ dash0 views update -f exported-view.yaml      # static-token profile, same view
View "Roundtrip Fix Test": updated
```

Behavior also reproduces directly against the API via curl: `PUT /api/views/<origin>` with a body whose `dash0.com/origin` matches the URL returns 200 under a static token but 404 under an OAuth access token.

### 3. Recording-rule apply (second apply) → 403 under OAuth

```bash
$ dash0 apply -f /tmp/rr.yaml      # OAuth, first apply
Recording rule "trace-rr-1781255639" (recording_rule_group_01ktxhr4ycetxsy8fq2kwehx5q) created

$ dash0 apply -f /tmp/rr.yaml      # OAuth, second apply
Error: document 1 (PrometheusRule): access denied; check your permissions
  (status: 403, trace_id: 747c4dbaccff44e194a60fd921f3671d)
  Forbidden: The given principal does not have the required permissions.
```

Same fixture, same dataset, with the static-token profile both applies succeed.
The 403 message ("the given principal does not have the required permissions") strongly suggests this is an authz check that has not been updated to grant OAuth-issued tokens the same permissions as the static token of the same user.

**Trace ID to investigate (OAuth):** `747c4dbaccff44e194a60fd921f3671d`

### 4. OTLP ingest → 401 under OAuth

Direct curl against the OTLP ingress with the active access token from the `minecraft` profile:

```bash
$ curl -s -X POST https://ingress.eu-west-1.aws.dash0-dev.com/v1/logs \
       -H "Authorization: Bearer dash0_at_8d1565f..." \
       -H 'Content-Type: application/json' \
       -d '{"resourceLogs":[]}'
{"code":16,"message":"invalid authentication token starting with 'at_8d1565f'"}
```

The same OTLP endpoint accepts the user's static `auth_*` token without issue.
The OTLP ingress appears to strip the `dash0_` prefix internally and then reject the resulting `at_…` value as an unrecognized token format.

The CLI now pre-flight-refuses send commands when the active profile is OAuth-typed and no static override is in effect, with a hint pointing users at the workaround (`DASH0_AUTH_TOKEN=auth_<...>` or `dash0 config profiles update <name> --oauth=false --auth-token auth_<...>`).
This is a CLI-side mitigation, not a fix for the underlying gap.

## Suggested questions for the backend team

1. **Is `dash0_at_*` (OAuth access token) expected to be honored on OTLP ingest?**
   If yes: there is a backend regression in the OTLP token-validation path that drops the `dash0_` prefix before matching.
   If no: the documentation must call this out, and `dash0 login` users will need a parallel "service-account static token" for telemetry sending.

2. **Are OAuth access tokens issued by `/oauth/token` granted the same scopes as a user's static `auth_*` token?**
   The 403 on recording-rules and the 404 on dashboard/view PUT (when restoring or updating) is the same shape an authz-deny would take if the OAuth token's permissions are a strict subset.
   We expect parity: an OAuth-issued token for user X should be able to do whatever a static token for user X can do.

3. **For dashboards specifically: does PUT-after-soft-delete check `deleted-at` differently for OAuth vs static tokens?**
   The reproduction is precise — same body, same URL, same user; only the bearer token differs.
   GET returns 200 for both (so the dashboard is reachable in the soft-deleted state).
   PUT returns 200 for static, 404 for OAuth.

4. **Is the 404 leaking authz state?**
   A 404 on a resource the caller cannot prove doesn't exist (because GET on the same path returns 200) typically indicates the server is masking a 403 as a 404 to avoid leaking existence.
   That's a fine pattern, but in this case both GET and PUT use the same user context — masking GET-existence with PUT-404 is internally inconsistent.

5. **Refresh-token rotation policy?**
   Tangential, but needed for the CI matrix that will exercise OAuth roundtrips: does the refresh token rotate on use, or remain stable across token exchanges?
   If it rotates, the CI matrix needs a re-provisioning step; if stable, a long-lived secret suffices.

## CLI-side mitigations already shipped in `feat/oauth-login`

- `internal/client/client.go` `checkOAuthOnOtlp` refuses OTLP send commands upfront when the active profile is OAuth-typed.
- `internal/login/logout.go` requires `--force` in agent mode so an AI agent cannot silently destroy an OAuth session.
- `internal/oauth/sanitize.go` strips newlines / line separators in AS-supplied error text to block forged-line injection.
- `internal/views/update.go` strips `dash0.com/origin` before PUT, matching the apply path.

The CLI does not currently pre-flight-refuse asset-mutation operations under OAuth, because the failures depend on server state (soft-delete history, recording-rule-name collisions) and can't be detected client-side without a round-trip.
A flat "OAuth profile cannot use `apply`" gate would be too broad — `apply` of a brand-new asset does work under OAuth today.

## References

- Branch: `feat/oauth-login` on dash0hq/dash0-cli
- Failing roundtrip tests on this branch (OAuth profile only):
  - `test/roundtrip/test_apply_dashboard_idempotency.sh`
  - `test/roundtrip/test_apply_view_idempotency.sh`
  - `test/roundtrip/test_view_roundtrip.sh`
  - `test/roundtrip/test_apply_recording_rule_idempotency.sh`
  - `test/roundtrip/test_perses_dashboard_roundtrip.sh`
- Same tests under `--profile dash0-dev-cli-test`: all PASS.
- Same tests under CI's `secrets.DASH0_AUTH_TOKEN` (static): historically pass, except for unrelated ingestion-latency flakes on `test_log_roundtrip` / `test_span_roundtrip`.
