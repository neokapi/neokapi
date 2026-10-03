---
id: m-01-bilingual-interop
sidebar_position: 1
title: "M-01: Bilingual format interop"
description: "kapi extract emits a bilingual file for a translator and kapi merge applies the returned targets back, with the content memory reading on extract and absorbing on merge; the native carrier is the lossless bilingual .kpz and XLIFF 2.x / PO are the industry-interop tier."
keywords: [neokapi, architecture decision, bilingual, XLIFF, PO, extract, merge, content memory, interchange]
---

import { RoundTripDiagram } from "@neokapi/docs-shared";

# M-01: Bilingual format interop

## Summary

`kapi extract` emits a bilingual file for a translator or reviewer; `kapi merge`
applies the returned targets to the translations of the project's sources,
through the change service ([E-09](../engine/e-09-the-change-contract.md)). The
project's
[content memory](../context/c-09-content-memory.md) participates on both sides of
that loop, pre-filling on extract and absorbing on merge, which is what makes
each pass cheaper than the last.

Interchange has two tiers, chosen by who receives the file. The **bilingual
`.kpz`** is the native carrier: lossless, deterministic, content-addressed, read
by a neokapi tool at the far end ([M-06](m-06-content-packages.md)). **XLIFF 2.x
and PO** are the industry-interop tier, for a recipient working in a third-party
translation tool.

Both tiers flow through the same two verbs. Every unit carries the revisions
it was extracted against, and segmentation is a stand-off overlay over the runs
rather than a rewrite of them ([M-02](m-02-segmentation.md)), so a project can
turn segmentation on or off between extractions without breaking a merge.

<RoundTripDiagram
  forward={[
    { label: "sources", sub: "project content", role: "io" },
    { label: "extract", sub: "kapi extract", role: "tool" },
    { label: "bilingual file", sub: ".kpz · XLIFF · PO", role: "io" },
  ]}
  back={[
    { label: "returned file", sub: "targets filled in", role: "io" },
    { label: "merge", sub: "kapi merge", role: "tool" },
    { label: "sources", sub: "per-locale output", role: "io" },
  ]}
  hub={{ label: "content memory", sub: "project store" }}
  forwardIndex={1}
  backIndex={1}
  forwardLabel="pre-fill"
  backLabel="absorb"
  ariaLabel="The extract and merge round trip, with the content memory pre-filling on extract and absorbing on merge"
  caption="One loop, two carriers: the native bilingual .kpz and the XLIFF/PO interop tier travel the same verbs."
/>

## The two verbs

`extract` and `merge` are top-level commands rather than named flows. An
engineer reading `kapi --help` sees them beside `run`, and their flag shapes (a
repeatable `-i`, a collection filter, a conflict policy) do not fit a flow
step's config.

```bash
kapi extract                              # every target locale in the recipe
kapi extract --target-lang fr
kapi extract --target-lang fr,de,es
kapi extract --only mobile                # one content collection
kapi extract --pattern 'src/**/*.json'
kapi extract --format kpz                 # the native bilingual carrier
kapi extract --xliff-version 2.0
kapi extract --no-memory
kapi extract --redact                     # placeholders out, originals stay local

kapi merge -i out/app-en-to-fr.xliff
kapi merge -i file1.xliff -i file2.xliff -i file3.po
kapi merge -i 'vendor-return/*.xliff'
kapi merge -i vendor-return/
kapi merge -i vendor-return/ --no-memory-update
```

Both handle every target locale in one pass. Omitting `--target-lang` uses the
recipe's `target_languages`; naming a comma-separated subset restricts it. One
output file per source→target pair, all sharing a single extraction batch id.
On merge, `-i` is repeatable and takes a path, a glob, or a directory, so a
vendor's whole multi-language return is one invocation; the carrier is detected
per input, and mixing XLIFF and PO in one batch is fine. A failure on one pair
or input is reported per item and the rest still apply, with the exit code
reflecting any failure.

Both verbs resolve the project the same way every project-aware command does:
an explicit `-p`, then `KAPI_PROJECT`, then the git-style upward walk for a
`kapi.yaml` recipe ([C-01](../context/c-01-project-model.md)). `merge` needs a
project or a `.kpz` workspace: `kapi merge work.kpz -o out/` emits the
target-language files of an ad-hoc workspace with no project in scope
([M-06](m-06-content-packages.md)), and inside a project `kapi merge` with no
`-i` materializes the target-language files from the project's block store,
the sink for a process-only run: the targets the store holds for a source
become one change set on its translation, which the change service writes from
the source's skeleton. A collection that names no `target` is source-only and
gets no file. With neither a project nor a workspace it says
so rather than guessing.

