# Maintaining the GitHub Actions

Repository-side notes on the two composite actions under `.github/actions/`.
For the user-facing action documentation, see [github-actions.md](github-actions.md) and the sibling [setup/README.md](../.github/actions/setup/README.md) and [send-log-event/README.md](../.github/actions/send-log-event/README.md).

## Keeping the actions in sync with CLI changes

When modifying the logic of `dash0 config`, ensure that the [setup](../.github/actions/setup/action.yaml) GitHub Action is not affected negatively.
Ensure that the constraints of `dash0 config profiles create` are enforced in the input validation of the setup GitHub action.

When modifying the flags of `dash0 logs send`, ensure that the [send-log-event](../.github/actions/send-log-event/action.yaml) GitHub Action inputs stay in sync.

## Testing the setup action

The workflow `.github/workflows/test-setup-action.yml` runs on every pull request and on every push to `main` (not just changes to the action) because CLI changes — especially to `dash0 config profiles create` — can break the action's profile-creation step.
Direct pushes to feature branches without an open PR do not trigger the workflow; open a (draft) PR to run it.
The workflow can also be triggered manually via `workflow_dispatch`.

The profile-creation tests mirror the parameter combinations tested in `TestCreateProfileCmdPartialFields` in `internal/config/config_cmd_test.go`.
Each combination is a separate job that asserts the correct fields are set and the omitted fields show `(not set)` (or `default` for dataset).
When adding or removing flags from `dash0 config profiles create`, update both the unit test and the workflow.

## Keeping the action READMEs in sync with the website

The two `README.md` files under `.github/actions/*/` are synced to `dash0.com/docs` as `Setup Dash0 CLI` and `Send Log Event` subpages under the `GitHub Actions` group.
Edits to either README also affect the website on the next release; conversely, treat the README as the source of truth and update it whenever the action's inputs, outputs, or behavior change.
See [`.github/workflows/sync-docs/transformations.yaml`](../.github/workflows/sync-docs/transformations.yaml) for the sync declarations.
