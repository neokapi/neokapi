---
id: 031-content-fidelity-surfacing
sidebar_position: 31
title: "AD-031: Content-Fidelity Surfacing"
description: "Architecture decision: a format reader serves two consumers from one parse — the translation pipeline (MT/TM) and LLM/RAG ingestion that wants all textual context (code, captions, alt-text, do-not-translate strings, config-excluded values, comments). Readers surface non-translatable contextual content as content by default rather than burying it in opaque skeleton, via two channels — renderable content as Block{Translatable:false} carrying a SemanticRole and a skeleton ref, comment/metadata context as Data or a NoteAnnotation — gated by a default-ON, per-format opt-out (extractNonTranslatableContent). Byte-exact round-trip, MT-skip, and Okapi parity all hold."
keywords: [content fidelity, non-translatable content, surfacing, LLM ingestion, RAG, docling, semantic role, skeleton, Block, Translatable flag, extractNonTranslatableContent, alt-text, captions, code blocks, do-not-translate, config-excluded values, comments, parity-safe carrier, dual consumer, architecture decision, neokapi]
---

# AD-031: Content-Fidelity Surfacing

## Summary

A format reader serves **two consumers** from a single parse. The first is the
translation pipeline — MT, TM, the editor — which wants only the prose a human
would localize. The second is **LLM/RAG ingestion**, which wants *all* textual
context: code listings, captions, image and shape alt-text, formulas,
do-not-translate UI strings, config-excluded values, and developer/translator
comments. Historically a reader was a hard binary — a fragment became either a
translatable `Block` or opaque skeleton bytes — so everything the first consumer
skips was invisible to the second.

This AD makes surfacing that contextual content a **cross-cutting reader
convention**: every reader classifies a fragment three ways, not two, and emits
non-translatable-but-meaningful context as *content* rather than hiding it. Two
channels carry it — renderable content as a `Block{Translatable:false}` bearing a
`SemanticRole` and a skeleton ref; comment and metadata context as a `Data` part
or a `NoteAnnotation`. The behaviour is gated per format by a default-ON opt-out,
`extractNonTranslatableContent`. Byte-exact round-trip, MT-skip semantics, and
Okapi parity are all preserved unchanged — the surfaced content is additive over
a parity-faithful core.

This rests on primitives defined elsewhere and introduces no new content-model
type: the `Translatable` flag, the `SemanticRole` taxonomy, `Data`, and notes are
all the content model's ([AD-002](002-content-model.md)); the reader output policy
and the skeleton/sub-skeleton mechanism are the format system's
([AD-005](005-format-system.md)).

## Context

The content model already encodes a **third state** between "translate this" and
"this is pure structure": a `Block` with `Translatable: false`
([AD-002](002-content-model.md)). Such a block is visible to anything reading the
part stream as content, yet machine translation skips it
([AD-012](012-mt-providers.md)), and it can carry a `SemanticRole` — `RoleCode`,
`RoleCaption`, `RoleFormula`, `RoleTableCell`, and the rest of the open taxonomy
in `core/model/structure.go` — so a consumer knows *what kind* of context it is.

The mechanism existed; the readers did not use it. A reader faced with a fenced
code block, a `<wp:docPr descr=…>` alt-text attribute, a config-excluded value in
a JSON file, or a `#.` translator note had two destinations only:
`Block{Translatable:true}` (wrong — MT would translate a code listing) or the
opaque skeleton / a contentless `Data` part (round-trips byte-exactly, but the
content is gone from the stream). The third state was never produced, so the
ingestion consumer saw a document stripped of exactly the context it most wants.