## The monolingual sibling

`extract`/`merge` are the **bilingual** round trip: source out, target back.
There is a parallel **monolingual** round trip on the same content model for
source-language work: `kapi inspect` reads a file into blocks, each with its
reference and revision, and `kapi apply` writes a change set of reviewed edits
back through the byte-faithful round trip
([E-09](../engine/e-09-the-change-contract.md), [S-03](../surfaces/s-03-agent-surfaces.md)).

The pairs share the engine (a format reader and writer
([E-02](../engine/e-02-format-system.md)), the skeleton store for faithful
reconstruction) but they answer different
questions and stay separate verbs:

- `kapi apply` lands a **reviewed** change set (a content fix, a decision, or
  an edit to a term, a memory pair, a recipe field) whole or not at all, each
  edit guarded by the revision it read and by the inline-code fidelity guard,
  exiting on the gate code when an edit is stale so a fix loop reads again. It
  never absorbs into the memory as a side effect.
- `kapi merge` applies a **translator's returned targets** and, by default,
  absorbs every accepted target into the content memory. That accretion is what
  merge is for. It sends the targets as a change set too, guarded by the
  revisions the returned file carries, and the conflict policy settles a
  translation that moved meanwhile.

Folding `apply` into `merge` would contaminate the memory with monolingual
source edits; folding `merge` into `apply` would lose the conflict policy and
the bilingual span mapping.

## Carriers

**Native: the bilingual `.kpz`.** A task-scoped profile of the `.kpz`
container ([M-06](m-06-content-packages.md)): one source→target pair, the blocks
with faithful inline codes, the segmentation and alignment overlays, the
per-source skeleton for round-trip, and the relevant memory-match and term
context, in one lossless, deterministic, content-addressed file. Selected with
`--format kpz`. It is lossless where XLIFF is lossy, carries memory and term
context inline rather than as separate TMX/TBX attachments, and, being
Merkle-hashable, gives integrity-verified, diffable review. It is *ecosystem*
interchange: both ends need a neokapi reader. Making it a cross-vendor standard
would be an open-spec and second-implementation effort, not a property of the
bytes.

**Industry interop: XLIFF 2.x and PO.** For any recipient on a third-party
tool. `extract` emits XLIFF 2.2 by default so an unknown recipient gets
something safe. The reader accepts the whole 2.x namespace family
(`urn:oasis:names:tc:xliff:document:2.0`, `…2.1`, `…2.2`) and preserves unknown
2.x attributes round-trip; the writer takes `--xliff-version 2.0|2.1|2.2` for
consumers pinned to older tooling. PO (gettext) is selected with `--format po`
and emits one entry per segment span (the whole block when there is no
segmentation overlay) with kapi's bookkeeping in developer comments
(`#. kapi-block: <id>`).

A `<unit>` that arrives with a `<source>` and no `<target>` is the normal shape
of an extraction handed to a translation step. The reader records a zero-width
target position after `</source>`, and the writer emits the `<target>` element
there when a translation exists, so a file that arrives without one leaves with
one and a run that translates nothing replays the bytes it read.

Other bilingual formats (XLIFF 1.2, Qt TS, the timed-text formats) remain
available as ordinary format support with their own readers and writers
([/formats](/formats)). They are not selectable as an extract carrier: a project
whose sources are in one of them extracts *from* it into a carrier above.

## Annotations cross the carrier

A block's runs carry what the source document structurally contained; what a
governance pass concluded about that content rides stand-off on an anchor
([F-02](../foundations/f-02-content-model.md)). XLIFF 2 has somewhere to put
such a conclusion, and the carrier keeps it in both directions.

**Reading.** An `<mrk type="term">` lands as a term overlay span; every other
`<mrk>` type lands as an `xliff2:mrk` span keeping its declared type, because a
marker this engine does not understand is still a decision someone recorded.
`<sm>`/`<em>` pairs are collected the same way, being what XLIFF 2 offers for a
span that need not nest; an `<sm>` with no `<em>` marks nothing and is dropped.
The marker positions are rebased onto the block by the same cursor walk that
builds the segment spans.

