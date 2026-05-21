# OpenXML faithful-writer design — closing the residual parity bugs

Design note for closing the three open OpenXML parity bugs (#597, #598
residual, 847-3) and the broader residual-divergent class (the residual-7
plus the `1083-*` / `1145-*` clusters) **architecturally**, favouring
ECMA-376 / ISO/IEC 29500 spec compliance over reproducing Okapi's
implementation, while *simplifying* the writer rather than porting more of
Okapi into it.

Companion to [`core/formats/openxml/PARITY_NOTES.md`](../../../core/formats/openxml/PARITY_NOTES.md)
(per-iteration working notes) and a sibling of
[`xliff-okapi-compat-quirks.md`](xliff-okapi-compat-quirks.md) — this is the
same "faithful default + opt-in okapi-compat" pattern, applied to OpenXML.

## TL;DR

The native OpenXML **writer reconstructs** WordprocessingML run-by-run and
then runs **WSO** ("Word Style Optimisation", `style_optimization.go`) to
**mimic Okapi's compact output** — synthesise a paragraph style from each
paragraph's common run properties, strip "moot" attributes, rename
font-subsets. WSO is **not required by the spec**: native is already a valid
ECMA-376 producer on 183-184/185 fixtures *without* it. WSO is the source of
all three open bugs plus the residual class, and its global, order-coupled
synth-style ID counter is the "architectural blocker."

**The fix is to delete WSO, not extend it.** Make the writer faithful
(preserve source `rPr`/styles, only substitute translated text — the
`OptimiseWordStyles=false` path that already exists), and move the
"these two formatting representations are equivalent" knowledge out of the
*writer* and into the *comparator* as a principled **effective-rPr
normalizer** (resolve `docDefaults` → style chain → `pStyle`/`rStyle` →
direct `rPr` → toggle props into effective per-run formatting). Fix the
reader's §17.16 complex-field state machine separately (it's a real
data-loss bug, independent of the writer).

Empirically validated (see [Validation](#validation)): with WSO off the
suite produces **valid output for all 185 fixtures with zero errors** and
**identical translatable text** (no data loss); the 66 fixtures that go
canon→div do so **purely on style-indirection** (Okapi `<w:pStyle>`
reference vs native inline `rPr`), which an effective-rPr normalizer
resolves.

## The three bugs, in spec terms

| Bug | Symptom | Spec | Subsystem |
|---|---|---|---|
| **#597** `delTextAmp` | single-char run loses `<w:spacing w:val="-2"/>`; `<w:rFonts w:cs="Helvetica"/>` injected | **real violation** — alters rendering (§17.3.2.35) | WSO rPr rewrite |
| **#598a** `830-7` | translatable text between `<w:fldChar end/>` and a following `<w:fldChar begin/>` in one run captured opaquely (untranslated) | **real data loss** (§17.3.2.1 CT_R; §17.16.5) | reader field machine |
| **#598b** `830-7` | run-level redundant `<w:color w:val="000000"/>` stripped though `docDefaults` declares no color | byte-shape; both render same | WSO attr strip |
| **847-3** | cross-paragraph field continuation differs in byte shape; *blocked* by synth-ID cascade | byte-shape; both spec-valid | WSO synth-ID coupling + reader field machine |

Only **#597** and **#598a** are genuine spec/correctness defects. #598b and
847-3 are byte-shape differences against Okapi's compact form — both
producers are spec-compliant and render identically (ECMA-376-1 §17.3.2.1
+ §17.7: direct formatting and style-based formatting are equally valid;
lifting common `rPr` into a synthesised style is *one permitted producer
choice*, not a requirement).

## Root cause

`core/formats/openxml`:

1. **WSO is Okapi-mimicry** (`style_optimization.go`, gated by
   `Config.OptimiseWordStyles`, default `true`). It runs as a **post-pass**
   on already-emitted faithful WML bytes: synthesises `NF…-NormalN` pStyles
   from common run `rPr`, strips moot `rFonts`/`color`, etc. Its job is to
   make native byte-match Okapi's *compact* output — nothing the spec asks
   for. It is the source of **#597** (rPr rewrite drops `spacing`, injects
   `rFonts` via the `commonRFonts` docDefaults overlay) and **#598b** (attr
   strip).
