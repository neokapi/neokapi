---
sidebar_position: 5
title: Context atlas
description: Implementation note for C-11. The kapi-context-atlas JSON document that `kapi context atlas --json` prints and the desktop's Explore view draws, field by field, with how the host builds each part and what it leaves out.
keywords: [context atlas, kapi context atlas, axes, places, gazetteer, geometry, map, history, events, content cells, JSON contract, implementation note]
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
for the typed axes it lays records out on, and
[C-12](/contribute/architecture/context/c-12-places) for the places a geo axis
holds.

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
    Places      []AtlasPlace  `json:"places"`       // the gazetteer the map draws from
    Records     []AtlasRecord `json:"records"`
    Events      []AtlasEvent  `json:"events"`
    Notes       []string      `json:"notes"`        // reach, truncation, what was not read
}

type AtlasSpan struct {
    From time.Time `json:"from"`
    To   time.Time `json:"to"`
}
```

`axes`, `places`, `records`, `events` and `notes` are always arrays, never
`null`.

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
    ID    string `json:"id"`
    Label string `json:"label,omitempty"` // the recipe's label, or a place's name
}
```

A geo value is an id and a label. Where the place sits, what it is made of and
how to draw it are in [`places`](#places), under the same id.

| Source | Axes | Values |
| --- | --- | --- |
| `structural` | `product`, `channel` | the profiles and channels the recipe declares, and any a record uses |
| `declared` | every axis `axes:` types or a coordinate names, and any other axis a record carries | a closed or ordered axis lists its declared values in order, then any other used; a geo axis lists the places the records use and every declared place, places made of places first, then by type from the widest kind to the narrowest (`place.Type.Rank`), then by name; a range axis lists used intervals by lower bound |
| `derived` | `family`, `actor`, `project` (with `all_projects`), `locale`, `region` | the values the records use; `region` is a geo axis holding the place each locale names (`place.OfLocale`): the subdivision of a `-u-sd-` extension, else the country or M49 region of its region subtag |

The view computes four more axes from the records and never receives them:
the status at the window's start, the status at the snapshot, the lifetime
(upcoming, in force, expired or open at the snapshot) and the age since the
record was recorded.

### Places

```go
type AtlasPlace struct {
    ID       string            `json:"id"`
    Name     string            `json:"name"`
    Names    map[string]string `json:"names,omitempty"`    // a declared place's names per locale
    Type     string            `json:"type"`               // world | region | country | subdivision | a declared type
    Within   []string          `json:"within,omitempty"`
    Of       []string          `json:"of,omitempty"`
    Centroid *[2]float64       `json:"centroid,omitempty"` // [lat, lon], rounded to two decimals
    Refs     []string          `json:"refs,omitempty"`
    Shape    string            `json:"shape,omitempty"`    // a declared place's GeoJSON file
    Geometry *AtlasGeometry    `json:"geometry,omitempty"` // the polygon that file holds
    Declared bool              `json:"declared,omitempty"`
}

type AtlasGeometry struct {
    Type        string `json:"type"`        // Polygon | MultiPolygon
    Coordinates any    `json:"coordinates"` // [lon, lat] positions in rings, as the file writes them
}
```

`places` is the part of the project's gazetteer a map needs, sorted by id. The
host collects:

1. every place a record names on a geo axis, the derived `region` axis
   included;
2. every place that contains one of those (`Gazetteer.Ancestors`), and the
   members of each place made of places, recursively;
3. every place the recipe declares, with what contains it and what it is made
   of;
4. then, for each country and subdivision in that set, the places directly
   below it (`Gazetteer.Children`), and for each region, the countries it
   holds through its subregions.

The last step is what lets a map drill down by name: a map opened at Norway
lists every Norwegian county, with records or without.

`within` and `of` carry the declarations unchanged, so a view that applies the
containment rules of
[C-12](/contribute/architecture/context/c-12-places#containment-and-overlap)
to them reaches the engine's answers. `@neokapi/context-atlas` does that in
`model/places.ts`, held to the engine by the shared fixture
`core/place/testdata/containment.json`.

A declared place with `shape:` carries `geometry`: the host reads the file
from the checkout and reduces it to one Polygon or MultiPolygon geometry. It
accepts a bare geometry, a Feature holding one, or a FeatureCollection of a
single such Feature. A file the host cannot read, one larger than 1 MiB
(`atlasMaxShape`), and one holding anything else are left out, and a note names
the recipe key, such as `places.bergen.shape: places/bergen.geojson is larger
than 1 MiB, so the place has no outline.` A project read with no checkout on
this machine carries no geometry, and a note says the shape was not read.

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

Trimmed from `kapi context atlas --json --no-content` over a copy of the
KapiMart desktop sample, after `kapi store import context` and one note
recorded on the Oslo campaign's pick-up page with `--valid-to 2027-01-01`. The
copy gives Grünerløkka a `shape:` file. The terms, the import records, most
places and the axes the example does not need are left out.

```json
{
  "format": "kapi-context-atlas",
  "version": 1,
  "project": "KapiMart",
  "generated_at": "2026-10-17T11:27:48.992071Z",
  "span": { "from": "2026-08-30T20:51:47Z", "to": "2027-01-01T00:00:00Z" },
  "axes": [
    { "id": "market", "label": "Market", "kind": "geo", "source": "declared",
      "values": [
        { "id": "nordics", "label": "Nordics" },
        { "id": "greater-oslo", "label": "Greater Oslo" },
        { "id": "world", "label": "World" },
        { "id": "oslo", "label": "Oslo" },
        { "id": "grunerlokka", "label": "Grünerløkka" }
      ] }
  ],
  "places": [
    { "id": "europe", "name": "Europe", "type": "region", "within": ["world"] },
    { "id": "greater-oslo", "name": "Greater Oslo", "type": "metro", "of": ["no-03", "no-32"], "declared": true },
    { "id": "grunerlokka", "name": "Grünerløkka", "type": "neighbourhood", "within": ["oslo"],
      "shape": "places/grunerlokka.geojson",
      "geometry": { "type": "Polygon", "coordinates": [[[10.744, 59.918], [10.772, 59.918], [10.772, 59.934], [10.744, 59.934], [10.744, 59.918]]] },
      "declared": true },
    { "id": "no", "name": "Norway", "type": "country", "within": ["northern-europe"], "centroid": [60.47, 8.47] },
    { "id": "no-03", "name": "Oslo", "type": "subdivision", "within": ["no"] },
    { "id": "no-32", "name": "Akershus", "type": "subdivision", "within": ["no"] },
    { "id": "nordics", "name": "Nordics", "type": "territory", "of": ["no", "se", "dk", "fi", "is"], "declared": true },
    { "id": "northern-europe", "name": "Northern Europe", "type": "region", "within": ["europe"] },
    { "id": "oslo", "name": "Oslo", "type": "locality", "within": ["no-03"], "centroid": [59.91, 10.75],
      "refs": ["wikidata:Q585", "geonames:3143244"], "declared": true },
    { "id": "world", "name": "World", "type": "world" }
  ],
  "records": [
    {
      "id": "0q9c74dgngmdpc5ph1rd180r",
      "family": "note",
      "label": "The pick-up page names the street and the park nearest to it.",
      "weight": 1,
      "coordinates": {
        "actor": "person", "age": "13-17", "audience": "shoppers", "brand": "kapimart",
        "channel": "marketing", "family": "note", "lifecycle": "current",
        "market": "grunerlokka", "product": "kapimart"
      },
      "recorded": "2026-10-17T11:27:48.623464Z",
      "valid_to": "2027-01-01T00:00:00Z",
      "history": [ { "at": "2026-10-17T11:27:48.623464Z", "state": "suggested" } ],
      "short": "0q9c74dgng",
      "actor": { "kind": "person" },
      "evidence": [ { "path": "campaigns/oslo/en/grunerlokka.md", "quote": "Our newest pick-up point is on Thorvald Meyers gate" } ],
      "text": "The pick-up page names the street and the park nearest to it.",
      "scope": "project age=13-17,audience=shoppers,brand=kapimart,channel=marketing,lifecycle=current,market=grunerlokka,product=kapimart until 2027-01-01"
    }
  ],
  "events": [
    { "id": "0q9c74dgngmdpc5ph1rd180r", "at": "2026-10-17T11:27:48.623464Z", "kind": "observe",
      "record": "0q9c74dgngmdpc5ph1rd180r", "actor": { "kind": "person" },
      "label": "The pick-up page names the street and the park nearest to it." },
    { "id": "window-closes:0q9c74dgngmdpc5ph1rd180r", "at": "2026-10-17T11:27:48.623464Z",
      "effective": "2027-01-01T00:00:00Z", "kind": "window-closes",
      "record": "0q9c74dgngmdpc5ph1rd180r",
      "label": "The pick-up page names the street and the park nearest to it." }
  ],
  "notes": [
    "Read 5 context operations of this project.",
    "Unit decisions are not read into the atlas; `kapi status` reports them."
  ]
}
```

The note sits at `grunerlokka`, the item's own point, inside the campaign's
`oslo`. Its places run up to the world, and the full document also lists every
Norwegian county and every country of Europe, so the map can drill from the
world to the neighbourhood.

## Drawing the places

The map in `@neokapi/context-atlas` draws `places` over built-in outlines that
ship in the package under `src/geo/`:

| File | Holds | Keyed by |
| --- | --- | --- |
| `world.json` | every country, object `countries` | ISO 3166-1 alpha-2 in lower case (`no`) |
| `admin1/<cc>.json` | one country's subdivisions, object `subdivisions`, and its outline, object `outline` | ISO 3166-2 in lower case (`no-03`) |

Both are TopoJSON, simplified and quantised. The world file loads with the
map; a country's file loads as a chunk of its own when the map drills into
that country. The data is Natural Earth (public domain) at a pinned release:
the admin-0 countries and the admin-1 states and provinces. Where Natural
Earth files subdivisions under an ISO 3166-2 region, the region's shape is the
union of its parts and takes the region's id (`fr-ara`).

`scripts/build-geometry.mjs` regenerates the directory:

```bash
vp run @neokapi/context-atlas#build-geometry
```

It downloads the two Natural Earth files at the pinned release, checks their
SHA-256 digests, keys every feature by place id, simplifies each shape relative
to its size, and writes the TopoJSON. `--cache <dir>` reads files already
downloaded. Where Natural Earth carries a subdivision under a code ISO 3166-2
has since replaced, or only as the parts of a current one,
`scripts/geometry-successors.json` lists the current id, the features it is
composed from and the change it rests on, and the script unions the parts. A
row naming a code Natural Earth lacks, or an id it already carries, stops the
build. `src/geo/README.md` names the release, the licence and the ids that have
no outline.

A place with no outline in either file is drawn at its centroid when it has
one, and is otherwise listed beside the map. The outlines are for drawing:
what contains what comes from `within` and `of` alone.

## Cost

The atlas reads the operation log once for the records and once more for the
histories, which fold the log at each distinct instant up to
`contextop.HistoryLimit`. The terms store is read whole, as `kapi context
search` reads it. Content cells read every file of the project once per
target language, as `kapi status` does, and keep counts, never blocks.
