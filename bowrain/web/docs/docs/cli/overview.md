---
sidebar_position: 1
title: Overview
description: The kapi connector in depth. The project model, the command set, flows, and configuration for a developer or CI job working from a checkout.
keywords: [kapi, bowrain plugin, project sync, kapi.yaml, push, pull, up, flows, CI]
---

# The developer route

This section documents **kapi**, the connector for a developer's working
checkout. It is one of [several routes into a
workspace](/server/connectors); a content platform, a design tool, or a
repository connected server-side reach the same workspace without any of it. Use
this section when the source files live in a repository someone edits every day
and the results should land in that working tree.

For when to choose this route over a server-side repository connector, see
[the kapi connector](/server/connectors/kapi).

:::note
The Bowrain commands ship as the **`kapi-bowrain` plugin** for the `kapi` CLI;
there is no separate `bowrain` binary. Every command below is invoked as `kapi
<command>` (for example `kapi init`, `kapi push`). See
[Installation](/installation) to set it up.
:::

## The project model

A connected project is a kapi project whose recipe declares a `bowrain:` block:

- **`kapi.yaml`**: the recipe (committed): languages, content collections,
  flows, plugins, the voice binding, and the server connection
- **`.kapi/terms.json`, `.kapi/memory/memory.json`**: the context sources the recipe
  binds (committed)
- **`.kapi/state/*.jsonl`**: the unit-state record (committed)
- **`.kapi/flows/`**: optional file-per-flow definitions (committed)
- **`.kapi/work/store.db`**: the local index over all of the above (gitignored,
  rebuilt from them)
- **`.kapi/work/cache/sync-cache.json`**: sync state (gitignored, local only)

The CLI searches upward from the current directory, the way git finds a
repository root. See [Project model](/cli/project-model) for the full recipe
reference.

## Catching a project up

One verb brings every language up to date:

```bash
kapi up          # runs on the server: org keys, shared memory and vocabulary
kapi up --plan   # dry run: pending work, reuse from memory, and a cost estimate
```

On a connected project `kapi up` prints its resolved venue first (*server*),
pushes what drifted, streams the run's progress into the terminal, and pulls the
results down. What a machine cannot decide parks into the team's
[review session](/server/review). See [`kapi up`](/cli/commands/up) for flags and
venue resolution, and [Keeping content caught up](/the-loop) for what a run
does.

## Moving content without producing

`kapi push` and `kapi pull` are pure transport. Like `git push` and `git
fetch`, they move project state and never draft anything:

```bash
kapi status    # local changes, per-language coverage, server standing
kapi diff      # compare local against the server
kapi pull      # fetch teammates' and reviewers' work
kapi push      # send local changes up
```

Only changed blocks transfer; sync is content-addressed.

## Running one flow

For a specific composition (one named flow, one pass, no gate loop), define a
flow in `.kapi/flows/` (or inline on the recipe) and run it:

```bash
kapi run my-flow          # a custom flow
kapi run translate-qa     # a built-in composed flow
```

Ad-hoc file work stays available outside any project, for example
`kapi translate messages.json --target-lang fr` or
`kapi pseudo-translate src/locales/en.json`. See [Flows](/cli/flows/overview).

## Configuration

```bash
kapi config name                                                   # print a project setting
kapi config name "My App"                                          # set it
kapi config set bowrain.server.url https://app.bowrain.cloud       # set the default server
```

See [`kapi config`](/cli/commands/config).

## Next steps

- [kapi connector](/server/connectors/kapi): where this route sits among the others
- [Project model](/cli/project-model): the recipe reference
- [Commands](/cli/commands/init): the full command reference
- [Flows](/cli/flows/overview): composing and customizing runs
- [The loop in CI](/cli/ci/overview): pipelines and the ship gate
