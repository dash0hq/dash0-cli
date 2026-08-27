# Integration Tests

Integration tests use a mock HTTP server (`internal/testutil/mockserver.go`) with JSON fixture files to simulate API responses from the Dash0 API.
The OpenAPI specification of the Dash0 API is available at `https://api-docs.dash0.com/reference`.

## Fixture Location
- Fixtures are stored in `internal/testutil/fixtures/`
- Organized by asset type: `dashboards/`, `checkrules/`, `views/`, `syntheticchecks/`
- Common fixture patterns: `list_success.json`, `list_empty.json`, `get_success.json`, `error_not_found.json`, `error_unauthorized.json`

## Generating Fixtures
- Fixture generation scripts are in `internal/testutil/fixtures/scripts/`
- Scripts follow the naming pattern: `<asset>_<functionality>.fixture.sh`
- Run all scripts: `DASH0_API_URL='https://api...' DASH0_AUTH_TOKEN='auth_...' ./generate_all.sh`
- Scripts output to stdout; redirect to create fixture files

## Writing Integration Tests
- Add `//go:build integration` at the top of integration test files (before `package`)
- Name files with `_integration_test.go` suffix for clarity
- Use `testutil.NewMockServer(t, testutil.FixturesDir())` to create a mock server
- Register routes with `server.On()` for exact paths or `server.OnPattern()` for regex patterns
- Always use `testutil.RequireHeaders` as the validator to ensure auth token and user agent validation
- Use constants for API paths and fixture filenames to avoid duplication
- Use `http.MethodGet`, `http.StatusOK`, etc. instead of string/numeric literals

## When to Update Fixtures
- When the Dash0 API response format changes
- When adding tests for new API endpoints
- When existing tests fail due to outdated fixture data
- Run `generate_all.sh` periodically to keep fixtures in sync with the actual API

# Roundtrip Tests

Roundtrip tests live in `test/roundtrip/` and exercise the CLI end-to-end against a real Dash0 environment.
They create assets, read them back, verify the output, and clean up.

## Prerequisites
- Build the CLI first: `make build`
- An active profile with `api-url` and `auth-token` must be configured.

## Running
- Run all: `bash test/roundtrip/run_all.sh`
- Run one: `bash test/roundtrip/test_dashboard_roundtrip.sh`

## Structure
- `run_all.sh` — orchestrator that runs all test scripts and reports pass/fail counts.
- `test_<asset>_roundtrip.sh` — CRUD roundtrip for each asset type (create, get, list, update, delete).
- `test_apply_<asset>_idempotency.sh` — verifies that `apply` is idempotent (apply twice, second reports no changes).
- `test_<asset>_annotations.sh` — verifies that user-settable annotations (`folder-path`, `sharing`) survive a roundtrip.
- `test_prometheus_rule_roundtrip.sh` — roundtrip for PrometheusRule CRD import via `check-rules create`.
- `test_perses_dashboard_roundtrip.sh` — roundtrip for PersesDashboard CRD import via `dashboards create`.
- `test_log_roundtrip.sh`, `test_span_roundtrip.sh` — send and query telemetry signals.
- `test_team_roundtrip.sh` — team CRUD and membership management.
- `fixtures/` — YAML asset definitions used as input by the test scripts.

## When to Add Roundtrip Tests
- When adding a new asset type or command, add a corresponding `test_<asset>_roundtrip.sh`.
- When adding a new `apply` code path (e.g., a new CRD format), add an idempotency test.
- When adding annotation support to an asset type, add an annotations test.
- When adding a new signal command (e.g., `metrics send`), add a send-and-query roundtrip.
- Register every new test script in `run_all.sh` (in `API_TESTS` or `OTLP_TESTS`).
  CI discovers test scripts automatically by scanning `test/roundtrip/test_*.sh`.

# End-to-End Tests

