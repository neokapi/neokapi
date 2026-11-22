---
title: push
sidebar_position: 5
---

# kapi push

Share your checkout's work with the team. A push sends the content, terms
edits, voice binding, decisions and the
[settings the recipe owns](/cli/project-model#settings-a-push-carries) to the
workspace, where the browser, Kapi Desktop and every other checkout read them.
`kapi push` only moves content and drafts nothing. With the project's
`bowrain.converge` policy at its default `on-push`, the server starts a run
when the push lands.

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
   computes a hash of each item's content, inline codes included, so a changed
   link counts as a changed block. If a file a collection covers cannot be read
   (it fails to parse, or it is a catalog in a schema this kapi does not read),
   the push stops before sending anything and names the file and the reason. A
   file that no format reads is left out of the tree, and the server keeps what
   it holds at that path.
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

Because the push declares the whole tree, the server also handles:

- **Deletions.** A path present in the last tree and absent from this one is
  retired on the server, on this stream.
- **Renames.** An item with the same content at a new path is a rename, so its
  targets, decisions and history move with it instead of starting over.
- **Collections and points.** Each item arrives with the collection and the
  coordinates the recipe resolved for it; the workspace's profile cards are
  derived from that declaration. The server refuses content at a point whose
  axis no recipe declares.

`.kapi/sync/sync-cache.json` records the last tree the client declared and the
ref it was committed as. It is gitignored and survives deleting `.kapi/work/`.
Deleting it loses an anonymous project's claim token. Otherwise the next push
declares the tree again, and the server answers with what it lacks.

## Decisions the server accepts

Push carries the project's committed decision record with the content it
applies to. The server decides review, so an approval sent this way follows
the same rules as one made in the web app. You need review permission for that
language in that project, and the workspace's
[separation of duties](/server/members-and-roles#governance-controls) applies
with you as the decider.

A push also carries how each translation was written, from the project's
record of its changes: the source a run made it from, and who wrote it.

- The server grades a translation a run on your machine produced against that
  source, as it grades one it drafted itself. If the source changed since, in
  its wording or an inline code such as a link, the dashboard shows it as
  stale, as your machine does (see
  [What an approval leaves behind](/server/review#what-an-approval-leaves-behind)).
- A translation you wrote by hand (with `kapi apply`, in Kapi Desktop, or
  through an agent working for you) counts as yours for separation of duties.
  When the workspace policy blocks approving your own work, the server refuses
  your approval of it, in the same push or a later one.
- A translation that a run produced, or that `kapi pull` or `kapi merge`
  brought in, names no author.

Each translation's record is sent once, and again when it changes.

The content lands either way. The push reports each verdict the server refused,
per language, and the translation stays awaiting review:

```
Pushed 47 blocks (12 uploaded), 512 words (scanned 12 files)
2 approvals not accepted for fr-FR: no review permission
1 approval not accepted for de-DE: separation of duties
3 local record(s) now match the platform; they will not be sent again
```

The last line shows the project's record updated to the server's answer, so
the next push does not send the same refused verdicts again.

Withdrawing an approval needs the same permission. A push that lowers a target
the server holds as approved, with the translation and its source unchanged,
withdraws the approval. The withdrawal applies only when you hold review
permission for that language. Otherwise the translation stays approved, the
push reports the demotion it did not apply, and kapi restores the project's
record to what the server holds:

```
1 demotion not accepted for nb-NO: withdrawing an approved translation needs review permission
1 local record(s) now match the platform; they will not be sent again
```

A rejection applies to the translation it names. If the server replaced that
translation after your working copy last pulled, the rejection leaves the
server's translation as it is, with its rung and its record. The push reports
the rejection it did not apply, and kapi replaces the project's record with the
server's. Pull before rejecting, so that the rejection names the translation on
the server.

```
1 demotion not accepted for nb: the rejection names a translation the platform no longer holds
1 local record(s) now match the platform; they will not be sent again
```

An edited translation lands at translated, as an edit made in the web app does.
Taking back an approval at reviewed is ordinary translation work and needs no
review permission.

`kapi up` prints the same lines after its push phase, on a connected project and
after `kapi up --local`. With `--json` it writes them as one `governance` record
in its NDJSON stream, with the field names `kapi push --json` uses:

```json
{"type":"governance","verdicts_refused":[{"locale":"nb","kind":"demotion","reason":"the rejection names a translation the platform no longer holds","count":1}],"verdicts_retired":1}
```

## Terms edits

When the project is claimed into a workspace and a baseline was pulled (see
[`kapi pull`](/cli/commands/pull)), push also reconciles local terms edits in
the bound terms store against that baseline. Ordinary edits, such as
definitions, notes, proposed terms and non-governed relations, apply directly
through the concept endpoints. Governed edits, such as a term set to
`forbidden` or `preferred`, an un-forbidding, a `replaced_by` relation, or a
concept delete, go into a single submitted change-set **proposal** for
review, with the same separation of duties the
[Context](/server/context#tiered-governance) hub enforces. Push reports what
applied directly and what it proposed, with a link to review the change-set.

## The voice binding

When the recipe binds a voice profile (`defaults.voice`, or a profile's own
`voice:`), push carries it into the
workspace [Context](/server/context) hub, matched by profile name: created on
first push, left alone when the content is unchanged, and otherwise applied as
a **new profile version**. The previous server-side version stays in the
version history, and vocabulary rules the server promoted from corrections are
kept. The output reports whether the voice was carried, skipped, or would be
pushed on a dry run.

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
