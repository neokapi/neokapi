---
title: status
sidebar_position: 2
---

# kapi status

Show where the project stands: per-scope coverage against the ship gates,
what is staged locally, and, on a connected project, the server delta and the
venue `kapi up` would run on. `kapi status` always exits `0`; a locale that is
behind is pending work, not an error.

## Usage

```bash
kapi status [flags]
```

## What it shows

### The coverage grid

One row per scope, a locale or a `locale/collection` pair, with the number of
units, one column per rung of the target ladder (the share of units that have
reached drafting, translation, review), a pipeline bar showing distance to the
bar, and a **ship** column. Ship is a verdict rather than a percentage: it
reads `ready` when the scope clears its gate and `blocked: <rung>` naming the
first unmet gate, so it points at the work. Units an autonomous AI approved
are counted under review and qualified as such, because gates only count them
under an `any` approver class.

Under the grid, two basis lines report what a percentage cannot: how many
units carry a decision made against content that has since changed, and how
many have never been judged. A project that names no target language reports
itself as monolingual rather than as empty.

### Staged decisions

Decisions recorded locally but not yet written to the committed record under
`.kapi/state/` are counted as staged; `kapi commit` publishes them. A clean
project prints nothing here.

### The server section

On a connected project (the recipe declares a `bowrain:` block), the plugin
adds the server's standing: the server URL and project, blocks pending push and
pending pull against the tree the project last declared (see
[`kapi push`](/cli/commands/push)), the last sync time, any run in flight, and
two more lines:

- **terms**: when the workspace's governed terms were last snapshotted into the
  local terms store (concept and relation counts), or `never synced` when no
  concept pull has run; refresh with `kapi pull --concepts`.
- **governance**: where this project's declared points, channels and voice
  stand against the workspace's, so a divergence is visible before a push
  raises it.

### The venue

Where `kapi up` would run the loop, `server` or `local`, with the recipe's
`bowrain.converge` policy, and a note when the venue is degraded (a server is
declared but the bowrain plugin is missing).

## Options

| Flag             | Description                                                                 |
| ---------------- | --------------------------------------------------------------------------- |
| `--locale`       | Limit the grid to a single target locale                                    |
| `--source-lang`  | Source language (overrides the project's `source_language`)                 |
| `--review`       | List translated units not yet approved (the review worklist) instead of the grid; approve one with `kapi apply` |
| `--ship`         | Emit the minimal `ship.json` manifest (locale → shippable, verified) instead of the grid: the shape a language picker consumes to hide locales that are not shippable and badge those shipped on machine review |
| `--emit <path>`  | With `--ship`, write the manifest to this path instead of stdout            |
| `--json`         | Output the structured result as JSON                                        |

`kapi status --ship --emit ship.json` at build time writes the same manifest
the server publishes at `GET /api/v1/projects/:id/ship.json`, so a site built
from a checkout and a site reading the server agree. See
[Ship states](/server/review#ship-states).

## Examples

```bash
# The coverage grid, the server delta and the venue
kapi status

# One locale
kapi status --locale fr

# The review worklist
kapi status --review

# The ship manifest for a language picker, written at build time
kapi status --ship --emit public/ship.json

# Machine-readable
kapi status --json
```

## Exit codes

- `0`: status displayed (behind is pending work, not an error)
- `1`: error (project not found, and so on)

## Related commands

- [`kapi diff`](/cli/commands/diff): the changed blocks, per file
- [`kapi pull`](/cli/commands/pull): fetch the server's changes
- [`kapi push`](/cli/commands/push): send local changes, and the tree the delta is measured against
- [`kapi up`](/cli/commands/up): catch the project up
- [`kapi check --ship`](/cli/ci/overview): the gate that fails a build

## When to use

Run `kapi status` to:

- **Check before push** to see what will be uploaded
- **Check after pull** to see what changed
- **See what is shippable** before a release, and what blocks the rest
- **Troubleshoot** sync issues or unexpected state
