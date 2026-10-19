---
id: c-12-places
sidebar_position: 12
title: "C-12: Places"
description: "Architecture decision: a geo axis holds place ids from the project's gazetteer: the built-in world, UN M49 regions, ISO 3166-1 countries and ISO 3166-2 subdivisions, and the places a recipe declares. Each place sits within places or is made of places. Containment and overlap are computed from those declarations only, by the same rules in Go and in TypeScript. A place's shape is used only to draw it on the map."
keywords: [places, gazetteer, geo axis, market, containment, overlap, ISO 3166-2, UN M49, CLDR, Natural Earth, made of, within, kapi context places, architecture decision, neokapi]
---

import { PlaceGraphDiagram } from "@neokapi/docs-shared";

# C-12: Places

## Summary

A geo axis ([C-02](c-02-coordinates-and-governance.md#declared-axes-can-be-typed))
holds **place ids** from the project's **gazetteer**. The gazetteer is the
built-in places, which are the world, the UN M49 regions, every ISO 3166-1
country and every ISO 3166-2 subdivision, plus the places the recipe declares
under `places:`: a city, a neighbourhood, a metropolitan area, a sales
territory. Each place **sits within** one or more places or **is made of**
places. Containment is computed from these two declarations only. A rule
scoped to Norway applies to content at Oslo because the recipe declares Oslo
within the county `no-03`, and the built-in data puts `no-03` within Norway.

`core/place` holds the gazetteer and the containment rules over it. The recipe
loader, the axis space, the operation log, the terms store, the host and the
atlas all call it. The map in `@neokapi/context-atlas` applies the same rules
to the place graph in the atlas document, and a conformance fixture that the Go
and TypeScript tests both run keeps the two implementations in agreement. A
place's shape is used only for drawing. The map draws built-in places from
Natural Earth outlines, and a declared place from the GeoJSON file it names.

## Context

The markets a project writes for often span several administrative levels. A
campaign can run in one city and one of its neighbourhoods. A sales territory
can cover five countries, and a delivery area can cover a metropolitan area
across two counties. A rule recorded for one of these places has to apply to
content written for the places inside it. Two rules whose places overlap have
to be detected as overlapping.

Containment could be computed from geometry, by testing whether one outline
lies inside another. The result would then depend on the boundary data, its
version and simplification, a geometry library, and the presence of the files.
Declared containment gives the same result on every machine, offline, for the
cost of a map lookup. Every change to it appears in the recipe's diff, where a
reviewer sees it beside the content it affects.

## Decision

<PlaceGraphDiagram
  chain={[
    { id: "world", name: "World", type: "world" },
    { id: "europe", name: "Europe", type: "region" },
    { id: "northern-europe", name: "Northern Europe", type: "region" },
    { id: "no", name: "Norway", type: "country" },
    { id: "no-03", name: "Oslo", type: "subdivision" },
    { id: "oslo", name: "Oslo", type: "locality", declared: true },
    { id: "grunerlokka", name: "Grünerløkka", type: "neighbourhood", declared: true },
  ]}
  siblings={{
    no: [
      { id: "se", name: "Sweden", type: "country" },
      { id: "dk", name: "Denmark", type: "country" },
      { id: "fi", name: "Finland", type: "country" },
      { id: "is", name: "Iceland", type: "country" },
    ],
    "no-03": [{ id: "no-32", name: "Akershus", type: "subdivision" }],
  }}
  madeOf={[
    { id: "nordics", name: "Nordics", type: "territory", of: ["no", "se", "dk", "fi", "is"], declared: true },
    { id: "greater-oslo", name: "Greater Oslo", type: "metro", of: ["no-03", "no-32"], declared: true },
  ]}
  caption="The places of the KapiMart sample. Built-in places form a tree under the world; a declared place sits within places or is made of them."
/>

### A place

```go
type Place struct {
    ID       string            // a slug, matched byte for byte: "oslo", "no-03", "no", "northern-europe", "world"
    Name     string            // English display name, from CLDR for a built-in place
    Names    map[string]string // display names per BCP 47 locale, for a declared place
    Type     Type
    Within   []string          // the places it sits inside
    Of       []string          // the places it is made of
    Refs     []string          // ids in other gazetteers: "wikidata:Q585", "geonames:3143244"
    Centroid *LatLon           // [lat, lon], for drawing
    Shape    string            // a declared place's GeoJSON file, project-relative
    Declared bool
}
```

A place sets exactly one of `within:` and `of:`. `Type` is a named kind of
place rather than a numbered level, because administrative levels differ
between countries:

| Type | Holds |
| --- | --- |
| `world` | the one place `world` |
| `region` | a UN M49 region, subregion or intermediate region |
| `country` | an ISO 3166-1 country or territory |
| `subdivision` | an ISO 3166-2 subdivision, built in, or declared for a municipality ISO 3166-2 does not list |
| `territory` | a market or sales territory, usually made of places; the default for a declared place with `of:` |
| `metro` | a metropolitan area, usually made of places |
| `locality` | a city, town or village |
| `district` | a borough or city district |
| `neighbourhood` | a neighbourhood within a locality or district |
| `postal-area` | the area of a postal code |
| `area` | any other area; the default for a declared place with `within:` |

`Type.Rank` orders the types as the table lists them, widest first. Place
lists sort by it, so a country comes before its subdivisions.

Names are for display. Matching compares ids only, so renaming a place changes
no content and no rule.

### The built-in gazetteer

`place.Builtin()` is shared and immutable:

- `world`, which contains every place.
- The UN M49 regions, subregions and intermediate regions, by slug
  (`northern-europe`, `sub-saharan-africa`, `latin-america-and-the-caribbean`),
  each within the region above it, a continent within the world.
- Every ISO 3166-1 country and territory, by its alpha-2 code in lower case,
  with its English short name, an approximate centroid, and the M49 region it
  sits in: the intermediate region where M49 names one (`ke` within
  `eastern-africa`), else the subregion. Antarctica sits directly within the
  world.
- Every ISO 3166-2 subdivision, by its code in lower case with the hyphen
  (`no-03`, `us-ca`, `gb-sct`), with its English name and the place it sits
  in, including second levels: `fr-69m` sits within the region `fr-ara`, which
  sits within `fr`.

The subdivisions come from **Unicode CLDR** at a pinned release
(`release-48-2`): `common/validity/subdivision.xml` for the ids CLDR marks
regular, `common/supplemental/subdivisions.xml` for containment, and
`common/subdivisions/en.xml` for the names. ISO 3166-2 changes that the pinned
release does not record yet are listed in `core/place/internal/gen/amendments.tsv`,
one line per added or retired id, each naming its source. The generator
(`go generate ./core/place`) downloads the three files, applies the amendments,
and writes `core/place/subdivisions.tsv`, which is committed and embedded.
The generator fails on an amendment that the pinned release already contains,
so a release bump also removes that amendment line. The CLDR data is under the
Unicode License v3, which ships beside it as `core/place/UNICODE-LICENSE.txt`.
The file has that name because a file named `LICENSE` marks a licence boundary
in this repository.

Every built-in place except the world sits within exactly one place, so the
built-in places form a tree. A test checks the data: the file parses,
ids are unique, every parent resolves, and no subdivision id collides with a
region slug or a country code.

### Declared places

A recipe adds places under the top-level `places:` key, beside `axes:`:

```yaml
places:
  oslo:
    name: Oslo
    type: locality
    within: no-03                     # one id or a list
    centroid: [59.91, 10.75]
    refs: [wikidata:Q585, geonames:3143244]
  grunerlokka:
    name: Grünerløkka
    type: neighbourhood
    within: oslo
  nordics:
    name: Nordics
    of: [no, se, dk, fi, is]          # type defaults to territory
  greater-oslo:
    name: Greater Oslo
    type: metro
    of: [no-03, no-32]

axes:
  market:
    kind: geo
    label: Market
```

`place.New(declared)` builds the project's gazetteer on top of the built-in
one and validates every declared place. Each error names the recipe key
(`places.<id>...`). The rules are:

- the id is a slug that no built-in place uses;
- the place sets `within:` or `of:`, and not both;
- every id it lists is a place;
- no chain of `within:` and `of:` leads from a place back to itself;
- the type is one a recipe may declare;
- `names` are keyed by locale;
- `refs` use a known scheme (`wikidata`, `geonames`, `nuts`, `osm`, `wof`),
  with a value in that scheme's format;
- the centroid is a valid latitude and longitude;
- `shape` is a `.geojson` or `.json` path relative to the project and inside
  it.

The recipe load then checks that the shape file exists. The [project file
reference](/reference/project-file#places) lists the error messages.

`KapiProject.Gazetteer()` returns the gazetteer built at load, and
`KapiProject.AxisSpace()` is `axis.NewSpace(p.Axes, p.Gazetteer())`, so every
caller that interprets a coordinate uses the project's places.

### Containment and overlap

`Gazetteer.Contains(outer, inner)` applies these rules to the declarations:

- every non-empty id contains itself, known or not;
- the world contains every known place;
- a place made of places sits within `outer` when every one of its members
  does;
- a place made of places contains `inner` when one of its members contains it,
  or when `inner` is declared within it;
- otherwise `inner` sits within `outer` when one of the places it sits within
  does.

An id that is not in the gazetteer contains only itself and sits within only
itself. A rule recorded at a place that another checkout declares therefore
still applies to content at that same id, and to no other place.

`Gazetteer.Overlaps(a, b)` is true when one place contains the other, when a
member of a place made of places overlaps the other place, or when a declared
place sits within both. The built-in places form a tree, so two built-in
places overlap only when one contains the other. Only a declared place with
several `within:` parents connects two branches. Suggestions at `greater-oslo`
and at `oslo` overlap, and so do suggestions at `nordics` and at `no-03`.
Suggestions at `oslo` and at `bergen` do not.

For a geo axis, `axis.Space.Contains` and `axis.Space.Overlaps` call these two
functions. Scope coverage of a point
([C-11](c-11-context-operations.md#coverage-reads-the-projects-typed-axes)),
the terms-store match of a concept at a point (`terms.AtPointIn`) and the
overlap test between records in the operation log's fold all go through them.

### Paths, ancestors and children

A surface that shows a place in context needs a single chain from the world
down to it. `Gazetteer.Path(id)` follows each place's first `within:` parent.
For a place made of places, the parent is the nearest place on its first
member's path that contains every member: `northern-europe` for `nordics`, and
`no` for `greater-oslo`. `Path("grunerlokka")` is
`[world europe northern-europe no no-03 oslo grunerlokka]` and `Path("nordics")`
is `[world europe northern-europe nordics]`.

`Ancestors(id)` lists every place that contains `id`, nearest first,
including each place made of places that contains it. `Children(id)` lists the
places whose path parent is `id`, plus the members when `id` is a place made of
places, sorted by name. The atlas uses these to collect the places a map can
drill into.

### Looking a place up

`Gazetteer.Search(query, limit)` matches places whose id starts with the query,
then places whose name starts with it, then places whose name contains it. It
ignores case and accents, so `grunerlokka` finds Grünerløkka, and a declared
place's names in other locales count as names. Within each tier the exact id
comes first, then wider kinds of place, then ids in order.

A person or an agent looks up an id with `kapi context places [query]
[--within <id>]` (`host.App.ContextPlaces`). With a query, the command lists
the matches, limited to places inside `--within` when it is given. With only
`--within`, it lists the places directly below that place. With neither, it
lists the declared places. Each line gives the id, the name, the type and the
path. If a coordinate value is not a place id, the recipe fails to load and the
error carries a hint from `place.Hint`. The hint is the lower-case or
hyphenated spelling when that is a place id, otherwise up to three matches and
the `kapi context places` command for finding more.

### A locale can name a place

`place.OfLocale(tag)` returns the place a locale tag identifies. That is the
subdivision in a `-u-sd-` extension (`nb-NO-u-sd-no03` is `no-03`), else the
country or M49 region of the region subtag (`fr-CA` is `ca`, `es-419` is
`latin-america-and-the-caribbean`). A tag with neither returns no place. The
atlas uses it to derive the `region` axis of every record that has a locale. A
recipe's `target_languages` may use the `-u-sd-` extension.

### The same rules in Go and TypeScript

The map filters and counts in the browser, so `@neokapi/context-atlas`
implements the same rules over the atlas's `places` (`placeIndex` in
`model/places.ts`: `contains`, `overlaps`, `path`, `children`).
`core/place/testdata/containment.json` is a small gazetteer with the cases for
containment, overlap and paths. The Go test and the TypeScript test both run
every case, and the Go test also checks that each built-in place in the fixture
matches the built-in gazetteer's id, type and parent. A change to the rules
updates both implementations and the fixture in the same commit.

### Drawing a place

The atlas document carries the places a map needs
([Context atlas](../../implementation/context/context-atlas.md#places)): every
place a record names, everything that contains them, the members of each place
made of places, the declared places, and the children of each country and
subdivision in that set. A declared place with a `shape:` carries its outline
as GeoJSON geometry. If the shape file cannot be read, is not a single Polygon
or MultiPolygon, or is larger than 1 MiB, the outline is omitted and the atlas
adds a note naming the recipe key.

The built-in outlines ship in `@neokapi/context-atlas` (`src/geo/`): Natural
Earth (public domain) at a pinned release, the admin-0 countries in one file
and each country's admin-1 subdivisions in a file of its own, keyed by place id
and loaded when the map drills into that country. A script fetches the pinned
files, checks their SHA-256 digests and writes the TopoJSON
(`vp run @neokapi/context-atlas#build-geometry`). Where Natural Earth carries a
subdivision under an older code, or only as the parts of a current one,
`scripts/geometry-successors.json` composes the current id from those parts and
cites the source of each row: the boundary or code change, or the parent
relation. The package's `src/geo/README.md` lists the
sources, the licence and the ids that have no outline.

The map draws a declared place from its geometry, else as a circle at its
centroid, else lists it beside the map. It flags a declared place whose
centroid falls outside the outline of the place it sits within, which usually
means the `within:` is wrong. Drawing has no effect on containment.

## Consequences

- A project names its own markets at any grain, from a neighbourhood to a
  sales territory, and a rule kept for a wider place holds in every place
  inside it without a copy per place.
- Containment depends only on the recipe and the pinned data, so it gives the
  same result on every machine and in every surface, with or without outline
  files.
- A wrong `within:` changes where rules apply. The change appears in the
  recipe's diff, and the map flags a centroid outside its parent.
- Updating the built-in subdivisions is a reviewed change to the generator's
  pinned release and the amendments file. The diff of the generated table lists
  each changed id.
- The map's outlines lag ISO 3166-2 wherever Natural Earth does. The map lists
  a place without an outline instead of drawing it. Containment is unaffected.

## See also

- [C-02: Coordinates and governance](c-02-coordinates-and-governance.md): the
  typed axes, of which geo is one, and the point a file sits at.
- [C-11: Context operations](c-11-context-operations.md): how a rule's scope
  covers a point, and how a note narrows to a place.
- [Context atlas](../../implementation/context/context-atlas.md): the `places`
  the atlas carries for the map.
- [Project file reference](/reference/project-file#places): every key of
  `places:` and the recipe-load error messages.
- [Place content in markets and areas](/kapi/places): the user guide.
