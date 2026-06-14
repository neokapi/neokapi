---
title: Gate brand terminology in CI
sidebar_label: Gate brand terminology in CI
---

# Use case: gate brand terminology in CI

A team's governed terminology lives in the [Brand](/server/brand) hub on the
Bowrain server — preferred terms, forbidden terms, the wording approved per
market. This guide wires that governed terminology into a CI gate, so a pull
request that uses a banned term or the wrong translation fails the build before
it merges.

The loop is two commands:

1. [`kapi pull`](/cli/commands/pull) fetches translations and, when the project
   is claimed into a workspace, also snapshots the workspace's governed concepts
   and their relations into the project's local termbase (`.kapi/termbase.db`).
2. `kapi verify --terms` checks the project's target files against that termbase
   and exits non-zero when a file violates it.

Pull the truth once, then verify offline — no per-file server round-trip, and
the gate enforces exactly what the hub shows.

## Prerequisites

- The project is claimed into a workspace (its `*.kapi` recipe carries a
  [`server:` block](/cli/project-model)).
- The project binds a termbase — `defaults.termbase` in the recipe, or the
  conventional `.kapi/termbase.db`, which is where `kapi pull` writes the
  governed concepts.
- The runner is authenticated. In CI, set `BOWRAIN_AUTH_TOKEN`; locally, run
  [`kapi auth login`](/cli/commands/auth).

## Locally

```bash
# 1. Pull translations and governed terminology from the workspace.
kapi pull

# 2. Gate the project's target files against it.
kapi verify --terms
```

`kapi verify --terms` runs only the terminology gate. With no gate flag, `kapi
verify` runs every bound gate (brand, terminology, QA); naming `--terms`
restricts it to terminology — and, because the gate is requested explicitly, an
unbound termbase becomes a reported failure rather than a silent skip, so CI
cannot pass by doing nothing.

Scope the check to one locale with `--locale`, or point at a specific glossary
with `--termbase`:

```bash
kapi verify --terms --locale fr
```

## In GitHub Actions

Install kapi and the bowrain plugin with
[`setup-bowrain`](/cli/use-cases/github-actions), pull terminology, then verify:

```yaml
name: Brand terminology gate

on:
  pull_request:
    paths:
      - "src/locales/**"
      - "*.kapi"
      - ".kapi/**"

jobs:
  terminology:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: neokapi/setup-bowrain@v1
        with:
          token: ${{ secrets.NEOKAPI_REGISTRY_TOKEN }}
          auth-token: ${{ secrets.BOWRAIN_AUTH_TOKEN }}
          server: https://dev.bowrain.cloud

      - name: Pull translations and governed terminology
        run: kapi pull

      - name: Gate against governed terminology
        run: kapi verify --terms
```

The `auth-token` and `server` inputs export `BOWRAIN_AUTH_TOKEN` and
`BOWRAIN_SERVER_URL`, which `kapi pull` uses to reach the workspace. A failing
gate exits non-zero and fails the job.

## Exit codes

`kapi verify` returns a single exit code the CI runner gates on:

| Exit | Meaning                                                                 |
| ---- | ---------------------------------------------------------------------- |
| `0`  | Pass — every requested gate passed                                     |
| `3`  | A gate failed (including a requested gate whose binding is missing)    |
| `1`  | Operational error (project not found, unreadable file, …)             |

Exit `3` means "not on-spec yet", not a crash: read the findings and fix them.
Pass `--no-fail` to always exit `0` (report mode) — useful inside an assistant
fix-loop that reads the findings from the output and re-runs; omit it for CI
gating, where the non-zero exit is the point.

Add `--json` to feed the structured findings to another tool:

```bash
kapi verify --terms --json
```

## Keeping the snapshot fresh

`kapi pull` refreshes the local termbase on every run, so pulling at the start of
each CI job keeps the gate aligned with the current governed terminology. When
the workspace changes a preferred or forbidden term — a
[governed edit](/server/brand#tiered-governance) that travels through a
[change-set](/server/brand#experiments-change-sets-and-pilots) — the next CI run
pulls it and gates against it automatically.

## Related

- [kapi pull](/cli/commands/pull) — fetches translations and governed
  terminology into the local termbase
- [Brand](/server/brand) — where terminology is governed
- [GitHub Actions](/cli/use-cases/github-actions) — installing kapi in CI and CI authentication
- [Source language preparation](/cli/use-cases/source-prep) — QA gates on source content in CI
