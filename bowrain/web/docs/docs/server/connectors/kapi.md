---
title: kapi (developer & CI)
sidebar_label: kapi
sidebar_position: 5
description: The kapi connector, how a developer's checkout and a CI job reach a Bowrain workspace, when to choose it over the server-side repository connector, and what stays local.
keywords: [kapi, connector, developer, CI, kapi up, kapi push, kapi pull, checkout, plugin]
---

# kapi connector

The kapi connector gives a **working checkout** and a CI job the same
workspace the browser shows: the team's decisions, the review queue, and runs
on the server that catch every language up. kapi runs where the developer
works, in a terminal, an editor or a CI runner. It moves content into the
workspace and back out to the files on disk.

Choose it among the [other connectors](/server/connectors) when the person who
owns the content also owns the repository.

## When to choose it

Use kapi when:

- Source content is files in a repository a developer edits every day, and the
  results should land in the working tree they already have open.
- A release should be **gated** on the project's checks and coverage with an
  explicit exit code a pipeline can act on.
- The work should be runnable on demand: before a commit, against a branch,
  with progress visible in the terminal.
- You sometimes want a local pass without the server. kapi holds the project's
  graph on its own, on files, with no account.

Use the [GitHub / GitLab connector](/server/connectors/github) when the
repository should stay caught up with no command and no pipeline: a push
webhook starts the server, and the results arrive as a pull request. Use a
[content platform or design connector](/server/connectors) when the source is
outside a repository.

You can combine them. A repository can deliver through GitHub while a
developer runs kapi locally against the same project.

## How it fits

kapi is a **source connector**: it pushes to and pulls from the workspace, as a
checkout does with a git remote. You work locally, and push and pull to share.

Content from a checkout and content from a server-side connector meet in the
same project. Once inside, both use the same voice profile, terms, content
memory and review session.

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

`kapi up` catches a project up. It pushes what drifted, and the server runs on
the organization's keys and shared memory. Progress streams into the terminal,
and the results land in the tree. Items a machine cannot decide wait in the
team's [review session](/server/review).

The full command reference lives in the [CLI section](/cli/overview); the
mechanics of a run are in [Keeping content caught up](/the-loop).

## In CI

A CI job holds a server token and no AI provider keys. The run executes on the
server venue, so provider credentials stay in the workspace. The release gate
is a separate step that you add:

```bash
kapi check --ship
```

It reports whether the scope is shippable, and exits **3** when the gate is
unmet, a code distinct from an operational error. An ordinary build passes when
target languages are behind; only this gate fails on them.

See [The loop in CI](/cli/ci/overview) for GitHub Actions, GitLab CI, and the
exit-code contract.

## What stays local

kapi owns the local files and the project configuration. The `kapi.yaml`
recipe, with its content collections, flows, plugins, languages, coordinates and
voice binding, lives in the repository and is versioned with everything else,
including the `bowrain:` block that names the workspace. Bowrain changes it
only through you. When a person approves an axis that a
[context scan](/server/context-scan) proposed, the next `kapi pull` edits
`defaults.coordinates` in your working tree, and you review and commit it.

Bowrain's own clients read projects from the server only. The Bowrain desktop
app is a working copy of the server; see
[How Bowrain and kapi fit together](/getting-started/kapi-vs-bowrain).

## Related

- [Connectors](/server/connectors): the full connector row
- [GitHub / GitLab connector](/server/connectors/github): the same repository, without a CLI
- [CLI overview](/cli/overview): commands, flows, and the project model
- [The loop in CI](/cli/ci/overview): pipelines and the ship gate
