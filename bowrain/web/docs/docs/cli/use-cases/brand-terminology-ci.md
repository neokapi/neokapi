---
title: Gate governed terms in CI
sidebar_label: Gate governed terms in CI
---

# Use case: gate governed terms in CI

A team's governed terms live in the [Context](/server/context) hub on the
Bowrain server: preferred terms, forbidden terms, the wording approved per
market. This guide wires those terms into a CI gate, so a pull request that
uses a banned term or the wrong rendering fails the build before it merges.

The loop is two commands:

1. [`kapi pull`](/cli/commands/pull) fetches results and, when the project
   is claimed into a workspace, also snapshots the workspace's governed concepts
   and their relations into the project's local terms store.
2. `kapi check --ship` runs the project's bound gates; the terms gate
   checks the project's target files against that terms store and exits non-zero
   when a file violates it.

Pull the truth once, then verify offline: no per-file server round-trip, and
the gate enforces exactly what the hub shows.

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
project's terms store has concepts in it, exactly the ones `kapi pull`
snapshotted, so the gate enforces what the hub shows with no extra
configuration.

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
      - uses: actions/checkout@v4

      - uses: neokapi/setup-kapi@v1
        with:
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
| `1`  | Operational error (project not found, unreadable file, …)             |

Exit `3` means "not on-spec yet", not a crash: read the findings and fix them.
Pass `--no-fail` to always exit `0` (report mode), useful inside an assistant
fix-loop that reads the findings from the output and re-runs; omit it for CI
gating, where the non-zero exit is the point.

Add `--json` to feed the structured findings to another tool:

```bash
kapi check --ship --json
```

## Keeping the snapshot fresh

`kapi pull` refreshes the local terms store on every run, so pulling at the start of
each CI job keeps the gate aligned with the current governed terms. When
the workspace changes a preferred or forbidden term, a
[governed edit](/server/context#tiered-governance) that travels through a
[change-set](/server/context#experiments-change-sets-and-pilots), the next CI run
pulls it and gates against it automatically.

## Related

- [kapi pull](/cli/commands/pull): fetches results and governed terms into the local terms store
- [Context](/server/context): where terms are governed
- [GitHub Actions](/cli/use-cases/github-actions): installing kapi in CI and CI authentication
- [Source language preparation](/cli/use-cases/source-prep): checks on source content in CI