**Writing.** Drawing an annotation is a declared writer capability, beside
generative and interchange: an `InlineAnnotationWriter` names the annotation
types it knows how to draw, and the registry records them in `FormatInfo`
without loading a plugin ([E-02](../engine/e-02-format-system.md)). The
declaration is a ceiling. `defaults.annotations.write` in the recipe narrows
it and cannot widen it, so naming a type a format cannot carry asks for
nothing rather than failing, because one recipe describes many outputs. The
XLIFF 2 writer draws a located term as an `<sm>`/`<em>` pair spliced into the
inline IR at emit time, never as an `<mrk>`: only the pair carries a span that
does not nest inside one element, which is exactly what an anchor exists to
express. A span straddling two `<segment>` elements is the one it cannot draw,
and that is recorded rather than half-drawn.

## What a returned unit names

A returned unit names its block by the id the reader assigned it: the XLIFF
unit's `id`, the PO entry's `kapi-block` comment, the block of a `.kpz`
overlay. It carries two revisions
([E-09](../engine/e-09-the-change-contract.md)): the revision of the
translation as the project held it at the extraction (`if-match`, `absent`
where there was none), and the revision of the source the unit was read from
(`basis`). A revision covers an edition's runs, inline codes included, and
nothing else. Segmentation is a stand-off overlay over those runs
([F-02](../foundations/f-02-content-model.md)), not a rewrite of them, so it
moves neither the block's content hash ([F-03](../foundations/f-03-identity.md))
nor its revision: a block's identity is stable across a segmentation toggle
between extractions.

The reader-assigned id beside the hash is made unique where it is assigned. A
container qualifies each delegated block by the member it came from
(`sf2_tu1`, the shape upstream Okapi uses for a subfilter), and a document that
repeats a `trans-unit/@id` gets one store identity per unit while the spelling
the document uses is kept on the block for the writer to emit
([E-02](../engine/e-02-format-system.md)).

With no segmentation overlay a block emits one XLIFF `<segment>` or one PO
entry over its whole content. With one, the writer materializes a
`<segment>`/`<ignorable>` (or a PO entry) per span and gap, and merge maps each
returned target back to its source span through the alignment overlay and
splices the target runs into place. Segment ids ride inside the overlay; the
block hash is the join key.

Per-segment memory lookup follows the same split. `Lookup` keys on the whole
block when there is no overlay; when one is present, extract iterates its spans
and calls `LookupSegment` for sentence-level leverage
([C-09](../context/c-09-content-memory.md)).

## Extraction bookkeeping

Each `kapi extract` run writes a manifest under
`.kapi/work/cache/extractions/<batch-id>/`, alongside the per-source skeletons,
for the next extract to reuse:

```yaml
schemaVersion: 1
kind: kapi-extraction
batchId: 6f2e8a1c-...
generator: { id: kapi, version: v1.x }
createdAt: 2026-04-24T10:00:00Z
inputsHash: sha256:...
sourceLocale: en
options: { format: xliff2, xliffVersion: "2.2" }
pairs:
  - targetLocale: fr
    output: out/northsea-en-to-fr.xliff
    files:
      - source: src/locales/en/app.json
        sourceHash: sha256:...
        blocks: 412
        segments: 640
        leverage: { exact: 108, fuzzy: 67, new: 237 }
        skeleton: skel-<source-hash>.bin
```

`inputsHash` fingerprints everything besides a source file's own bytes that
shapes its output: the recipe's format config, redaction and segmentation
settings, the memory the pre-fill drew from, the output options. An incremental
re-extract reuses a prior batch's work for a file only when both `inputsHash`
and the file's `sourceHash` match, so a changed recipe or a changed memory
re-extracts; `--force` skips the reuse entirely.

Everything a merge needs from the extraction travels in the bilingual file,
so the manifest can be lost. XLIFF 2.x carries file-level
`<note category="kapi">` notes with the batch id, the source path and the
source hash at extract time, the target language in `trgLang`, and on each
unit an `<mda:metadata>` whose `<mda:metaGroup category="kapi">` holds the
`if-match` and `basis` revisions. PO carries the same as extracted comments
(`#. kapi-batch:`, `#. kapi-source-file:`, `#. kapi-source-hash:` on the
header entry; `#. kapi-block:`, `#. kapi-if-match:` and `#. kapi-basis:` on
each entry) and the language in the header's `Language` field. A `.kpz`
records the revisions by block id in its interchange task. The batch id names
the local redaction vault and the memory provenance; the source path names the
document the units translate.

