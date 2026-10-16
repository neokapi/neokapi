---
sidebar_position: 5
title: Context atlas
description: Implementation note for C-11. The kapi-context-atlas JSON document that `kapi context atlas --json` prints and the desktop's Explore view draws, field by field, with how the host builds each part and what it leaves out.
keywords: [context atlas, kapi context atlas, axes, history, events, content cells, JSON contract, implementation note]
---

# Context atlas

The context atlas is one read of everything a project's context holds, laid
out for exploring: each record at its point on the project's axes, with the
statuses it held over time, and each event on two clocks.
`host.App.ContextAtlas` builds it, `kapi context atlas --json` prints it, and
the desktop's Explore view draws it. The field names below are the contract
between them. See [C-11](/contribute/architecture/context/c-11-context-operations#the-atlas-reads-the-whole-context-at-once)
for where the atlas sits among the context operations, and
[C-02](/contribute/architecture/context/c-02-coordinates-and-governance#declared-axes-can-be-typed)
for the typed axes it lays records out on.

## Request

```go
type ContextAtlasRequest struct {
    Project     string // recipe path or workspace key; empty resolves as every command does
    AllProjects bool   // every project in the workspace; content cells and terms are omitted
    Content     bool   // include content cells (unit counts by collection, locale and state)
    Limit       int    // cap on records and on events; 0 is 5000
}
```

`kapi context atlas` asks for content unless `--no-content` or
`--all-projects` is given. A cap that leaves something out says so in
`notes`.

## Document

```go
type ContextAtlas struct {
    Format      string        `json:"format"`       // "kapi-context-atlas"
    Version     int           `json:"version"`      // 1
    Project     string        `json:"project,omitempty"`
    GeneratedAt time.Time     `json:"generated_at"`
    Span        AtlasSpan     `json:"span"`         // earliest..latest instant across records and events
    Axes        []AtlasAxis   `json:"axes"`
    Records     []AtlasRecord `json:"records"`
    Events      []AtlasEvent  `json:"events"`
    Notes       []string      `json:"notes"`        // reach, truncation, what was not read
}

type AtlasSpan struct {
    From time.Time `json:"from"`
    To   time.Time `json:"to"`
}
```

`axes`, `records`, `events` and `notes` are always arrays, never `null`.

### Axes

```go
type AtlasAxis struct {
    ID     string       `json:"id"`
    Label  string       `json:"label,omitempty"` // the recipe's label; empty leaves it to the view
    Kind   string       `json:"kind"`            // category | geo | range | ordered
    Source string       `json:"source"`          // structural | declared | derived
    Values []AtlasValue `json:"values"`          // display order
}

type AtlasValue struct {
    ID       string      `json:"id"`
    Label    string      `json:"label,omitempty"`
    Parent   string      `json:"parent,omitempty"`   // geo: the region directly containing it
    Centroid *[2]float64 `json:"centroid,omitempty"` // geo country: [lat, lon]
    Members  []AtlasPlace `json:"members,omitempty"` // geo group or region: the countries it holds
}

type AtlasPlace struct {
    ID       string     `json:"id"`
    Label    string     `json:"label,omitempty"`
    Centroid [2]float64 `json:"centroid"`
}
```

A geo group or region lists the countries it holds (`axis.Space.Contains`),
so the map can draw where a rule scoped to the Nordics holds. A country, World
and anything holding more than 80 countries list none.

| Source | Axes | Values |
| --- | --- | --- |
| `structural` | `product`, `channel` | the profiles and channels the recipe declares, and any a record uses |
| `declared` | every axis `axes:` types or a coordinate names, and any other axis a record carries | a closed or ordered axis lists its declared values in order, then any other used; a geo axis lists its groups, then regions, then countries by name; a range axis lists used intervals by lower bound |
| `derived` | `family`, `actor`, `project` (with `all_projects`), `locale`, `region` | the values the records use; `region` is a geo axis read from each locale's region subtag (`axis.RegionOfLocale`) |

The view computes four more axes from the records and never receives them:
the status at the window's start, the status at the snapshot, the lifetime
(upcoming, in force, expired or open at the snapshot) and the age since the
record was recorded.

### Records

```go
type AtlasRecord struct {
    ID          string               `json:"id"`
    Family      string               `json:"family"`  // rule | note | memory | term | content | import
    Label       string               `json:"label"`
    Weight      int                  `json:"weight"`  // 1, or the unit count of a content cell
    Coordinates map[string]string    `json:"coordinates"`
    Recorded    time.Time            `json:"recorded"` // zero when unknown
    ValidFrom   *time.Time           `json:"valid_from,omitempty"`
    ValidTo     *time.Time           `json:"valid_to,omitempty"`
    History     []AtlasState         `json:"history"`  // time order
    Short       string               `json:"short,omitempty"`
    Actor       *contextop.Actor     `json:"actor,omitempty"`
    Evidence    []contextop.Evidence `json:"evidence,omitempty"`
    Text        string               `json:"text,omitempty"`
    Scope       string               `json:"scope,omitempty"`
    ContestedBy []string             `json:"contested_by,omitempty"`
}

type AtlasState struct {
    At    time.Time `json:"at"`
    State string    `json:"state"`
}
```

The state of a record at an instant is the last `history` entry at or before
it; before the first entry the record is absent.

| Family | Source | `id` | States |
| --- | --- | --- | --- |
| `rule` | an operation carrying a term rule | the operation id | `suggested`, `established`, `contested`, `withdrawn`, `dropped`, `reset` |
| `note` | an operation carrying prose | the operation id | as above |
| `memory` | an operation carrying a content-memory pair | the operation id | as above |
| `term` | a concept in the terms store | `term:<concept>` (a concept id that starts `term:` is used as it is) | `established` |
| `content` | a cell of `kapi status` coverage | `content:<collection>:<locale>:<state>` | `not-started`, `draft`, `translated`, `established`, `stale` |

How each part is built:

- **Operations.** Every operation that carries a subject is a record, oldest
  first. Its family follows the subject (`rule` for a term rule, `memory` for
  a content-memory pair, `note` for prose), except an `import`, whose family
  is `import`: the file's rows arrive as term records, and the import is their
  provenance. Its history comes from `Ledger.Histories`: the status it was born
  with at its own instant, then each change at the instant of the operation
  that made it. Past `contextop.HistoryLimit` instants a history holds only the
  first status and the current one, and a note says so. A correction recorded
  without a rule carries no subject; it is an event and not a record.
- **Terms.** One record per concept, at the coordinates the concept is scoped
  to, with `product` set from its profile when its coordinates leave it out.
  The label is the head term and the forms it discourages, and the window is
  the first discouraged term's. A rule a person kept is a `rule` record (what
  the log decided) and also a `term` record (what the store holds), and the
  two are distinct identities.
- **Content cells.** The host measures the project with the same tally as
  `kapi status` (`ProjectCoverageTally`), one file at a time, and adds one
  record per collection, locale and state, weighted by its unit count. A stale
  unit is counted under `stale` and not under `draft`. The tally knows where
  units stand now and not when they got there, so each cell has one history
  entry at `generated_at`, and a note says earlier states are not known.

Every record's `coordinates` carries the point it holds at and the derived
axes: `family`, `actor` (a person, an agent or a tool) for an operation,
`project` when the atlas reads every project, and `locale` with `region` for a
content-memory pair, a term and a content cell.

### Events

```go
type AtlasEvent struct {
    ID        string           `json:"id"`
    At        time.Time        `json:"at"`                  // recorded clock
    Effective *time.Time       `json:"effective,omitempty"` // effective clock when it differs
    Kind      string           `json:"kind"`                // an operation kind | term-added | window-opens | window-closes
    Record    string           `json:"record,omitempty"`    // the record it bears on
    Actor     *contextop.Actor `json:"actor,omitempty"`
    Label     string           `json:"label,omitempty"`
}
```

Every context operation is an event at the instant the log accepted it, with
the record of the subject it acts on. A record with a window adds
`window-opens` and `window-closes` events, recorded at the instant the window
was set (the operation that recorded it, or the keep that gave it one) and
effective at the bound. A profile the recipe bounds in time adds the same two
kinds at its bounds, with no record and no recorded instant of its own. A
concept the terms store dates adds a `term-added` event at that date. Unit
decisions are not read, and a note says so.

## Example

Trimmed from `kapi context atlas --json --no-content` over a copy of
`samples/audience-context` whose recipe adds an `axes:` block typing a geo
`market` (default `nordics`) and an `age` range. One note was recorded with
`--point market=no --point age=14-16 --valid-to 2027-01-01`; the second record
and the axes the example does not need are left out.

```json
{
  "format": "kapi-context-atlas",
  "version": 1,
  "project": "audience-context",
  "generated_at": "2026-10-16T21:57:24.829275Z",
  "span": { "from": "2026-10-16T21:48:52.824911Z", "to": "2027-01-01T00:00:00Z" },
  "axes": [
    { "id": "market", "label": "Market", "kind": "geo", "source": "declared",
      "values": [
        { "id": "nordics" },
        { "id": "no", "label": "Norway", "parent": "northern-europe", "centroid": [60.47, 8.47] },
        { "id": "se", "label": "Sweden", "parent": "northern-europe", "centroid": [60.13, 18.64] }
      ] },
    { "id": "family", "kind": "category", "source": "derived",
      "values": [ { "id": "rule" }, { "id": "note" } ] }
  ],
  "records": [
    {
      "id": "0q7xbm6sfmjng90p1rnknsbc",
      "family": "note",
      "label": "the teen page invites a trusted adult to help book",
      "weight": 1,
      "coordinates": {
        "actor": "agent", "age": "14-16", "audience": "teen", "channel": "teen",
        "family": "note", "market": "no", "product": "harbor-help",
        "service": "harbor-help", "topic": "video-appointments"
      },
      "recorded": "2026-10-16T21:48:52.824911Z",
      "valid_to": "2027-01-01T00:00:00Z",
      "history": [ { "at": "2026-10-16T21:48:52.824911Z", "state": "suggested" } ],
      "short": "0q7xbm6sfm",
      "actor": { "kind": "agent", "name": "claude-code", "session": "s0ab4e399" },
      "evidence": [ { "path": "site/locales/en/teen.json", "quote": "ask a trusted adult to help you book" } ],
      "text": "the teen page invites a trusted adult to help book",
      "scope": "project age=14-16,audience=teen,channel=teen,market=no,product=harbor-help,service=harbor-help,topic=video-appointments until 2027-01-01"
    }
  ],
  "events": [
    { "id": "0q7xbm6sfmjng90p1rnknsbc", "at": "2026-10-16T21:48:52.824911Z", "kind": "observe",
      "record": "0q7xbm6sfmjng90p1rnknsbc", "label": "the teen page invites a trusted adult to help book" },
    { "id": "window-closes:0q7xbm6sfmjng90p1rnknsbc", "at": "2026-10-16T21:48:52.824911Z",
      "effective": "2027-01-01T00:00:00Z", "kind": "window-closes",
      "record": "0q7xbm6sfmjng90p1rnknsbc", "label": "the teen page invites a trusted adult to help book" }
  ],
  "notes": [
    "Read 2 context operations of this project.",
    "Unit decisions are not read into the atlas; `kapi status` reports them."
  ]
}
```

## Cost

The atlas reads the operation log once for the records and once more for the
histories, which fold the log at each distinct instant up to
`contextop.HistoryLimit`. The terms store is read whole, as `kapi context
search` reads it. Content cells read every file of the project once per
target language, as `kapi status` does, and keep counts, never blocks.
