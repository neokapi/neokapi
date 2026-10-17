# Built-in shapes

The map draws countries and subdivisions from these files. Each is TopoJSON,
simplified and quantised, with every shape keyed by its place id.

| File               | Holds                                                                                | Key                                     |
| ------------------ | ------------------------------------------------------------------------------------ | --------------------------------------- |
| `world.json`       | Countries, object `countries`                                                        | ISO 3166-1 alpha-2 in lower case (`no`) |
| `admin1/<cc>.json` | One country's subdivisions, object `subdivisions`, and its outline, object `outline` | ISO 3166-2 in lower case (`no-03`)      |

A country takes `ISO_A2_EH` and falls back to `ISO_A2`. A shape with neither
(Somaliland, Northern Cyprus, Siachen Glacier) is drawn as land with no place
id. Where Natural Earth files a subdivision under an ISO 3166-2 region
(`region_cod`, as France's departments sit in regions), the region's shape is
the union of its subdivisions and is keyed by the region's code (`fr-ara`).

## Source and licence

[Natural Earth](https://www.naturalearthdata.com/) vector data, release
**v5.1.2**, read from
[nvkelso/natural-earth-vector](https://github.com/nvkelso/natural-earth-vector/tree/v5.1.2/geojson):

- `ne_50m_admin_0_countries.geojson` for `world.json`
- `ne_10m_admin_1_states_provinces.geojson` for `admin1/`

Natural Earth is in the public domain. Its terms of use:
<https://www.naturalearthdata.com/about/terms-of-use/>.

## Regenerating

```bash
vp run @neokapi/context-atlas#build-geometry
```

`scripts/build-geometry.mjs` fetches both files at the pinned release, checks
their SHA-256 digests, and rewrites this directory. Append `--cache <dir>` to
read files already downloaded to `<dir>`.

## Successors: current ids composed from older codes

Natural Earth v5.1.2 carries some subdivisions under codes ISO 3166-2 has
since replaced, or only as the parts of a current subdivision.
`scripts/geometry-successors.json` lists each current id with the Natural Earth
features it is composed from and the change or parent relation it rests on.
The generator unions the parts into one shape keyed by the current id. A row
that names a code Natural Earth lacks, or an id it already carries, stops the
build.

| Kind | Countries |
| --- | --- |
| A reform that merged subdivisions | Norway (2020 and 2024), France (Corse, Alsace) |
| A code change with the area unchanged | Norway, France, Poland, Czechia, Guatemala, Kazakhstan, India, South Africa, Taiwan, Hungary, Greece, Mexico |
| A parent level Natural Earth does not draw | Spain (autonomous communities), Belgium (regions), Ireland (provinces), Indonesia (geographical units), United Kingdom (countries) |

Norway's current counties are unions of the 19 counties Natural Earth draws
from before 2020: Innlandet is Hedmark and Oppland, Agder is Aust-Agder and
Vest-Agder, Vestland is Hordaland and Sogn og Fjordane, Trøndelag is
Sør-Trøndelag and Nord-Trøndelag, and Østfold, Akershus, Buskerud, Vestfold,
Telemark, Troms and Finnmark each take the pre-2020 county of that name. These
outlines follow the pre-2020 county borders, so the few municipalities that
moved between counties since then sit on their old side. The shapes keyed
`no-01`, `no-02`, `no-04` to `no-10`, `no-12`, `no-14`, `no-16`, `no-17`,
`no-19` and `no-20` are those former counties; no current place takes their
ids.

## What the data does not hold

Some current ids have no shape, and the map lists such a place beside the
outline of the place it sits in:

- Natural Earth draws only a coarser level: Czech districts, Kenyan counties,
  Estonian municipalities, Moroccan provinces, Luxembourg's cantons.
- The subdivision was split from another, or redrawn across older borders, so
  no union of Natural Earth features draws it: Kazakhstan's 2018 and 2022
  regions and the regions they were cut from, Indonesia's Papua provinces,
  North Kalimantan, Nepal's provinces, the Métropole de Lyon.
- Natural Earth predates a reform this table does not map: Latvia's 2021
  municipalities, North Macedonia's municipalities.
- Natural Earth draws no feature for it: Jan Mayen (`no-22`).
