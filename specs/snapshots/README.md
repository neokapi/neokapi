# snapshots/ — pinned spec text, two storage classes

Each snapshot is an unmodified copy of one published spec at one pinned
version: the original `source.(html|pdf|txt)`, the publisher's required
notice alongside it, and a `meta.yaml` provenance record
(`{sha256, fetched_at, wayback_url, license, publisher_version_label}`).
Snapshots are immutable — a new version is a new directory, never an edit to
an existing one. Citations (`{spec, version, url#fragment, clause, heading,
quote, quote_sha256}`) resolve against these snapshots, never the live
network. Which class a spec belongs to is its `rights` field in
`specs/catalog.yaml`.

## Vendor class — committed

`snapshots/<spec>/<version>/`

Specs whose rights class is `vendor`: IETF/WHATWG/Ecma/Unicode-data/Google
and community specs per repo LICENSE, plus W3C-TR/OASIS/MS-* — those last
three strictly as unmodified copies with the publisher's notice retained,
implementation excerpts only, never reassembled into a browseable mirror or
a spec-like derivative. CC-BY-SA material (CommonMark) keeps its own LICENSE
file alongside as a clearly separately-licensed unit inside this
Apache-2.0 repository.

## Cache class — gitignored

`snapshots/cache/<spec>/<version>/`

Specs whose rights class is `cache` (Unicode UAX/UTS/LDML prose; publishers
with unverified licenses). Same `<spec>/<version>` shape, but the whole
`cache/` subtree is gitignored (`specs/.gitignore`) and populated
deterministically by the fetch/extraction tooling wherever the text is
needed — the repository commits only the catalog entry and the citations
that resolve against the cached text.

Two classes is the whole model — `link-only` (Apple) and `never` (ISO/IEC)
specs have no snapshot in either class: link-only citations carry the URL
plus a short quote only, and ISO text never enters the repo, the shared
cache, or any prompt (use the free Ecma-376 / OASIS ODF twins).
