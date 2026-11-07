---
sidebar_position: 1
title: Overview
description: The kapi connector in depth. The project model, the command set, flows, and configuration for a developer or CI job working from a checkout.
keywords: [kapi, bowrain plugin, project sync, kapi.yaml, push, pull, up, flows, CI]
---

import { SwimlaneDiagram } from "@neokapi/docs-shared";

# The developer route

A checkout connected to a workspace reads the same context as the rest of the
team: the terms, voice profiles and decisions, the shared review queue, and
runs on the server that catch every language up. The connection is **kapi**
with the bowrain plugin, and the pages in this section document it.

kapi is one of [several routes into a workspace](/server/connectors). A content
platform, a design tool or a repository connected server-side reach the same
workspace without it. Use kapi when the source files live in a repository
someone edits every day and the results should land in that working tree.

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
- **the project's context store**: the terms, content memory, voice profiles and
  decisions, in a workspace under your data directory, shared by every checkout
- **`.kapi/`**: this checkout's cache (gitignored as a whole): `work/store.db`,
  its projection of the working tree, and `sync/`, the sync state

The CLI searches upward from the current directory, the way git finds a
repository root. See [Project model](/cli/project-model) for the full recipe
reference.

## Catching a project up

One verb brings every language up to date:

```bash
kapi up          # runs on the server: org keys, shared memory and vocabulary
kapi up --plan   # dry run: pending work, reuse from memory, and a cost estimate
```

<SwimlaneDiagram
  actors={[
    { label: "Your checkout", sub: "kapi up", role: "io" },
    { label: "Bowrain server", sub: "the run", role: "translate" },
    { label: "Reviewer", sub: "browser or desktop", role: "qa" },
  ]}
  messages={[
    { from: 0, to: 1, label: "push what drifted" },
    { from: 1, to: 1, label: "reuse, draft, check", detail: "progress streams to the terminal" },
    { from: 1, to: 0, label: "pull the results" },
    { from: 1, to: 2, label: "hold what needs a person", detail: "the review session" },
  ]}
  caption="kapi up on a connected project: the server runs the work, the checkout receives the results, and a person reviews what a machine cannot decide."
/>

On a connected project, `kapi up` first prints its resolved venue (*server*).
See [`kapi up`](/cli/commands/up) for flags and venue resolution, and
[Keeping content caught up](/the-loop) for what a run does.

## Moving content without producing

`kapi push` and `kapi pull` only move content. Like `git push` and `git
fetch`, they transfer project state and draft nothing:

```bash
kapi status    # local changes, per-language coverage, server standing
kapi diff      # compare local against the server
kapi pull      # fetch teammates' and reviewers' work
kapi push      # send local changes up
```

Only changed blocks transfer; sync is content-addressed.

## Running one flow

To run one named flow in one pass, without the gate loop, define the flow
inline in the recipe or in a file in the directory the recipe names with
`flows_dir:`, and run it:

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
