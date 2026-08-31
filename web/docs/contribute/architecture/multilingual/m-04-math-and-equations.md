---
id: m-04-math-and-equations
sidebar_position: 4
title: "M-04: Math and equations"
description: "The authoritative equation markup is captured and replayed byte-for-byte while a cgo-free converter projects it into portable MathML and LaTeX on parity-safe carriers, and the natural-language prose inside an equation is made translatable through a sub-skeleton that splices the translation back in place."
keywords: [neokapi, architecture decision, math, equations, OMML, MathML, LaTeX, formula, sub-skeleton, byte-exact]
---

# M-04: Math and equations

## Summary

An equation in a document is content. A display equation is context an
ingestion pipeline wants to read, and it can itself contain natural-language
prose (a "where", an "otherwise", a spelled-out unit) that has to be translated.
neokapi treats math as first-class content without ever corrupting the
authoritative source markup.

The design is a separation of concerns:

- The **authoritative markup stays verbatim.** OMML (Office Math Markup
  Language, ECMA-376 Part 1 §22.1) is captured byte-for-byte and replayed
  byte-for-byte. The round trip never serializes a parsed model back into the
  document.
- A **cgo-free converter** parses OMML into a small portable AST and renders
  Presentation MathML and LaTeX: a *projection* used only to produce additional
  portable renderings, never to reconstruct the source.
- Those renderings ride on **parity-safe placeholder carriers**, so a
  cross-format writer can emit math in each target's native idiom while
  head-to-head parity output stays byte-identical.
- **Standalone equations surface as non-translatable formula blocks**, so
  ingestion sees the whole formula and a cross-format export can render it.
- The **prose inside an equation** is made translatable through a skeleton
  **sub-skeleton** that splices the translation into the original OMML in place,
  leaving every other byte untouched.

## Why it is layered this way

Three properties of math shape the design.

**Math is context.** A formula carries meaning that downstream ingestion
benefits from reading. The classification a reader applies to any
non-translatable-but-meaningful fragment (surface it, do not bury it in the
skeleton) applies to an equation as much as to a code block or a caption
([E-02](../engine/e-02-format-system.md)). An equation buried opaquely is
context lost.

**Math can contain translatable prose.** OMML marks upright natural-language
text inside an equation with a normal-text element: the "where" clauses,
"otherwise" branches, and spelled-out units an author writes alongside the
symbols. That prose is genuine translatable surface; the surrounding symbolic
typography is not. Translating the prose while leaving the structure exact is a
sub-document problem, not a whole-equation one.

**Conversion is necessarily tolerant and lossy.** OMML to LaTeX is a projection
between two notations with different coverage. A construct the converter does
not model must degrade gracefully rather than fail a document read, and the
result must never be treated as authoritative. So the original OMML remains the
source of truth and the round trip replays *it*, never a re-serialization of the
AST, which is what makes an approximation in the converter unable to mangle a
document. Because the same conversion also runs in the browser labs, where no
cgo is available, the converter is pure Go with no native dependency.

## The converter

`core/math` reads OMML once into a tree of `Exp` nodes and serializes that tree
to any number of target notations. `Exp` is a sealed interface, a closed union
of concrete node types (numbers, identifiers, operators, fractions, scripts,
radicals, n-ary operators, delimited groups, matrices, accents, group
characters) marked by an unexported method.

```go
type Exp interface{ isExp() }

type Math struct {
    Body  Exp
    Block bool // display vs inline
}

func FromOMML(raw []byte) (*Math, error) // tolerant: unmodeled → Raw/Row; error only on malformed XML

func (m *Math) ToMathML() string         // Presentation MathML
func (m *Math) ToLaTeX() string          // LaTeX, no delimiters
func (m *Math) TranslatableText() string // the normal-text prose, in reading order
```

`FromOMML` is deliberately tolerant: an element it does not model degrades to a
best-effort node rather than failing, so a partial conversion never breaks a
document read; an error comes back only for malformed XML. The known coverage
approximations live as a ledger in the paired implementation note, not here.

Translating the embedded prose does not go through the AST at all. A separate,
byte-oriented engine works directly on the raw OMML so every non-prose byte is
preserved exactly:

```go
type NorSpan struct {
    Text       string
    Start, End int // byte offsets of the text CharData within the raw OMML
}

func NorTexts(raw []byte) []string
func NorSpans(raw []byte) []NorSpan
func SpliceNorText(raw []byte, translations []string) []byte // byte-exact in-place splice
```

