---
title: pull
sidebar_position: 4
---

# kapi pull

Fetch the server's changes into the working tree. `kapi pull` is pure
transport: it moves the results of runs and review, the workspace's governed
terms, and the recipe changes an approval decided on, and never drafts
anything.

## Usage

```bash
kapi pull [flags]
```

## Examples

```bash
# Pull everything that changed on the server
kapi pull

# Pull only French results
kapi pull --locale fr

# Pull several locales
kapi pull --locale fr --locale de

# Show what would be pulled without changing files
kapi pull --dry-run

# Re-download everything, unchanged blocks included
kapi pull --force

# Refresh only the workspace's governed terms into the local terms store
kapi pull --concepts

# Example output:
# Pulled 12 blocks for 2 locales
# Updated 4 file(s)
```

## Options

| Flag         | Description                                                                 | Default |
| ------------ | --------------------------------------------------------------------------- | ------- |
| `--locale`   | Target locales to pull (repeatable)                                         | all     |
| `--force`    | Re-download everything, ignoring what the tree already holds                | `false` |
| `--dry-run`  | Show what would be pulled without changing files                            | `false` |
| `--concepts` | Sync only the workspace terms (concepts + relations) into the local terms store; no content transport, no automations | `false` |

## What pull writes

A pull reads the server's tree and the changes since the ref the client last
committed or pulled (the protocol is described once, on
[`kapi push`](/cli/commands/push)), and writes several things into the tree:

- **Recipe changes**, first. When a person approved an axis a
  [context scan](/server/context-scan) proposed, the approval is waiting as a
  pending recipe change. Pull writes it into `kapi.yaml` as
  `defaults.coordinates.<axis>` and settles it with the server. A value the
  recipe already holds is taken as applied; a value that disagrees with what
  the recipe says is reported as a conflict and left for you to resolve. The
  recipe is a decision waiting to be reviewed in git, so a failure here never
  fails the pull.
- **Content.** Targets produced by runs and promoted by review land in the
  target files the recipe's `collections:` name, with the format's own writer.
  A file whose targets did not change is left untouched.
- **Decisions.** Review decisions made on the server are staged into the
  project's working store; `kapi commit` publishes them into the committed
  record under `.kapi/state/`.
- **Governed terms.** When the project is claimed into a workspace, pull
  snapshots the workspace's concepts and their relations into the project's
  terms store and records a baseline, so a later
  [`kapi push`](/cli/commands/push) can diff local terms edits against it and
  [`kapi check --ship`](/cli/use-cases/brand-terminology-ci) gates offline
  against the same governed vocabulary.

Pull also reports what it deliberately did not apply:

- **Retired items.** An item the server still streams whose source is gone
  from this checkout is skipped rather than resurrected.
- **Governance divergence.** Pull reports the collections the server holds and
  how it governs them. For a collection this recipe declares, `kapi.yaml` is
  the authority, so a point, channel or voice that differs on the server is
  reported and resolved in git rather than pulled down over the local
  governance.

## Exit codes

- `0`: success (changes pulled or already up to date)
- `1`: error (server unavailable, auth failed, and so on)

## Related commands

- [`kapi push`](/cli/commands/push): the protocol, and what a push declares
- [`kapi status`](/cli/commands/status): coverage, ship standing, and what is pending
- [`kapi diff`](/cli/commands/diff): the changed blocks, per file
- [`kapi up`](/cli/commands/up): push, run on the server, pull

## When to use

Pull from Bowrain Server to:

- **Fetch results** of runs and review completed by teammates
- **Take an approved axis** into the recipe, for review in git
- **Sync governed terms**, so `kapi check --ship` gates offline
- **Update source content** that entered Bowrain through another connector

Source content can originate from a server-side connector, a content platform,
a design tool, or a git host, not only from your local files. `kapi pull`
brings those upstream changes down, so the checkout is the local mirror of
content that may have entered Bowrain elsewhere.
