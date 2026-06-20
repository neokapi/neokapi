---
sidebar_position: 16
title: OMML Math Conversion
description: Implementation note for AD-032 — the cgo-free core/math converter from Office Math Markup Language (OMML) to portable LaTeX/MathML via a texmath-shaped Exp AST, the namespace-wrapping token-stream reader, the operator dictionary that separates math typography from m:nor prose, the byte-offset nor-splice algorithm the OpenXML sub-skeleton consumes, the public API surface, and a coverage-gap ledger of the documented OMML approximations.
keywords: [OMML, Office Math, MathML, LaTeX, equation localization, m:nor, texmath, Exp AST, byte-exact splice, sub-skeleton, core/math, implementation note, neokapi]
---

# OMML Math Conversion

Implementation details for the math-conversion subsystem decided in
[AD-032: Math and Equations](../architecture/032-math-and-equations.md).
The package `core/math` (Go import `github.com/neokapi/neokapi/core/math`,
package name `math`) is a **cgo-free, WASM-safe** converter between ECMA-376
Part 1 §22.1 Office Math Markup Language (OMML) and two portable notations —
Presentation MathML and LaTeX — by way of a small intermediate AST. The host
format reader keeps the original OMML bytes verbatim for the byte-exact
round-trip (see [Skeleton Store](/contribute/notes-internal/skeleton-store));
this package only produces the *additional* portable renderings that
cross-format writers (markdown, DocLang, a future HTML writer) emit, and the
nor-prose splice that writes equation prose translations back into the original
bytes.

The package is deliberately tolerant: an OMML element it does not model degrades
to a best-effort row or is dropped, never failing a document read. `FromOMML`
returns a fatal error only for malformed XML.

## The Exp AST

`Exp` is a closed interface — a sealed sum type guarded by the unexported marker
method `isExp()`, so only the node types declared in `math.go` can satisfy it.
The serializers type-switch over this closed set, each with a defensive
`default` branch (an empty string for an unexpected node). The node set:

| Node | Shape | Renders as |
|---|---|---|
| `Number` | `Text string` | `<mn>` / digits |
| `Ident` | `Text string` | `<mi>` / letters |
| `Operator` | `Text string` | `<mo>` / operator glyph |
| `Text` | `Content string; Normal bool` | `<mtext>` / `\text{}` — `Normal` true marks `<m:nor/>` prose |
| `Row` | `Items []Exp` | concatenation / `<mrow>` when used as an argument |
| `Fraction` | `Num, Den Exp; NoBar bool` | `<mfrac>` / `\frac` (or `\atop` when `NoBar`) |
| `Superscript` | `Base, Sup Exp` | `<msup>` / `^{}` |
| `Subscript` | `Base, Sub Exp` | `<msub>` / `_{}` |
| `SubSup` | `Base, Sub, Sup Exp` | `<msubsup>` / `_{}^{}` |
| `Radical` | `Degree, Body Exp` (`Degree` nil = square root) | `<msqrt>`/`<mroot>` / `\sqrt` |
| `Nary` | `Chr string; Sub, Sup, Body Exp` | `<munder/over/underover>` + body / `\sum` etc. |
| `Delimited` | `Open, Close string; Body Exp` (`""` = invisible fence) | fenced `<mrow>` / `\left…\right` |
| `Function` | `Name string; Arg Exp` | `<mi>name</mi>` + applic. / `\sin` or `\operatorname` |
| `Matrix` | `Rows [][]Exp` | `<mtable>` / `\begin{matrix}` |
| `Accent` | `Accent string; Body Exp` | `<mover accent>` / `\hat` etc. |
| `Bar` | `Body Exp; Top bool` | `<mover>`/`<munder>` / `\overline`/`\underline` |
| `GroupChr` | `Chr string; Pos string; Body Exp` | `<mover>`/`<munder>` / `\overbrace`/`\underbrace` |
| `Raw` | `Content string` | `<mtext>` / `\text{}` — graceful-fallback literal |

`Math` is the parse result: `Body Exp` plus `Block bool`, where `Block`
distinguishes a display equation (`<m:oMathPara>`) from an inline one
(`<m:oMath>`). The internal helper `row(items)` collapses a single-element slice
to its element, otherwise returns a `Row`.

Note that `Raw` is **defined and serialized but never constructed** by the
reader — see the coverage ledger below.

## The OMML token-stream reader

The reader (`omml.go`) is a hand-written recursive descent over
`encoding/xml.Decoder` tokens rather than a struct-unmarshal, because OMML mixes
ordered structural children with property elements and foreign WordprocessingML
runs that must be skipped without disturbing position.

