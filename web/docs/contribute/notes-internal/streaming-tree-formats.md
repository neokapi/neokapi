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

## Proof of concept: a bounded-memory, byte-exact streaming JSON tokenizer

The crux uncertainty was *not* the walk (clearly forward) but: **can we tokenize
JSON byte-exactly from a stream with bounded memory?** This spike builds and
validates exactly that.

- `core/formats/json/streamscanner.go` — `streamScanner`, a JSON tokenizer that
  reads from an `io.Reader` through a small `bufio` window (all lookahead ≤ 10
  bytes) and emits the same `token{typ,raw,value,prefix}` sequence as the
  buffered `scanner.next()`. Peak memory is `O(bufio window + current token)`.
- `core/formats/json/streamscanner_test.go`:
  - **`TestStreamScannerParity`** — across representative well-formed JSON/JSON5
    (nesting, escapes, surrogate pairs, numbers, literals, bare identifiers,
    single quotes, BOM, and `//` `/* */` `#` `<!-- -->` comments) the streaming
    scanner produces a **byte-identical token sequence** to the buffered scanner.
    Because the walk is shared and token-equal ⇒ byte-exact, this is sufficient
    to conclude an end-to-end streaming JSON read would be byte-exact without
    re-plumbing the walk yet.
  - **`TestStreamScannerBoundedMemory`** — tokenizing a document supplied as a
    *pure stream* (an `io.Pipe` generator, never a whole buffer): peak heap is
    **~1 MiB for a 13 MiB document and ~1 MiB for a 0.6 MiB document** — flat
    (≈1.01×) across a 20× size change. A buffered tokenizer would hold ≥ the
    whole document plus a token slice.

This de-risks the core claim: **a 13 MiB JSON tokenizes in under 1 MiB.**

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

## Productionization plan (the low-uncertainty remainder)

For JSON (then the same shape for XML and the catalogs):

1. **`tokenStream` seam.** Extract the walk's token access behind a tiny
   interface (`peek()` / `advance()`), with two backends: the existing slice
   (`scan()` output) and the new `streamScanner`. The walk body is unchanged.
2. **Gate the streaming path** in `readContent` to the cases that don't need the
   whole buffer: **skeleton wired** (same-format round-trip), **validation mode
   off** (RVM snippet windows need the buffer), and **no subfilter resolver**
   (embedded HTML-in-JSON dispatch). Everything else keeps the byte-identical
   buffered path. Declare `format.StreamingReader` (conditional, like
   paraplaintext) so the file-runner takes the concurrent feed.
3. **Reuse #1020's machinery** unchanged: the reader emits skeleton refs +
   prefix text as it walks; the writer consumes them via
   `format.StreamSkeletonWrite` (the concurrent skeleton store), giving
   byte-identical output with bounded memory.
4. **Validate** with the existing JSON suite (buffered path byte-identical
   through the seam) + a buffered-vs-streaming Part-stream parity test + the
   memory benchmark.

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

## Recommendation

Proceed to productionize streaming JSON behind the capability (steps 1–4),
then XML and the JSON/XML-substrate catalogs follow mechanically. This removes
the dominant OOM vector for structured-data documents. Leave YAML/Markdown
buffered (or tackle the parser swap separately).