The motivating bar is **docling-style ingestion fidelity** — a parse that
surfaces every textual region a downstream model could ground on, with each
region tagged by role. Reaching it does not require a parallel "ingestion reader";
it requires the existing readers to stop discarding context they already see while
walking the document. The change is therefore a convention applied uniformly, not
a new subsystem. Issue [#928] tracks the per-format rollout; AsciiDoc and Markdown
are the landed reference implementations.

## Decision

### The third classification: surface, don't hide

A reader classifies each fragment of its input three ways
([AD-005](005-format-system.md)):

| Fragment | Destination |
|---|---|
| Translatable prose | `Block{Translatable: true}` — the pipeline localizes it |
| Pure structure (delimiters, quoting, whitespace) | skeleton bytes |
| Non-translatable but meaningful context | **surfaced** — see the two channels below |

The first two are unchanged. The decision is that the third category — code,
verbatim/literal text, captions, alt-text, formulas, do-not-translate strings,
config-excluded values, comments — is no longer collapsed into the second. It
becomes content the ingestion consumer can read, while staying outside the MT
payload.

### Two surfacing channels

What a fragment *is* determines which channel carries it. Renderable content (text
that has a place in the rendered document) becomes a content block; out-of-band
annotation (text *about* the document) becomes data or a note.

| Channel | Carrier | Used for | Round-trip |
|---|---|---|---|
| Renderable contextual content | `Block{Translatable:false}` + `SemanticRole` + skeleton ref | code blocks, literal/verbatim text, captions, alt-text, formulas, do-not-translate strings, config-excluded values | verbatim bytes stay in skeleton; the surfaced body rides a skeleton ref, so the writer replays the original exactly |
| Comment / metadata context | `Data` part (`PartData`) or `NoteAnnotation` | developer/translator comments, review annotations, editorial notes | the comment bytes round-trip verbatim through the skeleton; the surfaced copy is informational only |

A `Block{Translatable:false}` from the first channel carries the role that names
its kind — alt-text surfaces as `RoleCaption`, a code listing as `RoleCode`, an
equation as `RoleFormula`, a non-translatable cell as `RoleTableCell` — and is
flagged so MT skips it ([AD-012](012-mt-providers.md)). The second channel keeps
comment context as *data* deliberately: a comment is not part of the rendered
text, so promoting it to a content block would misrepresent the document's
structure; it stays a `Data` part or a note that ingestion can read and the editor
can show.

### Default on, via an inverted opt-out flag

Surfacing is the **default**, controlled per format by a single boolean,
`extractNonTranslatableContent`, exposed as a `schema.Prop` in the generated
format reference and accepted in `ApplyMap` under that key. The implementation is
deliberately an **inverted private field**:

```go
// zero value false ⇒ surfacing ON (the opt-out default)
disableNonTranslatableContent bool

func (c *Config) ExtractNonTranslatableContent() bool { return !c.disableNonTranslatableContent }
func (c *Config) SetExtractNonTranslatableContent(v bool) { c.disableNonTranslatableContent = !v }
```

The inversion is the point: a freshly zero-valued config — a new format that has
not yet learned about the flag, a caller that constructs a config without calling
`Reset` — surfaces content automatically, because the *disable* bit must be set
explicitly to turn it off. The safe-for-ingestion behaviour is the one you get for
free; opting out is the deliberate act. The off-switch exists for two callers: the
parity harness, which pins the bridge-matching configuration (below), and
validation-only or pure-passthrough flows that want nothing but skeleton.

A format may also **scope** what counts as meaningful context. The design-tokens
reader composes the generic JSON reader but calls
`SetExtractNonTranslatableContent(false)` on that inner config: a token's `$value`,
`$type`, and `$extensions` are structured machine data (colours, dimensions, font
names), not contextual prose, so design tokens surface only `$description` as
translatable prose and let everything else pass through as non-translatable
structure. The convention is uniform; each reader decides which of its fragments
are genuinely *context* versus inert data.

### Round-trip, MT-skip, and parity all still hold

Surfacing is additive over the existing guarantees, not a relaxation of them.

- **Byte-exact round-trip.** The verbatim source bytes never leave the skeleton.
  A surfaced renderable block stands in for the rendered body via a skeleton ref
  (or a **sub-skeleton** — verbatim segments interleaved with refs to translatable
  spans inside an otherwise-opaque payload, [AD-005](005-format-system.md)); a
  surfaced comment's bytes are copied verbatim. An untranslated round-trip is
  byte-identical whether the flag is on or off — the openxml `#928` tests assert
  that `word/document.xml`, `ppt/slides/slide1.xml`, and the comment parts are
  byte-identical with the flag on versus off, and that the source `descr=` survives
  verbatim. Translation of a surfaced *translatable* span splices in place; the
  surrounding structure is untouched.
- **MT-skip.** A surfaced block carries `Translatable: false`, so machine
  translation skips it by the same rule it always has ([AD-012](012-mt-providers.md));
  the MT payload is unchanged.
- **Okapi parity.** The bridge has no notion of surfaced context, so a head-to-head
  with surfacing on would diverge by construction — the native stream would carry
  extra `Block`/`Data` parts the bridge never emits, and the canonical projection
  compares the `PartType` sequence and per-block `Translatable` flag
  ([AD-018](018-parity-testing.md)). The parity contract is "same semantic config →
  same results", not "same defaults": `runNative` (`cli/parity/spec/runner.go`)
  duck-types `interface{ SetExtractNonTranslatableContent(bool) }` on the reader's
  config and forces it **false** before reading, so the native stream is
  byte-identical to the bridge. The roles and properties a surfaced block carries
  are additionally **parity-safe carriers** — the canonical projection excludes
  `SemanticRole` / `StructureAnnotation`, `Properties`, `Annotations`, and the
  placeholder `Equiv`/`Disp` — but it is the flag, not the projection, that keeps
  the surfaced *parts themselves* out of the head-to-head. The full contract lives
  in [AD-018](018-parity-testing.md).

### A cross-cutting convention every reader follows

This is one convention applied across the reader fleet, not a per-format feature.
Office-document readers (DOCX/PPTX/XLSX, ODF) surface alt-text and comments; the
Markdown/markup family (Markdown, MDX, AsciiDoc, HTML, LaTeX, …) surfaces code,
verbatim/literal text, captions, and math markup; structured-data and catalog
formats (JSON, CSV, properties, Android XML, design tokens, …) surface isolated
and do-not-translate values; comment-bearing source and translation formats (PO,
doc-comment extractors, RTF annotations, …) surface notes-to-translators. Which
formats expose the flag, and exactly what each surfaces, is generated into the
format reference — see the [Format Reference](/formats) — rather than
enumerated here. The tactical ledger (per-format finding, carrier, skeleton
strategy, and the deliberately deferred edge cases) lives in the internal note,
[content-fidelity](/contribute/notes-internal/content-fidelity).

## Related

- [AD-002 Content Model](002-content-model.md) — the `Translatable` flag, the `SemanticRole` taxonomy, `Data`, and notes this convention reuses
- [AD-005 Format System](005-format-system.md) — the three-way reader output policy and the skeleton / sub-skeleton ref mechanism that keeps round-trip byte-exact
- [AD-012 Machine Translation Providers](012-mt-providers.md) — MT skips `Translatable == false`, so surfaced context never enters the MT payload
- [AD-018 Parity testing against Okapi](018-parity-testing.md) — the "same semantic config → same results" contract, the parity-safe carriers, and the runner hook that forces the flag off
- [AD-030 Multimodal Extraction and LLM Refinement](030-multimodal-extraction-and-llm-refinement.md) — extraction from non-text media likewise produces non-translatable, role-tagged Blocks for ingestion
- [AD-032 Math and Equations](032-math-and-equations.md) — equation surfacing (`RoleFormula` blocks, sub-skeleton `<m:nor/>` write-back) is an instance of this convention
- [content-fidelity](/contribute/notes-internal/content-fidelity) — the per-format finding/carrier ledger and deferred edge cases (issue [#928])

[#928]: https://github.com/neokapi/neokapi/issues/928