Merge writes each translation from the source as the project holds it, so it
needs the same project the extraction came from. That keeps the emitted XLIFF
or PO small and friendly to a third-party tool, and keeps the memory absorb
cheap. The project directory is already the unit of portability: `git push`
ships it.

## The memory loop

**On extract**, kapi queries the project's content memory for every segment it
emits:

- an exact match pre-fills `<target>` with `state="translated"`;
- a fuzzy match at or above the recipe's threshold pre-fills with
  `state="fuzzy"` and a match-quality sub-state;
- an **ambiguous** match (several full-score exacts with differing targets) is
  never pre-filled. An unattended merge would turn an arbitrary pick into
  published content; left empty, the segment surfaces as untranslated for a
  human to settle.

`--no-memory` turns the pre-fill off.

**On merge**, every accepted target segment is written into the project's
content memory with provenance: origin source `merge`, the batch id as the
reference, the source file path as the key, and the block hash and originating
filename as properties. `--no-memory-update` turns the write-back off.

Both halves are on by default because the loop is the mechanism: without the
write-back, leverage decays to zero, and making it opt-in would leave most
projects' memories empty. Read-only memories imported into a project are never
written to, so imported TMX stays reproducible from its source file.

## Conflict policy and stale units

Merge compiles a returned file into one `set_content` per translated unit, on
the edition of the unit's language, with the unit's `if-match` as its
`if_match` and its `basis` as its `basis`, and applies the change set under
`require_basis` through the change service. The service reads the source with
its translation joined from the file the recipe's target template names,
applies the operations, and writes that file from the source's skeleton
(`filehome.Options.Materialize`), so a translation follows its source's
structure. Redacted originals are restored before the units are compiled
([C-10](../context/c-10-redaction.md)).

A unit whose source changed since the extraction is refused `stale` naming
`basis`. It is **reported**, not applied, and not absorbed into the memory. A
unit that carries no `basis` is held to the source it carries: it applies when
that source is the source as it stands, or, for a carrier with no source text,
when the source file's hash is the one the file records. A unit whose block is
gone is stale too.

A unit whose translation changed in the project since the extraction is refused
`stale` naming `if_match`, and `defaults.merge.conflict_policy` settles it:

- `translator-wins` (default): merge sends the unit again over the revision the
  project now holds, and the returned target replaces it.
- `existing-wins`: the project's translation is kept and the returned one is
  skipped.
- `newest-wins`: the returned target replaces it when the returned file is newer
  than the translation's file, and is skipped otherwise.

Each pass drops the units refused for good and sends the rest again as one
change set, so what lands lands atomically. There is no interactive prompt, so
merge stays scriptable in CI. Any other refusal (a returned target that drops
an inline code its source protects) is reported per unit and counted as
`refused`; a file of which nothing lands fails. Partial returns are ordinary:
merge applies the translated units and leaves the rest of the translation as
it is.

## Recipe surface

```yaml
defaults:
  source_language: en
  target_languages: [fr, de, es]
  merge:
    conflict_policy: translator-wins # | existing-wins | newest-wins
  memory:
    fuzzy_threshold: 75              # 0..100
  segmentation:
    source: false                    # opt-in overlay on extract
    srx: rules.srx                   # optional SRX ruleset override
  annotations:
    write: [term]                    # narrows what a writer draws inline
```

Unknown fields are rejected with a clear error and enum values are validated
against the allowed set. Each section's defaults apply when it is absent.

## Related

- [F-02: The content model](../foundations/f-02-content-model.md): runs, overlays, and why segmentation does not move a block's identity
- [F-03: Identity](../foundations/f-03-identity.md): the content hash that is the merge key
- [E-02: The format system](../engine/e-02-format-system.md): the XLIFF, PO and skeleton machinery the carriers ride on, the marks a writer can draw, and reader-assigned identity
- [M-02: Segmentation](m-02-segmentation.md): the overlay a segment span comes from
- [M-06: Content packages](m-06-content-packages.md): the `.kpz` container, its bilingual profile, and the ad-hoc workspace
- [C-01: The project model](../context/c-01-project-model.md): recipe resolution and the extraction cache
- [C-09: Content memory](../context/c-09-content-memory.md): lookup, segment lookup, and merge provenance
- [C-10: Redaction](../context/c-10-redaction.md): what `--redact` keeps out of a file that leaves the machine
- [S-03: Agent surfaces](../surfaces/s-03-agent-surfaces.md): the monolingual `inspect`/`apply` loop