The splice replaces each prose span's CharData with its translation by document
order, XML-escaping the replacement and copying every other byte verbatim; an
empty or short slice leaves those spans untouched, so a no-op call returns the
input unchanged. It never round-trips through a serializer, which is why the
math structure is guaranteed intact.

## Capture and surface

The OpenXML reader captures an OMML subtree as a paragraph-opaque sentinel run
carrying the raw markup verbatim in the placeholder's data. How the equation is
then surfaced depends on where it sits in the paragraph:

| Position | Surfaced as | Carrier |
| --- | --- | --- |
| **Inline**, alongside the paragraph's own text | a placeholder run (type `struct:opaque-para-child`, subtype `openxml:oMath`) | `Ph.Data` (raw OMML) + `Ph.Equiv` (delimited LaTeX) + `Ph.Disp` (bare LaTeX) |
| **Standalone**, an equation-only paragraph | a detached non-translatable formula block | a placeholder run carrying the same data, equiv and disp |

The conversion produces two renderings from the captured markup: `Equiv` is
LaTeX wrapped in markdown math delimiters for writers that need a
self-delimiting form, `Disp` is the bare LaTeX for writers that supply their own
math context. Both ride on the placeholder's own fields, never mixed into its
data.

The standalone formula block is **not** skeleton-referenced: the paragraph's
bytes, or its sub-skeleton (below), already round-trip from the skeleton, so
the detached block exists purely as an export carrier. Surfacing is gated by the
reader's non-translatable-content setting, which is on by default; with it off,
the renderings are empty, no standalone block is emitted, and the markup is
replayed verbatim from the skeleton.

## The carriers are parity-safe

The placeholder's equivalence and display fields, and a block's semantic role,
are excluded from the canonical parity projection
([A-02](../assurance/a-02-parity.md)). Attaching portable renderings to a
placeholder and tagging a block as a formula therefore leaves head-to-head
output byte-identical, and the parity runner additionally forces the surfacing
setting off so it is absent from the comparison entirely.

Independently, the byte-exact document round trip replays the placeholder's raw
data and never a re-serialization of the AST, so the converter's approximations
cannot corrupt a document. The full projection contract belongs to
[A-02](../assurance/a-02-parity.md); the principle here is only that these three
fields are parity-safe carriers.

## Translatable prose inside an equation

When an equation carries prose, the reader writes it to the skeleton as a
**sub-skeleton**: verbatim markup segments interleaved with skeleton references
to one translatable block per prose span. The contract has two halves:

- **Untranslated**: each reference resolves to its block's source text, which
  the writer XML-escapes back into place, reproducing the original equation
  byte for byte.
- **Translated**: the reference resolves to the target, splicing the
  translation into place; the surrounding math structure is untouched.

Offsets are validated (monotonic and in range) before any block is emitted;
otherwise the reader falls back to writing the equation verbatim, so a
malformed capture costs the translatability of that one equation and nothing
else.

The sub-skeleton mechanism itself belongs to
[E-02](../engine/e-02-format-system.md) and the
[Skeleton Store](/contribute/implementation/engine/skeleton-store) note. What this
decision fixes is only the contract: the prose is translatable while the math is
byte-exact.

## Cross-format rendering

Because the portable renderings travel on the placeholder, an equation survives
format-to-format conversion ([S-04](../surfaces/s-04-toolbox.md)) rendered into
each target's native math idiom:

- **markdown** emits the delimited form: LaTeX inside markdown math delimiters.
- **DocLang** emits the bare form inside a formula element, which is what that
  format mandates there.

Both writers **skip** the prose blocks: the prose already rides inside the
formula's LaTeX as a text group, so emitting the spans again would duplicate it.

Inbound, the symmetry holds. Markdown inline math is read as an inline code run
whose data carries the MathML markup, so math authored in one format is
recognizable to editors and preview in another.

## Related

- [F-02: The content model](../foundations/f-02-content-model.md): the placeholder equivalence and display fields, semantic roles, and the inline math vocabulary these carriers use
- [E-02: The format system](../engine/e-02-format-system.md): the skeleton and the sub-skeleton mechanism, and the surfacing of non-translatable content
- [S-04: The toolbox](../surfaces/s-04-toolbox.md): the cross-format conversion that renders equations into each target's math idiom
- [A-02: Parity](../assurance/a-02-parity.md): why these carriers are excluded from the parity projection
- [OMML math](/contribute/implementation/multilingual/omml-math): the coverage-approximation ledger, the AST node mapping, and the splice algorithm in detail