2. **WSO is globally order-coupled.** Synth-style IDs come from a single
   **monotonic counter** (`idCounter` in `writer.go:Write`, mirroring
   Okapi's `IdGenerator`). Byte-equality requires native to visit
   paragraphs in Okapi's exact order *and* make the exact same
   synthesise/skip decision per paragraph, or every subsequent ID shifts —
   so any per-paragraph fix cascades into the 178 passing fixtures. This is
   the **847-3 "architectural blocker."**
3. **Per-run `rPr` is an out-of-band sidecar** (`openxml-per-run-rpr` et al.
   on `Block.Annotations`), aligned to text-bearing runs by position, with
   a guard that **silently drops the sidecar** (falling back to the
   paragraph-common subset) when the count mis-aligns (`writer.go` ~2334).
   When the guard fires, runs lose distinctive formatting — the `1083-*`
   (rStyle) and `1145-*` (per-run colour) clusters.
4. **The reader's complex-field machine does eager opaque capture**: once a
   field is active it captures whole runs opaquely
   (`parseRunWithFieldState`, `wml.go:3385`), so legitimately-translatable
   body text in the same run (the `end → text → begin` window) is lost —
   **#598a**.

## Spec framing (the north star)

ECMA-376-1 / ISO/IEC 29500-1:

- **§17.3.2.1 (CT_R)**: a run is an optional `rPr` plus a sequence of
  run-content children; every `rPr` child applies to the whole run. Text
  before a `begin` or after an `end` marker in the same run is ordinary
  body text.
- **§17.7 (styles)**: effective run formatting = `docDefaults` → style
  chain (`basedOn`) → paragraph `pStyle` `rPr` → character `rStyle` →
  direct `rPr` → toggle-property XOR rules. Direct and style-based
  formatting are equally valid; a conforming consumer resolves both to the
  same effective formatting.
- **§17.16 (fields)**: complex field = `fldChar begin` … `instrText` …
  `fldChar separate` … *field result* … `fldChar end`; fields may span
  paragraphs and nest. The instruction is a formula (not user content); the
  result is the cached display value.

⇒ **Native's faithful, source-preserving output is fully spec-compliant.**
The only real spec violation among the bugs (#597) is a *symptom of WSO*,
not of preserving source. Spec-first therefore means: stop re-deriving;
preserve source; prove equivalence-with-Okapi in the comparator, not the
producer.

## Options explored

| # | Option | Closes | Verdict |
|---|---|---|---|
| 1 | Faithful-default writer, WSO opt-in | #597, #598b, 847-3 cascade (in product) | **keep** — toggle already exists |
| 2 | Model per-run `rPr` as inline codes in the Fragment | F1 (per-run rPr loss) | **reject** — architecturally invasive (~30 `rPr` child types, translator opacity); the sidecar already exists and degrades gracefully |
| 3 | "Sound WSO": content-address synth IDs + faithful `RunFonts`/`RunMerger` port | #597, 847-3 cascade, residual-7 | **reject** — this is "port more Okapi"; *adds* complexity; IDs must still match Okapi visit order |
| 4 | **Synthesis: 1 + effective-rPr comparator + §16 field machine + delete WSO** | all 3 bugs + residual-7 + 1083/1145 | **adopt** — a net *deletion*, spec-first, reuses the established xliff pattern |

Option 2 was the initial instinct (it's how Okapi/XLIFF model runs) but the
code map showed the per-run `rPr` sidecar is already wired (`effectiveRPr(idx)`
is a clean seam) and inline-coding ~30 `rPr` child types would make the
Fragment opaque to translators for no practical gain — pseudo and
unit-segment MT/AI both preserve run identity, and the sidecar already falls
back gracefully. So F1 is "harden the existing sidecar emission," not a
content-model overhaul.

## The solution (Option 4)

### 4a. Faithful-default writer (WSO opt-in → eventually deleted)

`Config.OptimiseWordStyles` already gates WSO as a clean post-pass; the
faithful pre-WSO output (`renderWMLBlock` + `postNonWSOForName`) is already
the natural intermediate. **Flip the default to `false`.** Native then ships
faithful output: source `rPr` preserved inline, no synthesised styles, no
attr stripping.

- Closes **#597** (no rPr rewrite → `spacing` preserved; no `commonRFonts`
  overlay → no injected `rFonts`), **#598b** (no strip → `color` preserved),
  and removes **847-3's** synth-ID cascade (no synthesis → no counter).
- WSO is retained behind the flag during migration, then **deleted** once
  the comparator (4b) lands — removing `style_optimization.go`'s synth-style
  machinery, `idCounter`, `commonRFonts`, and the related run-merge/strip
  passes. This is the simplification the goal asks for.

### 4b. Effective-rPr canonical normalizer (the keystone "missing part")

A parity-only normalizer (`cli/parity/roundtrip/normalizers.go`) that, for
each `word/*.xml` part, resolves **both** sides to **effective per-run
formatting** before comparison: parse `styles.xml` + `docDefaults`, resolve
`pStyle`/`rStyle`/`basedOn` chains and toggle properties, inline the
resolved `rPr` onto each run, and drop now-redundant synthesised styles.
Then native-inline ≡ Okapi-synth-style → **canonical-equal**, with no WSO in
the producer. This implements ECMA-376 §17.7 style resolution and is
symmetric, principled, and reusable (it is the spec's own equivalence
relation). The reader already has style-resolution machinery
(`styleMap.effectiveProps`) to reuse.

This replaces WSO's role *in the comparison*. The 66 currently-WSO-dependent
fixtures stay canon (see [Validation](#validation)).

### 4c. Spec §17.16 complex-field state machine (reader; independent)

Replace "eager opaque capture once field-active" with a state machine that
extracts translatable body text wherever it legitimately appears — before
`begin`, in the `end → text → begin` window, and the field *result* between
`separate` and `end` per policy — and tracks field depth across paragraphs
and nesting. Field markup (`fldChar`, `instrText`) stays opaque
skeleton/`Ph` codes. Closes **#598a** data loss and the extraction side of
**847-3**. This is product-correct regardless of the writer mode.

### 4d. Harden per-run `rPr` emission (writer; clears 1083/1145)

Ensure the writer reliably emits **one `<w:r>` per text run** with its
`effectiveRPr(idx)` and that the sidecar alignment guard does not silently
fall back. With WSO gone, this is the whole formatting-fidelity story; the
channel already exists, so this is writer-internal hardening (close the run
on per-run `rPr` divergence between adjacent text runs — `writer.go` slow
path), not new architecture.

### Parity wiring note

With faithful as the **default**, the parity harness needs no writer
overlay to get faithful output, so the openxml writer does **not** need to
implement the harness `WriterConfigurable` interface (the spike showed it
currently doesn't — only relevant if we wanted an opt-in WSO-on parity
mode, which deleting WSO removes the need for).

## How each bug closes

| Bug | Closed by | Result |
|---|---|---|
| #597 spacing/rFonts | 4a (WSO off) | spec-correct: source `rPr` preserved inline |
| #598a field text loss | 4c (field machine) | translatable text extracted; no data loss |
| #598b color strip | 4a (WSO off) | source `color` preserved |
| 847-3 cascade + shape | 4a (no synth IDs) + 4c (field extraction) | faithful output; cascade eliminated |
| 1083-* / 1145-* | 4a + 4d | per-run `rPr` preserved |
| residual-7 class | 4a + 4b | faithful output; equivalence proved by normalizer |

## Validation

Empirical spike (2026-05-22), openxml parity suite with WSO forced off
(`OptimiseWordStyles=false`):

```
WSO ON  (baseline): native openxml  0 byte / 175 canon / 10 div
WSO OFF (faithful): native openxml  0 byte / 109 canon / 76 div
                    suite ran OK — zero writer errors on all 185 fixtures
```

- **Valid output, no data loss**: the suite produced valid WML for all 185;
  on `1200-4.docx` the extracted translatable text is **byte-identical**
  between native(WSO-off) and Okapi (same `<w:r>`/`<w:t>` counts).
- **The 66 canon→div are pure style-indirection**: the *only* difference on
  `1200-4` is Okapi emits `<w:pStyle w:val="NF974E24F-Normal1"/>` + the
  synth def in `styles.xml`, while native keeps the formatting inline —
  semantically identical. The divergence reasons cluster in `word/styles.xml`
  (synth styles native no longer creates) and `word/document.xml` (inline
  `rPr` vs `pStyle` ref), with modest Δ (213–952 B).

⇒ An effective-rPr normalizer (4b) that resolves the synth `pStyle`/style
chain to effective per-run formatting recovers these 66 to canon, confirming
WSO is removable.

## Implementation plan & sequencing

1. **4c field machine** (independent, product-correct, highest spec value):
   fix `parseRunWithFieldState` to extract the `end → text → begin` body
   window; verify cross-paragraph (`partCfs` + `partFieldStraddle`). Closes
   #598a now, no parity-tier risk.
2. **4b effective-rPr normalizer**: build in `cli/parity/roundtrip`; verify
   it brings the WSO-off fixtures to canon (target: openxml → ~all canon).
3. **4a flip default** `OptimiseWordStyles=false`; retire/adjust unit tests
   that assert WSO-on output; confirm parity via 4b. Closes #597/#598b/847-3.
4. **4d** harden per-run `rPr` emission; clear 1083/1145.
5. **Delete WSO** (`style_optimization.go` synth machinery, `idCounter`,
   `commonRFonts`, related strip/merge passes) once 4a-4d are green —
   the net simplification.

## Risks & cost

- **Cost**: 4b (the normalizer) is the substantial new piece (a focused
  §17.7 style resolver, parity-only — not shipped in the product). 4a is a
  one-line flip + test churn. 4c/4d are bounded reader/writer fixes. 5 is
  deletion. Net: the *shipped* writer gets markedly simpler.
- **Risk**: 4a changes native's default output for every `.docx`
  (inline `rPr` instead of synth styles) — fully spec-valid and identical
  rendering, but unit tests asserting WSO-on bytes must be updated. The
  faithful/closeable dashboard reframe already represents "native faithful,
  Okapi restructures" honestly, so the parity story is consistent.
- **Migration**: keep WSO behind the flag until 4b proves equivalence; flip
  default; then delete. No flag-day.