End-to-end tests run the real `dash0` binary against a real `git` binary inside a container, using [testcontainers-go](https://golang.testcontainers.org/).
They exist to prove that code shelling out to `git` (see `internal/git/`) works across a real process boundary — something neither unit tests (in-process) nor integration tests (a real temp git repo, but still in-process against a mocked HTTP server via `httptest`) can cover.
They need no live Dash0 credentials: the mock API server runs on the host and is exposed to the container via testcontainers-go's [`WithHostPortAccess`](https://pkg.go.dev/github.com/testcontainers/testcontainers-go#WithHostPortAccess), reachable at `http://host.testcontainers.internal:<port>`.

## Running
- Requires Docker (or a Docker-compatible daemon) running locally.
- Run: `make test-e2e`.
- **Colima users**: testcontainers-go's Docker auto-detection does not recognize colima's non-standard socket forwarding. Export these first:
  ```bash
  export DOCKER_HOST="unix://$HOME/.colima/default/docker.sock"
  export TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE="/var/run/docker.sock"
  ```
- GitHub-hosted `ubuntu-latest` CI runners have a working Docker daemon natively, so `test-e2e` needs no special CI runner capability, unlike a self-hosted Docker-in-Docker setup.

## Structure
- `test/e2e/Dockerfile` — a minimal Alpine image with `git` installed; never shipped, used only by this test tier.
- `test/e2e/setup_test.go` — cross-compiles a linux binary for the container's architecture (on the host, so the module's local `go.mod` replace directives, if any, resolve normally) and builds the image once per test binary run.
- `test/e2e/since_e2e_test.go` — one test per `--since` scenario fixture (see [Shared git-repo scenario fixtures](#shared-git-repo-scenario-fixtures) below), each starting a fresh container, copying the scenario's repo in, and asserting on `dash0`'s exit code and output.
- All files are tagged `//go:build e2e`, so `go build`/`go test` skip them unless `-tags e2e` is passed (as `make test-e2e` does).

## A note on container file ownership

`docker cp` (used to copy a scenario's repo into the container) preserves the host file owner's UID, which does not match the container's root user.
This trips git's "dubious ownership" guard (the fix for CVE-2022-24765) the same way a real CI environment can when a checkout is owned by a different UID than the one running commands — `actions/checkout` works around exactly this by marking the checkout safe.
The e2e harness does the same (`git config --global --add safe.directory '*'` inside the container after copying) rather than disabling the protection inside `dash0` itself.

## When to Add End-to-End Tests
- When adding a new `--since`/`--diff`-style scenario fixture (see below), add a matching `TestE2E_*` case.
- Scope new coverage to commands that actually shell out to `git`; commands that only call the Dash0 API are already covered by integration tests.

# Shared Git-Repo Scenario Fixtures

`--since`-related tests (unit, integration, and end-to-end) share one set of git-repo fixtures rather than each tier hand-rolling its own git setup.

## Fixture Location
- Checked-in fixtures: `internal/testutil/fixtures/git-scenarios/<scenario-name>.yml`, one `GitRepoFixture` document per scenario.
- Each fixture declaratively lists the commits to replay: `spec.repo.commits` is an ordered list, each with a `message`, an ordered list of `changes` to apply, an optional `label` naming the commit for later reference, and an optional `resetTo` that hard-resets to a labeled commit first (used to simulate a force-push). `spec.sinceRef` is the `--since` value the scenario is meant to be tested with: either a commit's `label` or a literal ref (e.g. git's all-zeros sentinel) used as-is.
- Each entry in `changes` has an explicit `op` (`add`, `modify`, or `delete`), a file `name`, and (for `add`/`modify`) its new full `content`. `op` is deliberately explicit rather than inferred from the same file name reappearing with different content in a later commit: a commit's intent reads correctly on its own, and `BuildGitScenario` cross-checks it against the file's actual existence at that point in history (e.g. `add` on a file that's already there, or `modify`/`delete` on one that doesn't exist yet, fails loudly with a message naming the likely correct `op`).
- There is no generation step or binary artifact to keep in sync: the fixture *is* the repo's history, in a form that's readable and diffable directly in review. A test builds the real repo from it fresh, every run.
- `internal/testutil/git_repo_fixture.schema.json` is the JSON Schema for this format; `TestGitScenarioFixtures_MatchSchema` in `internal/testutil/gitscenario_test.go` validates every checked-in fixture against it.

## Go Helper
`internal/testutil.BuildGitScenario(t, name) (repoDir, ref string)` parses a named scenario's YAML and replays its commits into a fresh `t.TempDir()` repo, returning the repo path plus the resolved ref to pass as `--since`. This is the one thing every test tier calls.

## Scenarios
- `whole-file-deletion` — a file is removed entirely between the ref and HEAD.
- `whole-directory-deletion` — every file under the ref is removed between the ref and HEAD, leaving the `-f` target (the repo root, in this scenario) with zero eligible YAML files.
- `directory-rename` — a file moves from one subdirectory to another between the ref and HEAD; deletion detection is by identifier, never by file path, so this must be a plain update, not a deletion.
- `multi-document-partial-deletion` — one document is removed from a multi-document YAML file; the file survives.
- `prometheus-alert-partial-deletion` — one alerting rule is removed from a `PrometheusRule` CRD; the CRD (and its shared `dash0.com/id`) survives.
- `prometheus-recording-partial-removal` — the same shape for a recording rule; the alert is a plain update, and the recording rule the CRD no longer declares is deleted despite the CRD's own identifier surviving (a coarse presence/absence signal, since there is no per-record identity to diff).
- `first-push-new-branch` — a minimal one-commit repo paired with the literal all-zeros SHA as the ref, simulating a branch's first push.
- `non-ancestor-force-push` — a commit is orphaned by a simulated force-push (`git reset --hard` + a new commit); it still resolves by SHA but is not an ancestor of HEAD.
- `too-shallow-clone` — the checked-in fixture carries full history; a `--depth 1` clone (performed by the test itself, via `file://`, not baked into the zip) makes the older ref unresolvable.
