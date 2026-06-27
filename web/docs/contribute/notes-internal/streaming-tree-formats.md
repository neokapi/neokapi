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
| **HTML** | Ancestor-stack streamable (tokenizer path) | The skeleton path is tokenizer-based; the DOM `html.Parse` path is for normalization/error-recovery. |
| arb, designtokens, i18next, xcstrings | Substrate = **JSON** | Inherit JSON's verdict (wrappers/config over the JSON reader). |
| resx, ts, tmx, androidxml | Substrate = **XML** | Inherit XML's verdict (XML-token walks). |
| messageformat, phpcontent | Streamable | Line-buffered/lexer, bounded per-line state. |
| **YAML** | **Hard** | `yaml.v3` materializes the whole `Node` AST; aliases are forward references; byte mapping needs pre-computed line offsets. Streaming needs a different parser. |
| **Markdown / MDX** | **Hard** | `goldmark` materializes the whole AST and normalizes (nesting fixes, link-def resolution); byte mapping needs the full parse. MDX segments forward but delegates Markdown spans to goldmark. |
| Archives (openxml/odf/epub/idml/icml) | Not applicable | Random-access zip — stream at the *entry* level (already do, #1020 §6), not within an entry. |

So the ancestor-stack model covers JSON + XML + their catalogs + HTML's
tokenizer path — the large majority of the remaining OOM surface. YAML and
Markdown are blocked by their third-party AST parsers, which is a parser-swap
project, not a data-model limitation.

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

- **JSON** — done (this PR).
- **XML** — `xml.Decoder` + iterative `elementFrame` stack already; convert the
  byte-offset skeleton to incremental prefix emission. The XML/JSON-substrate
  catalogs (resx/ts/tmx/androidxml; arb/i18next/designtokens/xcstrings) follow
  their substrate.
- **HTML** — the tokenizer skeleton path is streamable; the DOM path stays for
  normalization.
- **YAML, Markdown** — blocked by `yaml.v3` / `goldmark` full-AST parsers; need a
  streaming parser swap. Out of scope for this line of work.

A fully streaming **writer** (interleaved skeleton consumption) is a separable
step that would bound the writer's block map too; the current work bounds the
reader, which removes the document-shaped (DOM/token-slice) part of the peak.

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

JSON streaming is **implemented and shipped** in this line of work — the
dominant structured-data OOM vector for the reader. XML and the JSON/XML-substrate
catalogs follow the same pattern (each its own byte-exact conversion, validated by
its existing skeleton suite). YAML/Markdown stay buffered pending a streaming
parser swap.
