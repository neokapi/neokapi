---
title: Gate governed terms in CI
sidebar_label: Gate governed terms in CI
---

import { PipelineDiagram } from "@neokapi/docs-shared";

# Use case: gate governed terms in CI

A pull request that uses a forbidden term or the wrong rendering fails the
build before it merges. The gate enforces the terms your team governs in the
[Context](/server/context) hub on the Bowrain server: preferred terms,
forbidden terms and the wording approved per market.

<PipelineDiagram
  channelLabel=""
  stages={[
    { label: "Context hub", sub: "governed terms", role: "annotate" },
    { label: "kapi pull", sub: "snapshot into the terms store", role: "io" },
    { label: "kapi check --ship", sub: "offline, per target file", role: "qa" },
    { label: "exit code", sub: "0 pass · 3 fail · 4 did not run" },
  ]}
/>

It takes two commands:

1. [`kapi pull`](/cli/commands/pull) fetches results and, when the project
   is claimed into a workspace, also snapshots the workspace's governed concepts
   and their relations into the project's local terms store.
2. `kapi check --ship` runs the project's bound gates; the terms gate
   checks the project's target files against that terms store and exits non-zero
   when a file violates it.

The job pulls once and then checks offline, without a server request per file.
The gate enforces exactly what the hub shows.

## Prerequisites

- The project is claimed into a workspace (its `kapi.yaml` recipe carries a
  [`bowrain:` block](/cli/project-model)).
- The project has run `kapi pull` at least once, so its terms store holds the
  governed concepts.
- The runner is authenticated. In CI, set `BOWRAIN_AUTH_TOKEN`; locally, run
  [`kapi auth login`](/cli/commands/auth).

## Locally

```bash
# 1. Pull results and governed terms from the workspace.
kapi pull

# 2. Gate the project against its bound gates, terms included.
kapi check --ship
```

`kapi check --ship` runs every gate the project binds (voice, terms,
checks) plus its ship/source coverage gates. The terms gate runs because the
project's terms store holds the concepts `kapi pull` snapshotted, so it needs
no extra configuration.

Scope the check to one locale with `--locale`, or point the terms gate at a
specific terms store with `--termstore`:

```bash
kapi check --ship --locale fr
```

## In GitHub Actions

Install kapi with [`setup-kapi`](/cli/use-cases/github-actions) (the bowrain
plugin is included by default), pull terms, then gate:

```yaml
name: Terms gate

on:
  pull_request:
    paths:
      - "src/locales/**"
      - "kapi.yaml"
      - ".kapi/**"

jobs:
  terms:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7

      - uses: neokapi/setup-kapi@v1
        with:
          version: "1.2.0"
          plugins: bowrain@1.2.0
          auth-token: ${{ secrets.BOWRAIN_AUTH_TOKEN }}
          server: https://dev.bowrain.cloud

      - name: Pull results and governed terms
        run: kapi pull

      - name: Gate against the project's bound gates
        run: kapi check --ship
```

The `auth-token` input exports `BOWRAIN_AUTH_TOKEN`, which `kapi pull` uses to
reach the workspace; the `server` input (exported as `BOWRAIN_SERVER_URL`) is
only needed for a self-hosted server, since the hosted service is the default.
A failing gate exits non-zero and fails the job.

## Exit codes

`kapi check --ship` returns a single exit code the CI runner gates on:

| Exit | Meaning                                                                 |
| ---- | ---------------------------------------------------------------------- |
| `0`  | Pass: every bound gate passed                                          |
| `3`  | A gate failed                                                          |
| `4`  | A gate did not run: it had no content to check, or a checker failed    |
| `1`  | Operational error (project not found, unreadable file, …)             |

Exit `3` means the content does not meet the gate yet: read the findings and
fix them. Pass `--no-fail` to exit `0` for a failed gate (report mode). That
suits an assistant fix-loop that reads the findings and re-runs. Leave it out
for CI gating, which uses the non-zero exit status to stop delivery. Exit `4`
is never a pass, and `--no-fail` leaves it unchanged. The result's
`did_not_run_cause` says whether a checker is broken or the gate had nothing
to check.

Add `--json` to feed the structured findings to another tool:

```bash
kapi check --ship --json
```

## Keeping the snapshot fresh

`kapi pull` refreshes the local terms store on every run, so pulling at the
start of each CI job keeps the gate in step with the governed terms. A change
to a preferred or forbidden term is a
[governed edit](/server/context#tiered-governance) that goes through a
[change-set](/server/context#experiments-change-sets-and-pilots). Once it
merges, the next CI run pulls it and gates against it.

## Related

- [kapi pull](/cli/commands/pull): fetches results and governed terms into the local terms store
- [Context](/server/context): where terms are governed
- [GitHub Actions](/cli/use-cases/github-actions): installing kapi in CI and CI authentication
- [Source language preparation](/cli/use-cases/source-prep): checks on source content in CI
