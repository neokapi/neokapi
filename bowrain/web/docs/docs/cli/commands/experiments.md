---
title: experiments
sidebar_position: 13
---

# kapi experiments

Inspect the [brand knowledge graph](/server/brand)'s **change-sets** — reviewable
drafts of edits to the graph and to brand vocabulary — from the command line. A
change-set moves through `draft → in_review → approved → merged` (or is
`abandoned`); before it merges you can preview its
[blast radius](/server/brand#experiments-change-sets-and-pilots) over stored
content.

`kapi experiments` is a read surface: it lists change-sets, shows a change-set's
operations, reviews, and pilots, and previews blast radius. Authoring and
approving change-sets happens in the [Brand](/server/brand) hub.

The command reads through the Bowrain server, so the project must be claimed into
a workspace and you must be authenticated — run [`kapi auth login`](/cli/commands/auth),
or set `BOWRAIN_AUTH_TOKEN` in CI. Add `--json` to any subcommand for
machine-readable output.

## kapi experiments list

List the workspace's change-sets, optionally filtered by status.

```bash
kapi experiments list
kapi experiments list --status in_review
```

| Flag       | Description                                                                  | Default |
| ---------- | ---------------------------------------------------------------------------- | ------- |
| `--status` | Filter by status (`draft`, `in_review`, `approved`, `merged`, `abandoned`)   | —       |

```
  ID   STATUS     CREATED           NAME
  --   ------     -------           ----
  x-1  in_review  2024-02-20 09:00  Retire cockpit

1 experiment(s)
```

## kapi experiments show

Show a change-set's operations, reviews, and pilots. A change-set marked
**governed** carries at least one governed operation and so needs an approval
from someone other than its author before it can merge (separation of duties).

```bash
kapi experiments show x-1
```

```
Experiment: x-1
Name:    Retire cockpit
Status:  in_review
Governed: yes (carries a governed operation)
Description: Forbid cockpit, prefer dashboard
Created by: alice (2024-02-20 09:00)
Submitted:  2024-03-01 09:00

Operations (2):
  1. term.status
  2. relation.add

Reviews (1):
  bob: approve — ship it

Pilots (1):
  proj-123 @ main
```

## kapi experiments blast-radius

Preview a change-set's blast radius without persisting anything. The report
counts how many stored `(block, locale)` rows the draft would newly flag or
resolve, with a source word count as a re-translation effort proxy, broken down
per project.

```bash
kapi experiments blast-radius x-1
```

```
Blast radius of experiment x-1
  Affected blocks: 7 of 100
  New violations:  5
  Resolved:        2
  Words affected:  120

Per project:
  PROJECT    BLOCKS      NEW RESOLVED    WORDS
  Web             7        5        2      120
```

## Related

- [Brand → experiments](/server/brand#experiments-change-sets-and-pilots) — the change-set lifecycle, blast radius, and pilots
- [Brand → tiered governance](/server/brand#tiered-governance) — ordinary vs. governed edits and separation of duties
- [kapi concepts](/cli/commands/concepts) — read the concepts a change-set edits
- [MCP](/cli/mcp) — the same reads as the `experiment_status` tool for assistants
