---
title: terms pull
sidebar_position: 14
---

# kapi terms pull

Snapshot the workspace [brand knowledge graph](/server/brand) into the project's
local termbase, so terminology gates run offline against the same governed truth
the [Brand](/server/brand) hub shows.

The workspace graph is the source of truth for terminology. `kapi terms pull`
fetches every governed concept — its terms across locales and its typed
relations — and writes them into the project's bound termbase, refreshing any
concept already present. After a pull,
[`kapi verify --terms`](/cli/use-cases/brand-terminology-ci) gates without a
network round-trip. This is the CI gating loop: pull the truth once, then verify
offline.

## Usage

```bash
kapi terms pull
```

The project must be claimed into a workspace and you must be authenticated — run
[`kapi auth login`](/cli/commands/auth), or set `BOWRAIN_AUTH_TOKEN` in CI.

```
Pulled 142 concept(s), 318 term(s), 27 relation(s) from the workspace knowledge graph
Workspace: acme
Termbase:  /Users/me/my-project/.kapi/termbase.db

'kapi verify --terms' now gates offline against the governed terminology.
```

Add `--json` for machine-readable output (concept, term, and relation counts plus
the termbase path).

## Where it writes

`kapi terms pull` writes to the project's bound termbase:

- the `defaults.termbase` path from the recipe, if set (resolved relative to the
  project root); otherwise
- the conventional `.kapi/termbase.db` under the project state directory.

It writes through the framework termbase API, so the snapshot is identical in
shape to any local termbase — the same database
[`kapi verify`](/cli/use-cases/brand-terminology-ci) and `kapi termbase` read.
Relations whose endpoints were not pulled are skipped, so the snapshot never
carries a dangling edge.

## Related

- [Gate brand terminology in CI](/cli/use-cases/brand-terminology-ci) — the pull → verify gating loop
- [Brand](/server/brand) — the graph this snapshots from
- [kapi concepts](/cli/commands/concepts) — browse the same governed concepts online