### Synthetic namespace wrapping

An OMML subtree captured from a `.docx` carries **no namespace declarations of
its own** — `xmlns:m` and `xmlns:w` sit on a distant ancestor (the document
part) that the captured fragment does not include. Decoding the bare fragment
leaves the `m:`/`w:` prefixes unbound, so nothing resolves to the math
namespace. Both the parser and the nor-scanner therefore wrap the raw fragment
in a synthetic root that binds the two prefixes before decoding:

```go
const mathNS = "http://schemas.openxmlformats.org/officeDocument/2006/math"
const wprNS  = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

wrapped := `<ommlRoot xmlns:m="`+mathNS+`" xmlns:w="`+wprNS+`">` + string(raw) + `</ommlRoot>`
```

The parser then drives the decoder to the first `oMathPara`/`oMath` start element
*in the math namespace* (`se.Name.Space == mathNS`) and reads its child sequence.
Element matching is by resolved `xml.Name` (namespace + local), via the `mn(local)`
helper, never by raw prefix string — so a fragment that happened to bind a
different prefix would still parse.

### Sequence, argument, property, and skip primitives

Four primitives structure the descent:

- `seq(end)` — reads sibling math nodes until the matching end element,
  dispatching each math-namespace start element through `node` and **consuming
  any foreign element** (e.g. `w:rPr`) with `skip`.
- `arg(end)` — reads one argument container (`e`, `num`, `den`, `sub`, `sup`,
  `deg`, `fName`, `lim`, …) as a single expression via `row(seq(...))`.
- `props(end)` — reads a `…Pr` property element, returning its direct
  math-namespace children keyed by local name to their `m:val` attribute (e.g.
  `naryPr` → `{chr: "∑"}`); valueless children appear with an empty-string value,
  which is how presence flags like `degHide`, `subHide`, `supHide`, and the
  `<m:nor/>` marker are detected (`_, ok := pr["degHide"]`). A property that
  instead carries an `m:val` — e.g. a fraction's `fPr` `type="noBar"` or a
  `naryPr` `chr` — is read by value.
- `skip(end)` — depth-counted consumption of an entire subtree.

`node` is the structural dispatch table, keyed on the element's local name:
`r`→`run`, `f`→`fraction`, `sSup`/`sSub`/`sSubSup`→`script`, `rad`→`radical`,
`nary`, `d`→`delim`, `func`→`function`, `m`→`matrix`, `acc`→`accent`, `bar`,
`groupChr`, `limLow`/`limUpp`→`limit`, `eqArr`, `sPre`, and the wrapper trio
`box`/`borderBox`/`phant` (which reduce to their inner `<m:e>`). The default case
consumes and drops the element.

## The operator dictionary and m:nor distinction

`opdict.go` holds both the run-text tokenizer and the glyph→LaTeX maps.

### Math typography vs. normal-text prose

The decisive split happens in `run` (parsing `<m:r>`): the run's optional
`<m:rPr>` is inspected for an `<m:nor/>` child. The outcome routes the run's
`<m:t>` text down one of two paths:

- **Normal text** (`<m:nor/>` present) — the text is natural-language prose
  embedded in the equation ("where", "otherwise", a unit). It is kept **whole**
  as `Text{Normal: true}` and is the *only* translatable surface. It bypasses
  the tokenizer entirely.
- **Math text** (no `<m:nor/>`) — the text is mathematical typography. It is
  tokenized by `classifyRunText` into `Number` / `Ident` / `Operator` nodes:
  a maximal run of digits (allowing an embedded `.` between digits) becomes a
  `Number`; a maximal run of letters becomes a single `Ident`; any other
  non-space rune becomes an `Operator`. Whitespace is dropped — math layout
  supplies spacing.

`TranslatableText()` / `collectNormalText` walks the AST and concatenates only
`Text` nodes with `Normal == true` (and non-blank content), space-joined, in
reading order. A pure-typography equation therefore yields the empty string and
contributes no translatable block.

### Glyph maps

LaTeX serialization consults three lookup tables, all in `opdict.go`:

- `naryLaTeX` — n-ary operator glyph → command (`∑`→`\sum`, `∫`→`\int`,
  `⋃`→`\bigcup`, …), consulted first for a `Nary.Chr`.
- `symbolLaTeX` — operators, relations, Greek letters, and set/logic glyphs
  authors type as Unicode inside math runs (`≤`→`\leq`, `→`→`\to`, `π`→`\pi`, …),
  used by `latexOp`/`latexSymbol` and as the `Nary` fallback.
- `accentLaTeX` — combining or spacing accent glyph → command (`̂`/`^`→`\hat`,
  `̄`→`\bar`, `⃗`→`\vec`, …).

