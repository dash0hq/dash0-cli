# Sync Dash0 assets

Sync a directory of [Dash0](https://www.dash0.com) configurations (dashboards, check rules, synthetic checks, etc.) YAML files (`dash0 apply -f <path>`) from GitHub Actions, and deleting by default assets removed since the last push.
(The delete behavior is opt-out.)

This action wraps `dash0 apply --since <ref>` and resolves `<ref>` from the triggering event on your behalf, handling [`github.event.before`'s corner cases](#github-event-corner-cases-this-action-handles) for you.

This action is standalone: it installs the Dash0 CLI automatically if it is not already on `PATH`.
If the [setup](../setup/README.md) action has already run, the existing installation and profile are reused.

> [!IMPORTANT]
> Your workflow's `actions/checkout` step must fetch enough history to include the comparison ref — `fetch-depth: 0` (full history) is the simplest way to guarantee that.
> A smaller depth works too, as long as it covers the ref, but for `since: auto` that ref is `github.event.before`, and the depth needed to reach it is unbounded and unknown in advance (however many commits the triggering push contained) — so `fetch-depth: 0` is the only value that's safe by default. A shallow checkout that doesn't reach the comparison ref makes it unresolvable, and this action fails the job with a message naming `fetch-depth` as the likely cause.

## Quick start

```yaml
steps:
  - uses: actions/checkout@v4
    with:
      fetch-depth: 0

  - name: Sync Dash0 assets
    uses: dash0hq/dash0-cli/.github/actions/sync-assets@main
    with:
      api-url: ${{ vars.DASH0_API_URL }}
      auth-token: ${{ secrets.DASH0_AUTH_TOKEN }}
      path: dashboards/
```

You can use any git ref: `@main`, `@v1.1.0`, or `@<commit-sha>`.

## Preview pending changes on a pull request

`dry-run: true` computes and reports the pending plan without applying it.
Combined with `since: none` this makes no Dash0 API calls at all, so it can run without credentials:

```yaml
- name: Preview pending sync
  uses: dash0hq/dash0-cli/.github/actions/sync-assets@main
  with:
    path: dashboards/
    dry-run: 'true'
    since: none
```

## GitHub event corner cases this action handles

Wiring `--since` from `github.event.before` by hand has several sharp edges.
This action handles all of them automatically:

- **First push to a new branch.** `github.event.before` is git's all-zeros SHA sentinel. Deletion detection is skipped for the run; the job does not fail.
- **No `before` at all.** `workflow_dispatch` and `schedule` triggers don't set `github.event.before`. Deletion detection is skipped the same way, unless you pass an explicit `since` value.
- **`pull_request` events.** `since: auto` never derives a comparison ref here, even on `synchronize` (which does set `before`). Deletion detection is skipped unless you pass an explicit `since` value.
- **Too-shallow checkout.** A comparison ref that resolves but isn't reachable in the checkout fails the job with an error naming `fetch-depth: 0` as the fix (see the note above).
- **A force-pushed or rewritten branch.** A comparison ref that resolves but is not an ancestor of the current commit fails the job by default, naming the likely cause. `accept-non-ancestor-ref: true` is the explicit opt-out.
- **A broken expression.** An explicit `since` value that is an empty string or the all-zeros sentinel fails the job immediately, rather than being treated like `auto` finding no prior state.
- **Unsafe interpolation.** Because this action reads `github.event.before` itself, your workflow never has to interpolate it into a `--since` flag by hand.

See `docs/github-actions-maintenance.md` for the rationale behind each of these.

## Inputs

### CLI installation

| Input | Required | Default | Description |
|-------|----------|---------|-------------|
| `cli-version` | No | latest | Dash0 CLI version to install (e.g., `1.17.0`). Minimum supported: `1.17.0` (this action relies on `apply --since` and its `--dry-run --agent-mode` JSON output). Ignored if the CLI is already on `PATH`. |

### Connection

| Input | Required | Default | Description |
|-------|----------|---------|-------------|
| `api-url` | Recommended | | Dash0 API URL. Overrides the active CLI profile. Find yours under [Endpoints](https://app.dash0.com/goto/settings/endpoints?endpoint_type=api_http). |
| `auth-token` | Recommended | | Dash0 auth token. Overrides the active CLI profile. Store it in a [GitHub secret](https://docs.github.com/en/actions/security-for-github-actions/security-guides/using-secrets-in-github-actions). |
| `dataset` | No | `default` | Dash0 dataset name. Overrides the active CLI profile. |

If the [setup](../setup/README.md) action already configured a profile with these values, you do not need to specify them again.
Connection inputs are not required for a `dry-run: true` invocation combined with `since: none`, since that combination makes no Dash0 API calls.

### Sync

| Input | Required | Default | Description |
|-------|----------|---------|-------------|
| `path` | **Yes** | | Path to a file or directory containing asset definitions to sync (mirrors `apply -f`). |
| `since` | No | `auto` | Controls deletion detection; has three options, see below. |
| `dry-run` | No | `false` | Compute and report the pending plan (see Outputs) without applying it. |
| `accept-non-ancestor-ref` | No | `false` | Accept a comparison ref that is not an ancestor of the current commit (e.g. after a force-push) instead of failing the job. |
| `max-deletions` | No | | Fail the job before any mutating call when the pending plan reports more deletions than this. Leave unset for no limit. |

#### The `since` input

`since` has three options:

- **`auto`** (default) — derive the comparison ref from `github.event.before` on a `push` event. Never derives one on `pull_request`, `workflow_dispatch`, or `schedule`.
- **`none`** — disable deletion detection, regardless of what the triggering event would otherwise supply; use this to guarantee a run never deletes anything, or on `workflow_dispatch`/`schedule` to run as a plain create/update sync.
- **any other value** — used directly as the comparison ref, replacing event-derived resolution entirely. Lets a `workflow_dispatch` or `schedule` run (which has no `before` to derive from) opt into deletion detection against a caller-chosen ref, e.g. a tag:

```yaml
- name: Sync since the last release
  uses: dash0hq/dash0-cli/.github/actions/sync-assets@main
  with:
    path: dashboards/
    since: v1.2.0
```

See [GitHub event corner cases this action handles](#github-event-corner-cases-this-action-handles) above for how `auto` behaves when there's no usable prior state, and for the resolvability/ancestry checks a comparison ref goes through before this action ever invokes `dash0` — full ref-resolution semantics are documented in [`apply --since`](https://github.com/dash0hq/dash0-cli/blob/main/docs/commands.md#apply---since-experimental).

## Outputs

| Output | Description |
|--------|-------------|
| `since` | The comparison ref actually used for deletion detection this run (empty when deletion detection did not apply). |
| `deletion-count` | Number of assets planned for deletion. |
| `deletions` | JSON array of assets planned for deletion, each `{"kind": ..., "name": ..., "originOrId": ...}`. |
| `modification-count` | Number of assets planned for creation or update. |
| `modifications` | JSON array of assets planned for creation or update, same shape as `deletions`. Populated even when deletion detection does not apply — only `deletions` is empty in that case. |

These outputs are sourced from a `dash0 apply --dry-run --agent-mode` preflight that this action always runs before any mutating call, so they are available whether or not `dry-run` is set:

```yaml
- name: Sync Dash0 assets
  id: sync
  uses: dash0hq/dash0-cli/.github/actions/sync-assets@main
  with:
    api-url: ${{ vars.DASH0_API_URL }}
    auth-token: ${{ secrets.DASH0_AUTH_TOKEN }}
    path: dashboards/

- name: Comment on planned deletions
  if: steps.sync.outputs.deletion-count != '0'
  run: |
    COUNT="${{ steps.sync.outputs.deletion-count }}"
    if [ "$COUNT" = "1" ]; then
      echo "Deleted 1 asset: ${{ steps.sync.outputs.deletions }}"
    else
      echo "Deleted $COUNT assets: ${{ steps.sync.outputs.deletions }}"
    fi
```

## Supported runners

- `ubuntu-latest` (x64)
- `ubuntu-24.04-arm` (arm64)
