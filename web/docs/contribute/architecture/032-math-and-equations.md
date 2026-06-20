---
id: 032-math-and-equations
sidebar_position: 32
title: "AD-032: Math and Equations"
description: "Architecture decision: equations are first-class localizable content. A cgo-free converter (core/math) parses OMML (ECMA-376 Part 1 §22.1) into a portable AST and renders Presentation MathML and LaTeX; the host format keeps the original OMML bytes verbatim for byte-exact round-trip and parity, carries the portable renderings on parity-safe placeholder fields (Ph.Equiv/Ph.Disp), surfaces standalone equations as non-translatable RoleFormula blocks for cross-format export (markdown $..$/$$..$$, DocLang <formula>), and makes the natural-language prose inside an equation (<m:nor/>) translatable through a skeleton sub-skeleton that splices translations back byte-exactly."
keywords: [math, equations, OMML, OMath, ECMA-376, MathML, LaTeX, core/math, formula, RoleFormula, sub-skeleton, m:nor, nor splice, byte-exact, parity-safe, cross-format export, DocLang, markdown math, architecture decision, neokapi]
---

# AD-032: Math and Equations

## Summary

An equation in a document is content, not decoration. A Word display equation
is context an ingestion pipeline (LLM/RAG) wants to read, and it can itself
contain natural-language prose — "where", "otherwise", a unit — that must be
translated. neokapi treats math as first-class localizable content without ever
corrupting the authoritative source markup.

The design rests on a separation of concerns:

- The **authoritative math markup stays verbatim.** OMML (Office Math Markup
  Language, ECMA-376 Part 1 §22.1) is captured byte-for-byte and replayed
  byte-for-byte. The round-trip never serializes a parsed model back into the
  document.
- A **cgo-free converter** (`core/math`) parses OMML into a small portable AST
  and renders Presentation MathML and LaTeX — a *projection* used only to
  produce additional, portable renderings, never to reconstruct the source.
- Those renderings ride on **parity-safe placeholder carriers**
  (`Ph.Equiv`/`Ph.Disp`), so cross-format writers can emit math in each target's
  native idiom while head-to-head parity output stays byte-identical to the
  bridge.
- **Standalone equations surface as non-translatable `RoleFormula` blocks**, so
  ingestion sees the whole formula and cross-format export can render it.
- The **natural-language prose inside an equation** (`<m:nor/>`) is made
  translatable through a skeleton **sub-skeleton** that splices the translation
  into the original OMML in place, leaving every other byte untouched.

## Context

Two properties of math collide with a faithfulness-first tool, and a third
constrains where the code may run.

**Math is context.** A formula carries meaning that downstream LLM/RAG ingestion
benefits from reading. The classification a reader applies to any
non-translatable-but-meaningful fragment — surface it, do not bury it in the
skeleton — applies to equations as much as to code blocks or captions
([AD-031](031-content-fidelity-surfacing.md)). An equation buried opaquely is
context lost.

**Math can contain translatable prose.** OMML marks upright natural-language
text inside an equation with `<m:nor/>` — the "where" clauses, "otherwise"
branches, and spelled-out units an author writes alongside the symbols. That
prose is genuine translatable surface; the surrounding symbolic typography is
not. Localizing the prose while leaving the structure exact is a sub-document
problem, not a whole-equation one.

**Conversion is necessarily tolerant and lossy.** OMML → LaTeX is a projection
between two notations with different coverage; an OMML construct the converter
does not model must degrade gracefully rather than fail a document read, and the
result must never be treated as authoritative. The original OMML therefore
remains the source of truth, and the round-trip replays *it*, not a
re-serialization of the AST — so an approximation in the converter can never
mangle a `.docx`. Because the same conversion must also run in the browser labs,
where no cgo is available, `core/math` is pure Go with no native dependency.

## Decision

The design is layered: a standalone converter that knows nothing about
documents; the OpenXML host's capture-and-surface model; the carrier/parity
contract; the sub-skeleton that localizes embedded prose; and cross-format
rendering.

### The core/math converter

`core/math` (package `math`, cgo-free, WASM-safe) is a converter between OMML and
portable math notations via a small intermediate AST, shaped after Pandoc's
texmath: read OMML once into a tree of `Exp` nodes, then serialize that tree to
any number of target notations. `Exp` is a sealed interface — a closed union of
concrete node types (numbers, identifiers, operators, fractions, scripts,
radicals, n-ary operators, delimited groups, matrices, accents, …) marked by an
unexported `isExp()`.

```go
type Exp interface{ isExp() }

type Math struct {
    Body  Exp
    Block bool // display (<m:oMathPara>) vs inline (<m:oMath>)
}

func FromOMML(raw []byte) (*Math, error) // tolerant: unmodeled → Raw/Row; err only on malformed XML

func (m *Math) ToMathML() string         // Presentation MathML (<math> element)
func (m *Math) ToLaTeX() string          // LaTeX, no $ / $$ delimiters
func (m *Math) TranslatableText() string // concatenated <m:nor/> prose, reading order
```

`FromOMML` is deliberately tolerant: an element it does not model degrades to a
best-effort `Row`/`Raw` node rather than failing, so a partial conversion never
breaks a document read; an error is returned only for malformed XML. `ToMathML`
is wired but currently uncalled — reserved for a future HTML writer — and only
`ToLaTeX` is consumed by the OpenXML host today. The known OMML coverage
approximations live as a ledger in the paired note, not here.

Localizing the embedded prose does not go through the AST at all. A separate,
byte-oriented engine works directly on the raw OMML so that every non-prose byte
is preserved exactly:

