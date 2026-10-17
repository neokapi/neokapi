---
id: c-12-places
sidebar_position: 12
title: "C-12: Places"
description: "Architecture decision: a geo axis takes place ids from the project's gazetteer, the built-in world, UN M49 regions, ISO 3166-1 countries and ISO 3166-2 subdivisions plus the places a recipe declares. Each place sits within places or is made of places, and containment and overlap are read from those declarations alone, the same way in Go and in TypeScript. A shape draws a place on the map."
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
places, and those two declarations are the whole of containment. A rule scoped
to Norway answers for content at Oslo because the recipe declares Oslo within
the county `no-03`, and the built-in data puts `no-03` within Norway.

`core/place` holds the gazetteer and the one reading of it. The recipe, the
axis space, the operation log, the terms store, the host and the atlas all ask
it, and the map in `@neokapi/context-atlas` reads the same graph from the atlas
document. A conformance fixture both sides run holds the two implementations to
one set of answers. A place's shape is for drawing: the map draws built-in
places from Natural Earth outlines and a declared place from the GeoJSON file
it names.

## Context

The markets a project writes for rarely line up with one administrative level.
A campaign runs in one city and one of its neighbourhoods, a team owns a
territory of five countries, a delivery promise covers a metropolitan area that
spans two counties. A rule recorded for one of those has to reach the content
written for the places inside it, and two rules have to be recognised as
meeting when their places share ground.

Containment could be computed from geometry, by testing whether one outline
lies inside another. The answer would then depend on the boundary data, its
version and its simplification, on a geometry library, and on the files being
present. Containment declared as data gives every machine the same answer
offline, costs a map lookup, and puts every change to it in the recipe's diff,
where a person reviews it beside the content it moves.

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

A place names `within:` or `of:`, exactly one of the two. `Type` names what
kind of place it is, by name rather than by a numbered level, because the
levels differ from country to country:

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

`Type.Rank` orders the types from the widest kind to the narrowest in that
table's order, and sorting by it puts a country before its subdivisions
wherever places are listed.

A name is for display. Matching reads ids alone, so renaming a place moves no
content and no rule.

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
and writes `core/place/subdivisions.tsv`, which is committed and embedded. It
refuses an amendment the release already holds, so bumping the release removes
that line rather than applying it twice. The CLDR data carries the Unicode
License v3, shipped beside it as `core/place/UNICODE-LICENSE.txt`; the file is
not named `LICENSE`, because a file of that name draws the repository's licence
boundary.

Every built-in place sits within exactly one place, except the world, so the
built-in places form a tree. A test holds the data to that: the file parses,
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