A glyph absent from a table falls through to its literal text (operators) or to a
documented default (accents → `\hat`). `knownFuncs` (in `latex.go`) governs
whether a `Function.Name` renders as a backslash command (`\sin`) or
`\operatorname{…}`.

## The nor-splice algorithm

Translating equation prose requires writing a translation back into the *exact*
bytes of the original OMML so the surrounding math structure is untouched. This
is a byte-offset splice, implemented in `nor.go`.

### Byte-offset capture

`scanNorTexts` streams the namespace-wrapped fragment with a second, independent
`xml.Decoder` and tracks three booleans — `inRun` (inside `<m:r>`), `isNor`
(an `<m:nor/>` seen inside the current run), and `inMT` (inside `<m:t>`). When
character data arrives inside a nor-flagged `<m:t>`, it records the span. The
byte range is taken from `xml.Decoder.InputOffset()`:

- `mtStart` is captured at the `<m:t>` start element — i.e. the offset *just
  after* `<m:t>`, the first byte of element content;
- the end offset is `InputOffset()` at the `CharData` token — the byte just past
  the content.

Offsets are into the **wrapped** bytes. `wrapOMML` returns both the wrapped slice
and the prefix length, so the public `NorSpans` subtracts `prefixLen` to report
ranges into the caller's *raw* fragment. The `nor_test.go` contract asserts
exactly this: `raw[span.Start:span.End] == span.Text`.

### Byte-exact replacement

`SpliceNorText(raw, translations)` re-scans for the spans, builds a replacement
list (skipping entries that are empty, beyond the slice length, or equal to the
original text), XML-escapes each replacement with the same `esc` used for
element content, then rebuilds the output by copying verbatim between spans and
substituting inside them. The synthetic wrapper is stripped on return
(`out[prefixLen : len(out)-len(ommlSuffix)]`). Consequences, all asserted in
tests:

- A `nil` or all-empty `translations`, or translations equal to the originals,
  returns `raw` **byte-identical** (the replacement list is empty → early
  return of `raw`).
- A `translations` slice shorter than the span count leaves uncovered spans
  verbatim.
- Every non-prose byte — tags, `rPr`, the `<m:nor/>` markers, math runs — is
  preserved exactly.

### How the OpenXML sub-skeleton consumes the spans

The OpenXML writer does not call `SpliceNorText` directly; it drives the same
span set through the skeleton mechanism so a translated equation reproduces the
original bytes except where prose changes. `writeOMathSubSkeleton`
(`core/formats/openxml/omml_math.go`) calls `NorSpans(raw)`, **validates the
offsets** (monotonic, in range, `Start ≤ End`) before emitting anything — bailing
out to a verbatim write if they look wrong — then writes the equation to the
skeleton as alternating verbatim OMML segments (`raw[cursor:span.Start]`) and
skeleton refs to one `model.Block` per span, each typed `omml-nor`. On write,
the OpenXML writer's `renderBlock` renders an `omml-nor` block as bare
`xmlEscape`'d element-content text (matching `captureRawElement`'s CharData
escaping), so the ref resolves *inside* the `<m:t>…</m:t>`: untranslated ⇒
byte-exact, translated ⇒ in-place splice, math structure untouched. The
markdown and DocLang writers skip `omml-nor` blocks — the prose already rides
inside the formula's LaTeX.

This realizes the same parity guarantee the converter's pure splice does, via the
project's general skeleton machinery rather than a bespoke rewrite.

## Public API surface

| Symbol | Signature | Role | Wired into |
|---|---|---|---|
| `FromOMML` | `func([]byte) (*Math, error)` | Parse `<m:oMath>`/`<m:oMathPara>` into the AST; error only on malformed XML | `openxml` `ommlToMathEquiv` |
| `(*Math).ToLaTeX` | `func() string` | LaTeX, no `$`/`$$` delimiters | `openxml` (markdown `$..$`/`$$..$$` Equiv, DocLang `<formula>` Disp) |
| `(*Math).ToMathML` | `func() string` | Presentation MathML `<math>` element (`display="block"` when `Block`) | **wired-but-uncalled** — reserved for a future HTML writer |
| `(*Math).TranslatableText` | `func() string` | Concatenated `<m:nor/>` prose, space-joined, reading order; empty for pure math | unwired (round-trip tests only); the docx path surfaces prose via `NorSpans` |
| `(*Math).Block` | `bool` field | Display (`oMathPara`) vs inline (`oMath`) | selects `$$`/`$` delimiters and MathML `display` |
| `NorTexts` | `func([]byte) []string` | Each `<m:nor/>` run's text, document order | unwired (tests only) — enumeration convenience |
| `NorSpans` | `func([]byte) []NorSpan` | Prose text + byte offsets into `raw`, document order | OpenXML sub-skeleton (`writeOMathSubSkeleton`) |
| `SpliceNorText` | `func([]byte, []string) []byte` | Byte-exact replacement of nor-prose by document order | unwired (tests only); the docx path splices via the sub-skeleton over `NorSpans` |

