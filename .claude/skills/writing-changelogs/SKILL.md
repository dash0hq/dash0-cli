---
name: writing-changelogs
description: Writes and reviews user-facing changelog entries and release notes for the Dash0 CLI. Use when creating or editing .chloggen YAML files, reviewing release-note wording in a pull request, or turning a code change into a changelog entry.
---

# Writing Dash0 CLI changelogs

A changelog is for someone using the CLI, not someone reviewing the implementation. Explain **what changed for them and why it matters**.

## Establish the change

- Read the relevant code diff, issue, and documentation. Identify what users could do before, what they can do now, and the practical benefit. Do not write from the PR title alone.
- Check `.chloggen/TEMPLATE.yaml`, `.chloggen/summary.tmpl`, and recent `CHANGELOG.md` entries for the current format. Create the file with `make chlog-new`, as `docs/changelog-maintenance.md` describes.
- If the change has no user-visible effect, recommend skipping the changelog instead of inventing a benefit.

## Write the entry

- Lead with the user-facing outcome: a problem resolved, a capability gained, or behavior changed. Make the benefit concrete rather than promotional.
- Make `note` a self-contained sentence. Name commands or flags when they help someone discover or use the feature.
- Use `subtext` only for **new** information users need: how to use it, an important limitation, changed defaults, compatibility, or migration guidance. Omit it if the note is sufficient. Write one sentence per line.
- Keep implementation, refactoring, test coverage, dependencies, UI polish, and internal consistency out of the release note unless they materially affect users.
- Prefer plain, specific wording. Avoid filler such as "enhances visibility," "improves the experience," "seamlessly," "empowers users," and descriptions of work performed ("added support for").
- Verify every claim against the diff. Don't imply guarantees, scope, or benefits the implementation doesn't establish.
- Preserve the file's metadata (`change_type`, `component`, `issues`, `change_logs`) unless correction is required. Produce valid YAML, following the repo's schema, and run `make chlog-validate`.

## Example

Implementation-focused:

```yaml
note: "`k8s pods watch` and the workload variants show an Errors chart next to Requests and Duration, like Dash0's service catalog"
subtext: |
  The Errors chart shows the percentage of requests that failed over the range, and says so when none failed.
  On a wide terminal the three charts sit side by side at the same height.
```

User-focused:

```yaml
note: "Spot error spikes in a pod or workload: `k8s pods watch` and the workload watch commands chart the percentage of failed requests next to request rate and latency"
subtext: |
  The chart plots the error rate over the time range, so you can see when failures started.
  It always covers all of the workload's pods, also with `--aggregate` or `--per-pod`.
```

The note leads with what the user can now do and still names the commands. The subtext drops card alignment and the empty-state message, which are UI polish, and keeps the two facts a user needs: the rate is plotted over time, and it ignores the aggregation flags.

## Final review

Check that someone unfamiliar with the PR can tell what changed and why to care. Remove details that only maintainers would appreciate. Look for redundant `subtext` and unsupported claims. When reviewing, propose the replacement and briefly explain the most important edit; change files only when asked.