`place.New(declared)` builds the project's gazetteer over the built-in one and
validates every declared place, naming the recipe key at fault
(`places.<id>...`). The id is a slug no built-in place holds. The place sits
within places or is made of places, never both, and every id it names is a
place. No place reaches itself through `within:` and `of:`. The type is one a
recipe may declare, `names` are keyed by locale, `refs` use a known scheme
(`wikidata`, `geonames`, `nuts`, `osm`, `wof`) with a value of that scheme's
shape, the centroid lies on the globe, and `shape` is a project-relative
`.geojson` or `.json` path inside the project. The recipe load then checks that
the shape file exists beside the recipe. The [project file
reference](/reference/project-file#places) lists the messages.

`KapiProject.Gazetteer()` returns the gazetteer built at load, and
`KapiProject.AxisSpace()` is `axis.NewSpace(p.Axes, p.Gazetteer())`, so every
caller that reads a coordinate reads the project's places.

### Containment and overlap

`Gazetteer.Contains(outer, inner)` reads the declarations:

- every non-empty id contains itself, known or not;
- the world contains every known place;
- a place made of places sits within `outer` when every one of its members
  does;
- a place made of places contains `inner` when one of its members contains it,
  or when `inner` is declared within it;
- otherwise `inner` sits within `outer` when one of the places it sits within
  does.

An id the gazetteer does not hold contains and sits within nothing but itself.
That keeps a rule recorded at a place another checkout declares answering for
content at that same place, while it answers nowhere else.

`Gazetteer.Overlaps(a, b)` holds when one place contains the other, when a
member of a place made of places overlaps the other place, or when a declared
place sits within both. The built-in places form a tree, so two built-in
places share ground only when one contains the other; a declared place with
several `within:` parents is what joins two branches. Two suggestions at
`greater-oslo` and at `oslo` meet, and a suggestion at `nordics` and one at
`no-03` meet; a suggestion at `oslo` and one at `bergen` do not.

`axis.Space.Contains` and `axis.Space.Overlaps` answer a geo axis with these
two functions, so a rule's scope covers a point
([C-11](c-11-context-operations.md#coverage-reads-the-projects-typed-axes)),
a concept in the terms store answers at a point (`terms.AtPointIn`), and two
records meet in the operation log's fold through the same reading.

### Paths, ancestors and children

Surfaces that show a place in context need one chain from the world down to it.
`Gazetteer.Path(id)` follows a place's first `within:` parent. A place made of
places follows the nearest place on its first member's path that contains
every member: `nordics` follows `northern-europe`, `greater-oslo` follows `no`.
So `Path("grunerlokka")` is
`[world europe northern-europe no no-03 oslo grunerlokka]` and `Path("nordics")`
is `[world europe northern-europe nordics]`.

`Ancestors(id)` lists every place that contains `id`, nearest first, including
each place made of places that holds it. `Children(id)` lists the places whose
path runs through `id` next, and the members of a place made of places, sorted
by name. The atlas uses them to collect the places a map can drill into.

### Looking a place up

`Gazetteer.Search(query, limit)` matches places whose id starts with the query,
then places whose name starts with it, then places whose name holds it. It
ignores case and accents, so `grunerlokka` finds Grünerløkka, and a declared
place's names in other locales count as names. Within each tier the exact id
comes first, then wider kinds of place, then ids in order.

`kapi context places [query] [--within <id>]` is the command a person or an
agent uses to choose an id (`host.App.ContextPlaces`). With a query it lists the
matches, inside `--within` when given; with only `--within` it lists the places
directly below that place; with neither it lists the declared places. Each line
gives the id, the name, the type and the path. A value that names no place
fails the recipe load with a hint from `place.Hint`: the lower-case or
hyphenated spelling when that is a place, otherwise up to three matches and the
`kapi context places` command that finds more.

### A locale can name a place

`place.OfLocale(tag)` is the place a locale tag names: the subdivision a
`-u-sd-` extension names (`nb-NO-u-sd-no03` is `no-03`), else the country or
M49 region of the region subtag (`fr-CA` is `ca`, `es-419` is
`latin-america-and-the-caribbean`). A tag with neither names no place. The
atlas reads it for the derived `region` axis of every record that carries a
locale, and a recipe's `target_languages` may carry the `-u-sd-` extension.

### One reading in Go and TypeScript

The map filters and counts in the browser, so `@neokapi/context-atlas` holds
the same rules over the atlas's `places` (`placeIndex` in `model/places.ts`:
`contains`, `overlaps`, `path`, `children`). `core/place/testdata/containment.json`
is a small gazetteer with the cases for containment, overlap and paths. The Go
test and the TypeScript test both run every case, and the Go test also checks
that each built-in place in the fixture matches the built-in gazetteer's id,
type and parent. A rule change lands in both implementations and the fixture
together.

### Drawing a place

The atlas document carries the places a map needs
([Context atlas](../../implementation/context/context-atlas.md#places)): every
place a record names, everything that contains them, the members of each place
made of places, the declared places, and the children of each country and
subdivision in that set. A declared place with a `shape:` carries its outline
as GeoJSON geometry; a shape file that cannot be read, holds no single polygon,
or exceeds 1 MiB is left out with a note naming the recipe key.

The built-in outlines ship in `@neokapi/context-atlas` (`src/geo/`): Natural
Earth (public domain) at a pinned release, the admin-0 countries in one file
and each country's admin-1 subdivisions in a file of its own, keyed by place id
and loaded when the map drills into that country. A script fetches the pinned
files, checks their SHA-256 digests and writes the TopoJSON
(`vp run @neokapi/context-atlas#build-geometry`). Where Natural Earth carries a
subdivision under an older code, or only as the parts of a current one,
`scripts/geometry-successors.json` composes the current id from those parts and
names the change it rests on. The package's `src/geo/README.md` lists the
sources, the licence and the ids that have no outline.

The map draws a declared place from its geometry, else as a circle at its
centroid, else lists it beside the map. It flags a declared place whose
centroid falls outside the outline of the place it sits within, which is the
usual sign of a wrong `within:`. None of this changes what contains what.

## Consequences

- A project names its own markets at any grain, from a neighbourhood to a
  sales territory, and a rule kept for a wider place holds in every place
  inside it without a copy per place.
- Containment is a property of the recipe and the pinned data, so every
  machine, every surface and every reader of the operation log agrees on it,
  with or without outline files.
- A wrong `within:` moves rules; it shows in the recipe's diff, and the map
  flags a centroid outside its parent.
- Updating the built-in subdivisions is a reviewed change to the generator's
  pinned release and the amendments file, and the generated table shows it id
  by id.
- The map's outlines lag ISO 3166-2 where Natural Earth does, and a place
  without one is listed rather than drawn. Containment is unaffected.

## See also

- [C-02: Coordinates and governance](c-02-coordinates-and-governance.md): the
  typed axes, of which geo is one, and the point a file sits at.
- [C-11: Context operations](c-11-context-operations.md): how a rule's scope
  covers a point, and how a note narrows to a place.
- [Context atlas](../../implementation/context/context-atlas.md): the `places`
  the atlas carries for the map.
- [Project file reference](/reference/project-file#places): every key of
  `places:` and the messages a recipe load gives.
- [Place content in markets and areas](/kapi/places): the user guide.