`ToMathML` is fully implemented and unit-tested (`math_test.go` asserts its
`<mfrac>`, `<msup>`, `<munderover>`, `display="block"` output), but no writer
calls it yet — it exists so an HTML/MathML writer can adopt it without further
work in `core/math`. `NorTexts`, `TranslatableText`, and `SpliceNorText` are
likewise part of the public surface with no current production caller: the docx
write-back path enumerates and splices prose through `NorSpans` and the
sub-skeleton (above), so these standalone helpers serve callers that want the
conversion without the skeleton machinery and are exercised by `nor_test.go` /
`math_test.go`.

## Coverage-gap ledger

The converter trades completeness for tolerance and a small AST. The
approximations below are present in the code today; each is cited to its source.
None breaks a read — they affect only the fidelity of the portable rendering, not
the verbatim OMML round-trip (which the host keeps independently).

| # | Construct | Approximation | Source |
|---|---|---|---|
| 1 | Unmodeled elements | The `node` default case **drops** the element (`skip` → `nil`). `Raw` is defined as the "graceful fallback" literal and is rendered by both serializers, but the reader **never constructs it** — graceful degradation is "drop", not "keep as `Raw`". | `omml.go` `node` default; `math.go` `Raw`; `mathml.go`/`latex.go` `Raw` cases (unreachable from `FromOMML`) |
| 2 | `limLow` / `limUpp` | Modeled as a plain `Subscript` / `Superscript`. Under-/over-limit positioning collapses to an inline script (`base_{lim}` / `base^{lim}`), losing `\underset`/`\overset`-style stacking. | `omml.go` `limit` |
| 3 | `sPre` (pre-scripts) | Modeled as `SubSup{Base: Row{}}` emitted *before* the base — yields `{}_{sub}^{sup}base`, an approximation of true pre-script / tensor positioning. | `omml.go` `sPre` |
| 4 | Delimiter separators | Multiple `<m:e>` operands in `<m:d>` are always joined with a literal `\|` operator (`Operator{Text: "\|"}`); the `dPr` `sepChr` is **not read** despite the inline comment mentioning it. | `omml.go` `delim` |
| 5 | Matrix / `eqArr` alignment | Matrix column properties and cell justification (`mPr`/`mcJc`) are skipped; `eqArr` is modeled as a one-column `Matrix`, losing its `&` alignment points. LaTeX is always `\begin{matrix}` (no fence/alignment variant). | `omml.go` `matrix`, `eqArr`; `latex.go` `Matrix` |
| 6 | `box` / `borderBox` / `phant` | Reduced to their inner `<m:e>`; border-box rendering and phantom (invisible-spacing) semantics are dropped. | `omml.go` `node` (`box`/`borderBox`/`phant` → `arg`) |
| 7 | Run-text tokenization | Whitespace inside a math `<m:t>` is dropped; a maximal letter run becomes a single `Ident`, so a typed multi-letter token (e.g. `sin` not wrapped in `<m:func>`) renders as one identifier rather than a recognized function. | `opdict.go` `classifyRunText` |
| 8 | Unknown accent glyph | An accent glyph absent from `accentLaTeX` falls back to `\hat` in LaTeX (lossy default). | `latex.go` `Accent`; `opdict.go` `accentLaTeX` |
| 9 | `GroupChr` in LaTeX | The actual group character (`Chr`) is ignored in LaTeX — output is always `\overbrace`/`\underbrace` chosen by `Pos`. (MathML preserves the glyph.) | `latex.go` `GroupChr` vs. `mathml.go` `GroupChr` |

## Related

- [AD-032 Math and Equations](../architecture/032-math-and-equations.md) — the decision this note implements
- [AD-002 Content Model](../architecture/002-content-model.md) — `Block`, `Run`, `Ph` (placeholder `Equiv`/`Disp`), and `SemanticRole`
- [AD-018 Parity testing against Okapi](../architecture/018-parity-testing.md) — why `Ph.Equiv`/`Ph.Disp` and `SemanticRole` are parity-safe carriers
- [Skeleton Store and Streaming HTML](/contribute/notes-internal/skeleton-store) — the skeleton mechanism the OMML sub-skeleton rides on
