---
title: concepts
sidebar_position: 12
---

# kapi concepts

Read the workspace [brand knowledge graph](/server/brand) from the command line.
`kapi concepts` is a read surface over the governed concepts the
[Brand](/server/brand) hub manages: each concept's terms and statuses, its typed
relations, and its story. Authoring happens in the hub (and, for governed edits,
through [change-sets](/cli/commands/experiments)); the CLI reads.

The command reads through the Bowrain server, so the project must be claimed into
a workspace and you must be authenticated — run [`kapi auth login`](/cli/commands/auth),
or set `BOWRAIN_AUTH_TOKEN` in CI.

Add `--json` to any subcommand for machine-readable output.

## kapi concepts list

List concepts in the workspace graph, narrowed by facets or a free-text query.

```bash
kapi concepts list
kapi concepts list --status forbidden
kapi concepts list --domain ui --market dach
kapi concepts list --q dashboard --limit 20
```

| Flag        | Description                                                            | Default |
| ----------- | --------------------------------------------------------------------- | ------- |
| `--status`  | Filter by term lifecycle status (`preferred`, `admitted`, `deprecated`, `forbidden`, …) | —       |
| `--domain`  | Filter by subject-field domain                                        | —       |
| `--market`  | Filter by [market](/server/brand#markets) validity tag               | —       |
| `--q`       | Free-text query against the term text                                 | —       |
| `--limit`   | Maximum number of concepts to return                                  | `50`    |

```
  CONCEPT      DOMAIN   TERMS
  -------      ------   -----
  c-dashboard  ui       Dashboard [en], Tableau de bord [fr], Cockpit [en]
  c-cockpit    ui       Cockpit [en]

2 concept(s)
```

## kapi concepts show

Show one concept in full: its terms grouped by locale with statuses, and its
typed relations in both directions.

```bash
kapi concepts show c-dashboard
```

```
Concept: c-dashboard
Domain:  ui
Definition: The product's main landing screen.

Terms:
  en
    Dashboard (preferred)
    Cockpit (forbidden)
  fr
    Tableau de bord (preferred)

Relations:
  REPLACED_BY <- c-cockpit — renamed in 2024

Updated: 2024-02-01T10:00:00Z
```

The relation vocabulary (`REPLACED_BY`, `USE_INSTEAD`, `BROADER`, `COMPETITOR`,
…) is the framework's SKOS-aligned label set; see
[Brand → the concept graph](/server/brand#the-concept-graph).

## kapi concepts story

Show a concept's merged timeline (oldest first): its revisions, the observations
recorded against it, its comment threads, and the change-sets that touched it.

```bash
kapi concepts story c-dashboard
```

```
Story of concept c-dashboard
  2024-01-01 10:00  [revision] alice — created
  2024-03-01 09:00  [changeset] bob — retire cockpit (x-1)
```

## Related

- [Brand](/server/brand) — the hub these concepts belong to
- [kapi experiments](/cli/commands/experiments) — inspect the change-sets that edit them
- [kapi terms pull](/cli/commands/terms-pull) — snapshot governed concepts into the local termbase
- [MCP](/cli/mcp) — the same reads as `concept_search` and `concept_story` tools for assistants