```go
type NorSpan struct {
    Text       string
    Start, End int // byte offsets of the <m:t> CharData within the raw OMML
}

func NorTexts(raw []byte) []string                       // the <m:nor/> prose, in document order
func NorSpans(raw []byte) []NorSpan                       // the same prose with byte offsets
func SpliceNorText(raw []byte, translations []string) []byte // byte-exact in-place splice
```

`SpliceNorText` replaces each `<m:nor/>` `<m:t>` CharData with its translation
(by document order), XML-escaping the replacement and copying every other byte
verbatim; an empty or short `translations` slice leaves those spans untouched, so
a no-op call returns `raw` unchanged. The splice never round-trips through the
serializer, which is why the math structure is guaranteed intact.

### Capture and surface in OpenXML

The OpenXML reader captures an OMML subtree as a **paragraph-opaque sentinel
run** (`sentinelParaOpaque`, U+E105) carrying the raw OMML verbatim in the
placeholder's `Data`. How the equation is then surfaced depends on its position
in the paragraph:

| Equation position | Surfaced as | Carrier |
|---|---|---|
| **Inline** — sits in a `<w:p>` alongside translatable text | a placeholder run (`Type` `struct:opaque-para-child`, `SubType` `openxml:oMath`) | `Ph.Data` (raw OMML) + `Ph.Equiv` (markdown-delimited LaTeX) + `Ph.Disp` (bare LaTeX) |
| **Standalone** — an equation-only paragraph | a detached **non-translatable `RoleFormula` block** | a placeholder run carrying the same `Ph.Data`/`Equiv`/`Disp` |

`ommlToMathEquiv` produces the two renderings from the captured OMML:
`Equiv` is LaTeX wrapped in markdown math delimiters (`$…$` inline, `$$…$$`
display) for writers that need a self-delimiting form; `Disp` is the bare LaTeX
for writers that supply their own math context. Both ride on the placeholder's
`Equiv`/`Disp`, never mixed into `Ph.Data`.

The standalone `RoleFormula` block is **not** skeleton-referenced: the
paragraph's bytes (or its `<m:nor/>` sub-skeleton, below) already round-trip from
the skeleton, so the detached block exists purely as an export carrier. Surfacing
is gated by `extractNonTranslatableContent` (default ON,
[AD-031](031-content-fidelity-surfacing.md)); with the flag off, `Equiv`/`Disp`
are empty, the standalone block is not emitted, and the OMML is replayed verbatim
from the skeleton.

### Carriers are parity-safe

`Ph.Equiv`, `Ph.Disp`, and a block's `SemanticRole` are excluded from the
canonical parity projection ([AD-018](018-parity-testing.md)). Attaching portable
renderings to a placeholder and tagging a block `RoleFormula` therefore leaves
head-to-head output byte-identical to the okapi-bridge, and the parity runner
additionally forces `extractNonTranslatableContent` off so the surfacing is
absent from the comparison entirely. Independently, the byte-exact `.docx`
round-trip replays `Ph.Data` — the raw OMML — and never a re-serialization of the
AST, so the converter's approximations cannot corrupt a document. The full
projection contract is in [AD-018](018-parity-testing.md); the principle here is
only that `Equiv`/`Disp`/`SemanticRole` are parity-safe carriers.

### Translatable prose inside an equation

When an equation carries `<m:nor/>` prose, `writeOMathSubSkeleton` writes the
equation to the skeleton as a **sub-skeleton**: verbatim OMML segments
interleaved with skeleton refs to one translatable `omml-nor` block per prose
span. The contract:

- **Untranslated** — each ref resolves to its block's source text, which the
  writer XML-escapes back into the `<m:t>`, reproducing the original equation
  byte-for-byte.
- **Translated** — the ref resolves to the target, splicing the translation into
  the `<m:t>` in place; the surrounding math structure is untouched.

Offsets are validated (monotonic and in range) before any block is emitted;
otherwise the reader falls back to writing the equation verbatim. The
sub-skeleton store mechanism itself is described in
[AD-005](005-format-system.md) and the
[Skeleton Store](/contribute/notes-internal/skeleton-store) note; this AD fixes
only the contract that prose is localizable while the math is byte-exact.

### Cross-format rendering

Because the portable renderings travel on the placeholder, an equation survives
format-to-format conversion (`kconv`, [AD-023](023-toolbox-utilities.md)) rendered
into each target's native math idiom:

- **markdown** emits `Ph.Equiv` — LaTeX in markdown math delimiters.
- **DocLang** emits `Ph.Disp` — bare LaTeX inside a `<formula>` element (DocLang
  mandates undelimited LaTeX there).

Both writers **skip** `omml-nor` blocks: the prose already rides inside the
formula's LaTeX (as `\text{…}`), so emitting the spans again would duplicate it.
Inbound, the symmetry holds: markdown inline `<math>` is read as an inline
`fmt:math` / `md:math-inline` code whose `Data` carries the MathML markup, so
math authored in one format is recognizable to editors and preview in another.

## Related

- [AD-002: Content Model](002-content-model.md) — the `Equiv`/`Disp` run fields, `SemanticRole` (`RoleFormula`), and the `fmt:math` inline vocabulary these carriers use
- [AD-005: Format System](005-format-system.md) — the skeleton and the sub-skeleton mechanism that localizes `<m:nor/>` prose byte-exactly
- [AD-018: Parity testing against Okapi](018-parity-testing.md) — why `Equiv`/`Disp`/`SemanticRole` are parity-safe carriers
- [AD-023: Toolbox Utilities](023-toolbox-utilities.md) — `kconv` cross-format conversion that renders equations into each target's math idiom
- [AD-031: Content-Fidelity Surfacing](031-content-fidelity-surfacing.md) — surfacing standalone equations is an instance of content-fidelity surfacing
- [OMML Math](/contribute/notes-internal/omml-math) — the OMML coverage-approximation ledger, the AST node mapping, and the splice algorithm in detail
