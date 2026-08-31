---
title: push
sidebar_position: 5
---

# kapi push

Send local changes to Bowrain Server. `kapi push` is pure transport: it moves
project state, the content, the terms edits, the voice binding, and never
drafts anything. With the project's `bowrain.converge` policy at its default
`on-push`, the server starts a run of its own when the push lands.

## Usage

```bash
kapi push [paths...] [flags]
```

## Examples

```bash
# Push all local changes to the server
kapi push

# Push specific files
kapi push src/locales/en/

# Show what would be pushed without uploading
kapi push --dry-run

# Re-upload every block, ignoring what the server already holds
kapi push --force

# Push only local terms edits (no content, no automations)
kapi push --concepts

# Example output:
# Pushed 47 blocks (scanned 12 files)
```

## Options

| Flag         | Description                                                                 | Default |
| ------------ | --------------------------------------------------------------------------- | ------- |
| `--force`    | Re-upload everything, even unchanged blocks                                 | `false` |
| `--dry-run`  | Show what would be uploaded without sending                                 | `false` |
| `--stream`   | Target stream (default: auto-detected from git/CI)                          | `$auto` |
| `--concepts` | Sync only local terms edits to the workspace (direct edits + governed change-set); no content transport, no automations | `false` |

## The protocol

A push declares its tree and uploads only what the server lacks. The same
protocol serves `kapi push`, `kapi up`, and the server-side connectors, and
[`kapi pull`](/cli/commands/pull) reads the same tree back.

1. **Scan.** kapi reads the recipe's `collections:`, extracts every block, and
   computes each item's content hash.
2. **Declare the tree.** The client sends the server its whole tree: every
   tracked item, its path, its collection and the point it sits at, and its
   hash. The server diffs that against the tree it holds and answers with the
   items it needs.
3. **Upload.** Only the items the server asked for travel, in chunks, to
   presigned upload URLs (or through the server when the blob store is not
   reachable from the client). Media assets referenced by the content travel
   the same way.
4. **Commit.** The client commits a manifest naming what it uploaded. The
   server validates it, answers `202`, and a worker applies the push: content is
   stored, and the output's `ingest` field says whether the push was applied
   or queued when the command returned.

Because the tree is declared whole, the server reconciles what a diff alone
could not:

- **Deletions.** A path present in the last tree and absent from this one is
  retired on the server, on this stream.
- **Renames.** An item with the same content at a new path is a rename, so its
  targets, decisions and history move with it instead of starting over.
- **Collections and points.** Each item arrives with the collection and the
  coordinates the recipe resolved for it; the workspace's profile cards are
  derived from that declaration. The server refuses content at a point whose
  axis no recipe declares.

`.kapi/work/cache/sync-cache.json` records the last tree the client declared
and the ref it was committed as. It is gitignored and safe to delete: the next
push declares the tree again and the server tells it what it lacks.

## Terms edits

When the project is claimed into a workspace and a baseline was pulled (see
[`kapi pull`](/cli/commands/pull)), push also reconciles local terms edits in
the bound terms store against that baseline. Ordinary edits, such as
definitions, notes, proposed terms and non-governed relations, apply directly
through the concept endpoints. Governed edits, such as a term set to
`forbidden` or `preferred`, an un-forbidding, a `replaced_by` relation, or a
concept delete, are bundled into a single submitted change-set **proposal** for
review, the same separation of duties the
[Context](/server/context#tiered-governance) hub enforces. Push reports what
applied directly versus what was proposed, with a link to review the
change-set.

## The voice binding

When the recipe binds a voice profile (`defaults.voice`, conventionally
`.kapi/voice.yaml`, or a profile's own `voice:`), push carries it into the
workspace [Context](/server/context) hub, matched by profile name: created on
first push, a no-op when the content is unchanged, and otherwise applied as a
**new profile version**. The previous server-side state is archived in the
version history, never overwritten, and vocabulary rules the server promoted
from corrections are preserved. The output reports whether the voice was
carried, skipped, or would be pushed on a dry run.

## Exit codes

- `0`: success (changes pushed or already up to date)
- `1`: error (server rejected, network error, and so on)

## Related commands

- [`kapi pull`](/cli/commands/pull): fetch the server's changes into the tree
- [`kapi status`](/cli/commands/status): coverage, ship standing, and what is pending
- [`kapi diff`](/cli/commands/diff): the changed blocks, per file
- [`kapi up`](/cli/commands/up): push, run on the server, pull

## When to use

Push to Bowrain Server to:

- **Share your work** with your team, and let the server catch the project up
- **Send terms edits**: ordinary edits land directly; governed edits travel up
  as a [change-set](/server/context#experiments-change-sets-and-pilots)
  proposal
- **Carry the voice profile** the recipe binds
- **Integrate with CI/CD** pipelines, where `kapi up` is usually the better
  verb because it also waits for the run and pulls the results

## Best practices

1. **Run `kapi status`** before pushing to see what changed
2. **Pull first** if working with a team, so the tree you declare is current
3. **Use `--dry-run`** when unsure about what will be uploaded
