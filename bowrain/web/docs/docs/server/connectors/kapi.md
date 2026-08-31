---
title: kapi (developer & CI)
sidebar_label: kapi
sidebar_position: 5
description: The kapi connector, how a developer's checkout and a CI job reach a Bowrain workspace, when to choose it over the server-side repository connector, and what stays local.
keywords: [kapi, connector, developer, CI, kapi up, kapi push, kapi pull, checkout, plugin]
---

# kapi connector

kapi is the connector for a **working checkout**. Where the other connectors
reach into a system from the server, kapi runs where the developer is, a
terminal, an editor, a CI runner, and drives content into the workspace and
back out to the files on disk.

It is one route among [several](/server/connectors), and it is the right one
when the person who owns the content is the person who owns the repository.

## When to choose it

Reach for kapi when:

- Source content is files in a repository a developer edits every day, and the
  results should land in the working tree they already have open.
- A release should be **gated** on the project's checks and coverage with an
  explicit exit code a pipeline can act on.
- The work should be runnable on demand: before a commit, against a branch,
  with progress visible in the terminal.
- A local pass is wanted occasionally without the server: kapi holds the
  project's graph on its own, on files, with no account at all.

Reach for the [GitHub / GitLab connector](/server/connectors/github) instead
when the repository should be governed with no command and no pipeline: a push
webhook triggers the server, and the results arrive as a pull request. Reach for
a [content platform or design connector](/server/connectors) when the source
never lives in a repository in the first place.

The choices are not exclusive. A repository can deliver through GitHub while a
developer runs kapi locally against the same project.

## How it fits

kapi is a **source connector**: it pushes to and pulls from the workspace rather
than being reached into. The relationship is the one a checkout has with a
remote: you work locally, and push and pull to share.

The server-side connectors and kapi meet in the same project. Content that
arrived from a content platform and content that arrived from a checkout are
ordinary project content once inside: the same voice profile, the same terms,
the same content memory, the same review session.

## Setting it up

kapi is a separate CLI, and Bowrain support ships as a plugin for it:

```bash
brew install neokapi/tap/bowrain-cli
```

Every command runs as `kapi <command>`; there is no separate `bowrain` binary.
See [Installation](/installation) for the other platforms and for pinning a
plugin version.

Connect a repository to a workspace:

```bash
kapi init
```

The wizard signs you in and connects the project, writing a `kapi.yaml` recipe
at the project root and a sibling `.kapi/` directory. The recipe's
`bowrain:` block is what makes the project connected.

## Everyday use

```bash
kapi up          # catch the project up; runs on the server
kapi status      # per-scope coverage, ship standing, server standing, and venue
kapi push        # send local changes up (pure transport)
kapi pull        # fetch teammates' and reviewers' work (pure transport)
```

`kapi up` is the verb that catches a project up: it pushes what drifted, the
server runs on the organization's keys and shared memory, progress streams into
the terminal, and results land in the tree. What a machine cannot decide parks
into the team's [review session](/server/review).

The full command reference lives in the [CLI section](/cli/overview); the
mechanics of a run are in [Keeping content caught up](/the-loop).

## In CI

A CI job holds a server token and no AI provider keys: the run executes on the
server venue, so provider credentials never leave the workspace. The release
gate is a separate, opt-in step:

```bash
kapi check --ship
```

It answers one question, is this scope shippable, and exits **3** when the
gate is unmet (distinct from an operational error). Ordinary builds are never
failed by language drift; only this explicit gate is.

See [The loop in CI](/cli/ci/overview) for GitHub Actions, GitLab CI, and the
exit-code contract.

## What stays local

kapi owns the local files and the project configuration. The `kapi.yaml`
recipe, with its content collections, flows, plugins, languages, coordinates and
voice binding, is authored and versioned in the repository with everything else,
including the `bowrain:` block that names the workspace. Bowrain never writes it
on its own: when a person approves an axis a [context scan](/server/context-scan)
proposed, the approval arrives as a `kapi pull` that edits
`defaults.coordinates` in your working tree, for you to review and commit.

The reverse also holds: Bowrain's own clients do not source projects from a
filesystem. The desktop app is a working copy of the server, not a local project
tool; see [How Bowrain and kapi fit together](/getting-started/kapi-vs-bowrain).

## Related

- [Connectors](/server/connectors): the full connector row
- [GitHub / GitLab connector](/server/connectors/github): the same repository, without a CLI
- [CLI overview](/cli/overview): commands, flows, and the project model
- [The loop in CI](/cli/ci/overview): pipelines and the ship gate
