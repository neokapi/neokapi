---
id: streaming-tree-formats
title: "Spike: streaming the whole-document (tree) formats"
description: "Findings from a spike on whether the tree/structured-data format readers (JSON, XML, catalogs, …) can be converted to bounded-memory streaming, validated by a streaming JSON tokenizer."
keywords: [streaming, bounded memory, json, xml, tree formats, OOM, spike]
---

# Spike: streaming the whole-document (tree) formats

## Context

[#1016 / PR #1020](https://github.com/neokapi/neokapi/pull/1020) converted the
read→process→write path to true streaming and adopted seven **line/record**
formats (splicedlines, versifiedtext, properties, srt, fixedwidth, paraplaintext,
mosestext). The remaining **whole-document** formats — JSON, XML, HTML, YAML,
Markdown, and the catalogs built on them — still buffer the entire document (and
often a DOM/AST several× larger), so they remain the operational OOM vector:
peak ≈ document × parse-expansion × concurrency, capped only by the `core/safeio`
1 GiB input guard (a guard, not a memory budget).

This spike tests the hypothesis behind reducing that risk:

> **Most info we care about is block-level, and structure is built on
> previously-seen data.** i.e. for *extraction*, a translatable unit's context
> (identity, path, translatability) comes from its **ancestors + already-seen
> header declarations** — never forward references or random/backward access —
> so a streaming tokenizer + a bounded container stack could replace the full
> tree, emitting non-translatable bytes as skeleton as it descends.

## Verdict: the hypothesis holds for the structured-data tree formats

It is **correct** for JSON, XML, and everything layered on them; the genuine
exceptions are YAML and Markdown, where the blocker is a *third-party full-AST
parser*, not the data model.

### Evidence — JSON is already a forward, ancestor-only walk

`core/formats/json/reader.go` tokenizes the whole buffer (`sc.scan()` →
`[]token`) then walks it with `walkTokenValue/Object/Array`. That walk:

- advances **strictly forward** — `pos` only ever increments; every site is
  `peek tokens[*pos]` then `*pos++`, never a backward index;
- carries context as **ancestors only** — `path` (the JSON key path) is built
  from enclosing keys, `parentLayerID` from the enclosing layer, and per-object
  pending state is saved/restored on scope entry/exit (a bounded stack);
- emits **skeleton bytes incrementally** — each token already carries its
  `prefix` (the exact whitespace/punctuation/comment bytes before it), which is
  written to the skeleton as the walk passes; the only whole-buffer dependency is
  `layer.Properties["json.original"]`, set **only on the non-skeleton path**.

So nothing in JSON extraction needs the whole tree — only a bounded ancestor
stack and the current token. The buffering is incidental: `scan()` materializes
all tokens, and `io.ReadAll` materializes the bytes.

### Evidence — XML already streams its parser

`core/formats/xml/reader.go` uses Go's streaming `encoding/xml.Decoder` with an
explicit **iterative `elementFrame` stack** (deliberately replacing recursion to
bound depth). Context (ITS rules, translatable-attr resolution) is resolved by an
**upward** walk of that ancestor stack. It still `io.ReadAll`s, but only for
byte-offset skeleton slicing — the same incidental dependency as JSON.

### The `encoding/xml` substrate vs. ITS — two independent concerns

A crucial distinction (an earlier spike conflated them):

- **The substrate blocker is `encoding/xml` giving no raw source bytes.** The
  byte-exact skeleton records `[start,end)` offsets (`InputOffset()`) during the
  walk and slices the *original buffer* at the end — hence `io.ReadAll`. This is
  a **mechanism** problem shared by every `encoding/xml`-based reader (`xml`,
  `tmx`, `ts`). It is **not** fixed by `x/net/html` — that is an HTML5 tokenizer
  (no namespaces; different CDATA/PI/DOCTYPE/entity semantics), right for the
  HTML reader (which already uses its `Raw()`), wrong for the XML family. It is
  fixed by a **capturing reader** over `doc.Reader` (a sliding window exposing
  `slice(start,end)` + `discardTo(offset)`) so `InputOffset()` ranges resolve to
  raw bytes without holding the whole document, plus **incremental prefix
  emission** (flush skeleton text/refs and discard the buffer prefix as ranges
  become *decided* — no future token can nest in them — reusing the existing
  sort + `removeOverlappingParents` over the decided prefix). No stdlib fork.
- **ITS 2.0 global rules are a `core/formats/xml`-only concern.** The `its`
  package is imported by exactly one reader. `<its:rules>` carry document-wide
  XPath selectors with last-rule-wins precedence and can appear anywhere (or be
  externally linked), so resolving them is inherently whole-document. **Local
  ITS markup + inheritance is streaming-safe** (ancestor-only, on the element
  stack). There is no community "streaming ITS" profile — Okapi's `ITSEngine`,
  GNOME `itstool`, and the W3C reference implementations all build the tree and
  apply XPath; XLIFF side-steps it by pre-resolving global rules at extraction.
  Whole-document ITS is correct where it is actually used.

So ITS does **not** force the whole XML family into memory. `tmx`/`ts` don't use
ITS and stream unconditionally on the substrate; generic `xml` streams **except**
when the document declares global ITS rules, where it correctly falls back to the
buffered walk. The one honest `O(document)` island — generic XML *with* global
ITS rules — is explicitly scoped, not a family-wide constraint.

## JSON: implemented (streaming reader, wired and gated)

JSON is now a **full production implementation**, not a spike. The streaming
tokenizer is wired into the reader behind a `tokenStream` seam:

- `core/formats/json/streamscanner.go` — `streamScanner`, a JSON tokenizer that
  reads from an `io.Reader` through a small `bufio` window (all lookahead ≤ 10
  bytes) and emits the same `token{typ,raw,value,prefix}` sequence as the
  buffered `scanner.next()`. Peak memory is `O(bufio window + current token)`.
- `core/formats/json/tokenstream.go` — the `tokenStream` interface with two
  backends: `sliceStream` (the buffered token slice) and `streamTokenStream`
  (the streaming scanner). The forward, ancestor-only walk
  (`walkTokenValue/Object/Array`) is unchanged — only the token source differs.
- `core/formats/json/reader.go` — `readContent` takes the streaming path
  (`readContentStreaming`) when the read is a **same-format skeleton round-trip
  with validation off** (`skeletonStore != nil && ValidationMode == Off`); it
  walks the document token-by-token with an ancestor-only key-path stack, never
  materialising the document or its token slice. Validation mode (needs the
  buffer for snippet windows) and the cross-format path (stores `json.original`)
  keep the buffered walk, byte-identical. `StreamingReader` is declared so the
  file-runner concurrent-feeds the reader.

Memory split: the **reader** is now bounded; the **writer** keeps its buffered
block map, which holds only the *translatable* blocks (a fraction of the
document), not the whole DOM — so peak drops from `O(document + token slice +
DOM)` to `O(nesting depth + current token + translatable blocks)`. A fully
streaming writer (interleaved skeleton consumption handling the JSON writer's
`layer:<path>` child-layer refs) is a further, separable step.

Validation:
- The existing skeleton round-trip suite (`TestSkeletonStore_ByteExact` and its
  whitespace/comment/nesting/array/unicode/escape subtests, the
  `SkeletonTranslation_*` tests) now runs **through the streaming path** and is
  byte-exact.
- `TestStreamScannerParity` — the streaming tokenizer is token-identical to the
  buffered one across nesting, escapes, surrogate pairs, JSON5, BOM, and all four
  comment styles.
- `TestStreamingReaderMatchesBuffered` — the streaming read emits the same
  Block/Data part stream as the buffered read.
- `TestStreamScannerBoundedMemory` / `TestStreamingReaderBoundedMemory` —
  tokenizing/reading a document supplied as a *pure stream* (an `io.Pipe`
  generator, never a whole buffer) holds **~1 MiB for a 12–13 MiB document** —
  flat (≈1.01×) across a 20× size change. **A 13 MiB JSON reads in under 1 MiB.**

## Per-format classification

| Format | Verdict | Notes |
|---|---|---|
| **JSON** | Ancestor-stack streamable | Forward walk + prefix-skeleton; PoC built here. |
| **XML** | Ancestor-stack streamable | Already `xml.Decoder` + iterative `elementFrame` stack. |
| **HTML** | **Blocked — unbounded forward lookahead** | The leaf/container classification (`forwardScanForBlockChildren`) walks forward from an element's open until it hits either a block-level child (→ container) or the parent's own end-tag (→ leaf). That distance is unbounded — a `<td>` with a large inline body pushes `</td>` arbitrarily far ahead — and the streaming `tokenizer.Buffered()` window caps at ~4 KB (measured: a 428 KB `<td>` body exposes only 4081 buffered bytes after the start tag), so the scan exhausts and defaults to *container*, mis-classifying leaf blocks and regressing byte-exactness (#151/#608). Not a data-model limitation; a genuine parser-model blocker. Buffered stays. |
| arb, designtokens, xcstrings | **Done** (per-entry / two-pass streaming JSON) | Each ships a bounded reader+writer: a `streamScanner`/`json.Decoder` twin produces the identical token/value sequence without `io.ReadAll`. **designtokens** & **arb** use a two-pass (pass 1 re-opens the file for a bounded metadata pre-scan — `$deprecated` / `@id` descriptions — pass 2 streams). **xcstrings** buffers one entry subtree at a time and reuses `emitEntry`/`skelWalker`. arb is a *partial* bound (retains O(messages) descriptions); designtokens/xcstrings are *full* bounds. |
| i18next | Substrate = **JSON** (streaming) | **Done** — wraps the streaming JSON reader/writer, declares `StreamingReader`/`StreamingWriter`. |
| **xml, tmx, ts** | Substrate = **`encoding/xml`** | `xml.Decoder` + `InputOffset()` byte-offset skeleton. Streamable via a **capturing reader** (resolves offsets to raw bytes without `io.ReadAll`) + **incremental prefix emission**. `tmx`/`ts` stream unconditionally; `xml` gates on *no global ITS rules* (see below). |
| resx, androidxml | **Done** (custom-tokenizer streaming) | Substrate was `newTokenizer(string(content))` + whole-buffer `.original` layer property (not `encoding/xml`). Each now ships a `streamTokenizer` reading from a bufio window that produces the identical token sequence, buffering one entry subtree at a time; `.original` retained only when no skeleton store is wired. |
| messageformat | **Done** (streaming pair) | Line-buffered `bufio` reader + `StreamSkeletonWrite` writer; parse errors surface on the Read channel. |
| applestrings | Line/record but **whole-buffer transcode** | Whole-document UTF-16→UTF-8 transcode + BOM detection before parsing; de-prioritized. |
| **YAML** | **Hard** | `yaml.v3` materializes the whole `Node` AST; aliases are forward references; byte mapping needs pre-computed line offsets. Streaming needs a different parser. |
| **Markdown / MDX** | **Hard** | `goldmark` materializes the whole AST and normalizes (nesting fixes, link-def resolution); byte mapping needs the full parse. MDX segments forward but delegates Markdown spans to goldmark. |
| Archives (openxml/odf/epub/idml/icml) | Not applicable | Random-access zip — stream at the *entry* level (already do, #1020 §6), not within an entry. |

So the ancestor-stack model covers JSON + XML + their catalogs (arb,
designtokens, xcstrings, i18next) + the `encoding/xml` + custom-tokenizer
families — the large majority of the remaining OOM surface. The residue is
blocked, not deferred: **HTML** by unbounded forward lookahead for
leaf/container classification (see the table), and **YAML / Markdown** by their
third-party full-AST parsers — parser-model limitations, not the data model.

### Implemented streaming pairs (as of the catalog assessment)

The genuinely-streaming reader+writer pairs shipped so far: the #1020
line/record formats, **JSON** (#1138), **messageformat**, **i18next** (inherits
streaming from the JSON reader/writer it wraps), the **`encoding/xml` group**
(xml #1211, tmx #1165, ts #1168), the **custom-tokenizer group** (resx #1169,
androidxml #1170), and the **JSON-substrate catalogs** (designtokens #1213, arb
#1216, xcstrings #1217). What remains buffered is parser-model-blocked (HTML,
applestrings, YAML, Markdown — see below), not awaiting a substrate conversion.

`messageformat` was the one cheap non-substrate candidate ([#1025](https://github.com/neokapi/neokapi/issues/1025)):
its reader now parses one message per line via a `bufio` window (no
`io.ReadAll`, no retained `parsedLines`) and its writer routes a streaming
skeleton store through `format.StreamSkeletonWrite`. The one behavioural change
is that a syntax error now surfaces on the Read channel (`PartResult.Error`)
instead of from `Open` — the streaming consequence of not pre-parsing — which
the file runner and subfilter driver already propagate. Verified byte-exact
through the streaming path and flat-peak on an `io.Pipe`-streamed document.

### Catalog formats: shipped, plus the two that stay buffered

The precondition (`isStreamingPair` = `IsStreamingReader && IsStreamingWriter`; a
`StreamingReader` must read incrementally from `doc.Reader`, never `io.ReadAll`)
now holds for the whole JSON-substrate catalog family. What shipped, and what
stays buffered and why:

- **xcstrings, arb, designtokens — done.** Each replaced its
  `io.ReadAll`+`json.Unmarshal` substrate with a bounded `streamScanner`/
  `json.Decoder` twin that emits the identical token/value sequence. **arb** and
  **designtokens** use a two-pass over a re-openable file (pass 1 = bounded
  metadata pre-scan: `@id` descriptions / `$deprecated`; pass 2 streams from
  `doc.Reader`); **xcstrings** buffers one `strings` entry subtree at a time and
  reuses the buffered walk's `emitEntry`/`skelWalker` for byte-exact skeleton.
  Non-file inputs fall back to the buffered walk. arb carries a documented
  *partial* bound (it retains O(messages) `@id` metadata — descriptions,
  placeholder hints — but not the content bytes, so a description-free document is
  fully bounded); designtokens and xcstrings are *full* bounds.
- **i18next** streams via its inner JSON reader/writer (unchanged).
- **applestrings — stays buffered.** The `.strings` grammar *is* line/record, but
  the reader does a **whole-buffer UTF-16→UTF-8 transcode + BOM detection**
  (`decodeToUTF8(raw)`) before parsing and retains the original bytes for
  byte-faithful rewrite. The transcode is inherently whole-document for UTF-16
  inputs, so a bounded reader is not free. Buffered stays (review §5.5:
  per-document guard + the §5.3 `safeio` wrap is adequate).
- **HTML — stays buffered.** Blocked by unbounded forward lookahead for
  leaf/container classification (see the per-format table). Not a substrate
  conversion; a parser-model limitation.

## Productionization plan (remaining formats)

JSON above is the reference implementation. The same shape applies to the rest:

1. **`tokenStream`-style seam** — route the walk's token access through an
   interface with a buffered backend and a streaming one (a streaming
   tokenizer/decoder). The walk body is unchanged. (XML already has the
   streaming half — `xml.Decoder` — so the work is emitting skeleton incrementally
   instead of byte-offset slicing.)
2. **Gate** to the same-format skeleton round-trip with validation off, declaring
   `format.StreamingReader`. Everything else keeps the byte-identical buffered
   path.
3. **Validate** with the existing skeleton suite (which then runs through the
   streaming path) + a streaming-vs-buffered Part-stream parity test + a
   bounded-memory benchmark on an `io.Pipe`-streamed document.

Per-format status / next:

- **JSON** — done (#1024).
- **`encoding/xml` group (xml, tmx, ts)** — capturing reader (`slice`/`discardTo`)
  + incremental prefix emission over the decided-frontier. **`tmx` (#1165) and
  `ts` are done** — both stream reader-side (gated to the same-format skeleton
  round-trip with validation off; `tmx` additionally gates on UTF-8 input, since
  its UTF-16 transcode is whole-document). Each keys skeleton refs by
  `tuIdx:lang` / `blockIdx:elemType`, so the multi-variant / synthesized-section
  cases need no `WriteLang`. Discard happens at the record boundary (`</tu>` /
  `</message>`), so within-record backward slices stay in the window; the
  prologue / line-break are resolved from the peeked head. **`xml` is done** —
  it uses a **two-pass** (gated to the same-format skeleton round-trip with
  validation off, over a re-openable **file** input): pass 1 scans a re-opened
  copy with `its.ExtractRulesReader` (#1171) for `<its:rules>` (bounded — only
  the rules are retained), pass 2 streams the extraction from `doc.Reader`
  through the CaptureReader. ITS documents stream too — not just ITS-free ones —
  because the resolver is **ancestor-only** (selectors match `ctx.Path`, never
  forward/descendant/sibling axes). Non-file inputs (and validation mode) fall
  back to the buffered walk. Skeleton is flushed incrementally at depth≤1
  boundaries (per-batch `removeOverlappingParents` + an `emitPos`-skip for
  cross-batch translatable-ancestor parents), and container frames that
  accumulate only whitespace/comments have their runs reset each flush so a
  container with thousands of children/comments stays bounded.
- **resx, androidxml** — custom-tokenizer substrate (not `encoding/xml`): each
  needs a **streaming variant of its own tokenizer** + dropping `.original`
  retention (only retained when no skeleton store is wired). **`resx` and
  `androidxml` are done** — each adds a `streamTokenizer` that reads from a bufio
  window producing the identical token sequence, and a streaming walk that
  buffers only one entry subtree at a time (`resx` the `<data>` entry; `androidxml`
  streams *through* `<resources>` and buffers each `<string>`/`<string-array>`/
  `<plurals>` subtree, reusing the buffered walk's emit handlers).
- **arb, designtokens, xcstrings** — done (JSON-substrate streaming; see the
  catalog section above).
- **HTML** — **stays buffered.** Attempted and rejected: leaf/container
  classification (`forwardScanForBlockChildren`) needs forward lookahead to the
  parent's end-tag, whose distance is unbounded, and the streaming
  `tokenizer.Buffered()` window caps at ~4 KB (measured). Streaming would default
  large leaf blocks to *container* and regress byte-exactness (#151/#608). A
  parser-model blocker, not a substrate conversion.
- **YAML, Markdown** — blocked by `yaml.v3` / `goldmark` full-AST parsers; need a
  streaming parser swap. Out of scope for this line of work.

**Fully-streaming writers are now done too** for the `encoding/xml` +
custom-tokenizer group (resx, androidxml, tmx, ts, xml). Each writer declares
`StreamingWriter`, so the file-runner's `isStreamingPair` engages the concurrent
`NewStreamingSkeletonStore` and the writer consumes skeleton entries interleaved
with the Part stream — pulling each block on demand instead of buffering the
whole block map. resx/androidxml use the generic `format.StreamSkeletonWrite`
(single-block refs); tmx/ts pull blocks by arrival index for their composite
`tuIdx:lang` / `blockIdx:elemType` refs (evicting past the current record); xml
keeps a small rolling window so a content block's inline attribute-ref blocks
(emitted just before it) are still resolvable. Reader and writer are both bounded
now, so a round-trip's peak is O(read-ahead + current record), independent of
document size. Verified: streaming-pair byte-exactness (matching the buffered
path) + a bounded-writer benchmark holding a fraction of a MiB for a multi-MiB
document, race-clean.

### Out of scope / caveats

- Malformed-input error byte-offsets and the validation-mode snippet window
  genuinely need the buffer — those modes stay buffered (gated out).
- `json.original` (non-skeleton cross-format path) stays buffered; streaming
  targets the same-format skeleton round-trip, which is the bounded-memory goal.
- YAML and Markdown need a streaming parser to replace yaml.v3 / goldmark — a
  separate, larger effort; not recommended as part of this line of work.
- Even where streaming is unbounded-in-theory, the practical guard remains: lower
  the `safeio` input cap per server context and bound concurrency for the formats
  that stay buffered.

## Status

Streaming (bounded-memory reader **and** writer) is **implemented and shipped**
for every tractable whole-document format: JSON, the `encoding/xml` group (xml,
tmx, ts), the custom-tokenizer group (resx, androidxml), and the JSON-substrate
catalogs (arb, designtokens, xcstrings, i18next) — each its own byte-exact
conversion, validated by its existing skeleton suite through the streaming path
plus a bounded-memory benchmark. The residue stays buffered by a **parser-model**
limitation, not the data model: **HTML** (unbounded forward lookahead for
leaf/container classification), **applestrings** (whole-document UTF-16
transcode), and **YAML / Markdown** (yaml.v3 / goldmark full-AST parsers). Those
are parser-swap projects, tracked separately from [#1025](https://github.com/neokapi/neokapi/issues/1025).
