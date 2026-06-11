# XLIFF 2.x Reader/Writer — Design Spec

**Status**: design doc, pre-implementation. Companion to `core/formats/xliff2/spec.yaml` (which currently mirrors okapi parameter parity and will be repointed at this design once implementation lands).

**References**:
- [XLIFF 2.2 Part 1: Core](https://docs.oasis-open.org/xliff/xliff-core/v2.2/xliff-core-v2.2-part1.html) (OASIS standard, 2024)
- [XLIFF 2.2 Part 2: Extended modules](https://docs.oasis-open.org/xliff/xliff-core/v2.2/xliff-extended-v2.2-part2.html)
- [XLIFF 2.0 OS](http://docs.oasis-open.org/xliff/xliff-core/v2.0/os/xliff-core-v2.0-os.html), [XLIFF 2.1 OS](http://docs.oasis-open.org/xliff/xliff-core/v2.1/os/xliff-core-v2.1-os.html)

---

## 1. Vision

XLIFF 2.x sits at neokapi's **system boundary**: it's the format we exchange with translation tools, TMS systems, MT vendors, and human translators. It is not an internal IR. Therefore the contract is asymmetric:

| Direction | Contract | Status |
|---|---|---|
| **Read** | Lossless to neokapi's content model — every spec attribute is either decoded into a typed model field or preserved as opaque metadata for re-emission. | ✅ |
| **Write (generation/canonical)** | Spec-conformant, well-indented output with canonical attribute ordering (id first, then alphabetical groups). XLIFF-aware indenter respects mixed-content semantics. Used when no source DOM is available (e.g. HTML→XLIFF 2). | ✅ |
| **Idempotent** | `read → write → read → write` produces byte-equal output on the second iteration regardless of the first. Exercised by `TestRoundTrip_AllFixtures` against 40 okapi-testdata fixtures. | ✅ 40/40 |
| **Write (untouched round-trip)** | Read an existing XLIFF 2 file and write it back without modifying any segment → byte-equal output. The writer captures the source bytes and emits them verbatim when no patching was needed, bypassing etree's serialization quirks. Exercised by `TestRoundTrip_ByteEqualUntouched`. | ✅ 37/37 (3 fixtures excluded for documented reader normalizations: XML 1.1→1.0 coercion, CR-entity→LF normalization) |
| **Write (modified round-trip, minimal diff)** | When some segments were modified, patch only those `<source>`/`<target>` elements in the source DOM; everything else (comments, module data, attribute order on untouched units, custom namespaces) survives verbatim through etree. Stale-IR detection is automatic — when `Segment.Runs` text disagrees with `SegmentInlineAnnotation` text, the writer falls back to `Runs`. No caller contract required. | ✅ |

We deliberately **do not** inherit the Okapi XLIFF 2 toolkit's documented losses (skeleton dropped, comments dropped, formatting dropped, attributes reordered/added/removed). Okapi's losses come from collapsing XLIFF 2 down to its Java `TextFragment` IR; neokapi's content model is richer and we own the full pipeline, so we can do better.

## 2. Library choice

We move off `encoding/xml` for XLIFF 2 and onto **`github.com/beevik/etree`**:

- Pure Go, no cgo, fits the single-binary distribution principle (AD-001).
- DOM-style API mirrors XLIFF 2's nested structure and the Okapi Java reference toolkit.
- Preserves attribute order, comments, processing instructions, and the original namespace prefix bindings.
- Handles `xmlns:*` correctly (the bug that motivated this refactor).
- Captures all whitespace (significant and insignificant) in `Text`/`Tail` fields on load.
- **Round-trip is byte-equal when we don't call `Indent()`** — verified with mixed content, `xml:space="preserve"`, multi-line text, trailing spaces, and inline codes (`<sc/>Hello<ec/>`).
- Patching one element's text leaves all surrounding whitespace untouched (minimal-diff).

The one trap: `Document.Indent(n)` has a known etree limitation ([etree#88](https://github.com/beevik/etree/issues/88)) — it honors `xml:space="preserve"` for **leaf** text-only elements (when `IndentSettings.PreserveLeafWhitespace=true`) but **not** for mixed content (text + child elements). A `<source xml:space="preserve"><sc/>Hello<ec/></source>` will get newlines injected between siblings, violating XML 1.0 §2.10. We avoid this by:

- **Round-trip mode**: never calling `Indent()`. etree captured the source whitespace into `Text`/`Tail` on load; passthrough write reproduces it byte-for-byte.
- **Generation mode**: building the DOM with explicit `Text`/`Tail` values during construction rather than calling `Indent()` at the end. We control whitespace per-element, mixed-content elements get empty `Text`/`Tail` between their children, and the etree limitation never engages.

**Out of scope**: XSD validation. We rely on structural checks during read; XSD validation can be added later via libxml2 cgo if a use case appears.

## 3. Document model mapping

```
XLIFF 2.x element        →  neokapi model
─────────────────────────────────────────────
<xliff>                  →  Layer (root) — version, srcLang, trgLang, namespaces
<file>                   →  Layer (file) — id, original, srcDir/trgDir, translate, canResegment
<skeleton>               →  Layer.Properties[xliff2:skeleton-href] (when external)
                            OR Skeleton subtree preserved opaquely (when inline)
<group>                  →  GroupStart / GroupEnd Parts
<unit>                   →  Block — id, name, translate, canResegment, srcDir/trgDir, type
<segment>                →  Block.Source[i] / Block.Targets[trgLang][i]
<ignorable>              →  Block.Source[i] with IgnorableSegment marker
<source>                 →  Segment.Runs (decoded inline content)
<target>                 →  Segment.Runs in Block.Targets[trgLang]
<notes><note>            →  Block.Properties (unit-level) or Layer.Properties (file-level)
<originalData><data>     →  Unit-level OriginalDataAnnotation (id → bytes, preserved)
<cp hex="X"/>            →  Run.Text contains the actual code point (we resolve)
<ph>                     →  Run.Code{kind: Placeholder, attrs}
<pc>                     →  Run.Code{kind: PairedOpen}, … nested runs … , Run.Code{kind: PairedClose}
<sc>/<ec>                →  Run.Code{kind: SpanStart}, …, Run.Code{kind: SpanEnd}
<mrk>                    →  Run.Annotation{kind: Marker, attrs} wrapping nested runs
<sm>/<em>                →  Run.Annotation{kind: SpanMarkerStart/End}
```

### Inline content (the hard part)

`<source>` and `<target>` allow nested mixed content with up to 8 inline element types in any order, themselves nestable inside `<pc>` and `<mrk>`. We model this as a **typed Run sequence** (already partially present in `core/model/segment.go` for xliff 1.x — extend rather than introduce a parallel type).

Each inline element preserves all its spec attributes:

```go
type CodeAttrs struct {
    ID            string
    DataRef       string  // <ph>/<sc>/<ec>
    DataRefStart  string  // <pc> only
    DataRefEnd    string  // <pc> only
    CanCopy       string  // "yes"|"no"  (omit when default)
    CanDelete     string
    CanReorder    string  // "yes"|"firstNo"|"no"
    CanOverlap    string
    CopyOf        string
    Dir           string  // "ltr"|"rtl"|"auto"
    Disp          string
    DispStart     string
    DispEnd       string
    Equiv         string
    EquivStart    string
    EquivEnd      string
    SubFlows      string
    SubFlowsStart string
    SubFlowsEnd   string
    SubType       string  // "xlf:b" etc.
    Type          string  // "fmt"|"ui"|...
    Isolated      string  // <sc>/<ec> only
    StartRef      string  // <ec>/<em>
}
```

`<originalData>` is preserved at the unit (Block) level as an `OriginalDataAnnotation` — a map of `id → bytes` — and the writer re-emits it iff at least one inline code on the unit references it via `dataRef`/`dataRefStart`/`dataRefEnd`.

### Module data

Module elements (mda, mtc, gls, fs, res, slr, val, its, pgs) appearing on `<file>`, `<group>`, `<unit>`, or inside `<source>`/`<target>` are **preserved opaquely**: the reader stashes the etree subtree on the parent's `Annotations` under a typed annotation, and the writer re-attaches it on emit.

We do not interpret module semantics in the v1 implementation — but because we preserve the subtree, no data is lost across round-trip.

Specific exceptions where we DO interpret modules in v1:
- `mda:metadata` on `<file>` → expose select keys as Layer.Properties (e.g. tool name/version) for downstream display.
- `mtc:matches` on `<unit>` → exposed as `MatchesAnnotation` so QA tools can leverage existing translation candidates without re-deserializing.

## 4. Reader contract

### Parse mode

DOM-based with etree. We do not stream tokens; an XLIFF 2 file is bounded (typically <50 MB) and the DOM cost is acceptable. If profiling later shows a need, etree supports an iterative API.

### Output Parts

```
PartLayerStart  (xliff root → Layer with version, srcLang, namespaces)
  PartLayerStart  (file → Layer{ID: "file-X"} with file attrs)
    PartGroupStart (when file has <group>)
      PartBlock × N
    PartGroupEnd
    PartBlock × M  (units directly under file)
  PartLayerEnd
PartLayerEnd
```

Note: we use **nested Layers** (xliff root + file) rather than just one Layer-per-file, because XLIFF 2 allows `<notes>` and module data on `<xliff>` itself that needs a home.

### Preservation guarantees

| Source feature | How preserved |
|---|---|
| XML comments | etree DOM nodes attached to nearest enclosing Block/Layer; re-emitted in original position. |
| Processing instructions | Same as comments. |
| Custom namespace prefixes | etree preserves the prefix → URI binding from the source. |
| Attribute order on root/file/group/unit | etree preserves element-attribute order. |
| Unknown extension elements | Preserved as opaque annotation subtrees. |
| `<skeleton>` (inline) | Preserved as Layer.Annotation; opaque, not parsed. |
| `<skeleton href="…">` | Layer.Properties["xliff2:skeleton-href"]. |
| `xml:space="preserve"` | Honored — segment whitespace not normalized. |
| `xml:lang` on source/target | Validated against srcLang/trgLang per spec; kept as Segment metadata. |

## 5. Writer contract

### Modes

1. **Round-trip mode**: writer received Parts from an XLIFF 2 reader. The reader stashed both the parsed etree DOM AND the original input bytes on the file Layer via `SourceDOMAnnotation`. The writer:
   - Walks the DOM. For each `<unit>` and `<segment>`, compares the model's content against the DOM's content (deep `Inline`-IR equality when `SegmentInlineAnnotation` is present, fallback to text comparison).
   - When a segment's content matches → leaves the DOM verbatim.
   - When it differs → replaces just that `<source>` or `<target>` element's children with re-rendered IR.
   - When NO segment was patched (and no explicit file notes were stamped via `SetFileNotes`) → emits the original input bytes verbatim, bypassing etree's serializer entirely. This is the byte-equal short-circuit.
   - When patching occurred → serializes the mutated DOM via etree. The patched bodies use canonical formatting; everything outside them keeps the source's original bytes (modulo etree's serialization conventions).

2. **Generation mode**: writer received Parts from a non-XLIFF-2 source (e.g. HTML reader → XLIFF 2 writer). The writer builds a fresh etree DOM from canonical templates and serializes. Produces clean, well-indented, namespace-minimal XLIFF 2.2 by default.

The mode is selected automatically by whether `Layer.Annotations["xliff2:source-dom"]` is present.

### Idempotency

`writer(reader(writer(reader(X)))) = writer(reader(X))` for every well-formed XLIFF 2 input X. This is the contract `TestRoundTrip_AllFixtures` checks.

### Defaults (generation mode)

- XML declaration: `<?xml version="1.0" encoding="UTF-8"?>`
- Indentation: 2 spaces
- XLIFF version: latest supported (currently 2.2) unless overridden
- Namespace declarations: only those actually referenced
- Attribute ordering on each element: spec-defined order (id first, then alphabetical for the rest)

## 6. Conformance and validation

The reader rejects documents that:
- Are not well-formed XML.
- Use `<xliff>` without a recognized 2.x namespace and without `version="2.0"|"2.1"|"2.2"`.
- Have `<target>` elements but no `trgLang` on `<xliff>` (per spec §3.3).
- Have `<unit>` with zero `<segment>` children (per spec §3.2.2).

The reader does NOT validate:
- ID uniqueness scopes — preserved as-is.
- Inline code pairing rules (sc/ec matching, isolated semantics) — preserved as-is.
- Module-specific constraints — out of scope for v1.

## 7. Testing strategy

Three tiers:

1. **Spec conformance** — every element/attribute documented in this spec has at least one targeted unit test (`reader_test.go`, `writer_test.go`).
2. **Self round-trip** — `TestRoundTrip_AllFixtures` runs every fixture in `okapi-testdata/1.48.0-v4/{okapi,integration-tests}/.../xliff2/` through `read → write → read → write` and asserts pass1 == pass2.
3. **Fixture diff bound** — for the round-trip case (input is well-formed XLIFF 2), pass1 must differ from input only in normalized formatting (whitespace) and namespace prefix order. Tracked as a separate optional test.

## 8. Out of scope (deferred to v2 of this spec)

- XSD validation against the OASIS schema.
- Active interpretation of mtc:matches (read/write to TM).
- Active interpretation of slr:* (size restriction enforcement).
- Active interpretation of val:validation (rule execution).
- Active interpretation of its:* (locQualityIssue surfacing).
- Bidirectional text directionality computation (we preserve `dir`/`srcDir`/`trgDir` but don't apply them).

## 9. Migration plan

1. Add etree dependency to framework `go.mod`.
2. Replace reader internals with etree-based parser; keep public API (`NewReader`, `Open`, `Read`).
3. Replace writer internals with etree-based serializer + DOM-patch path; keep public API.
4. Extend `core/model` with full `CodeAttrs` (or place it in `core/formats/xliff2/inline.go` as a typed annotation).
5. Run `TestRoundTrip_AllFixtures` to drive bug fixes.
6. Update `spec.yaml` to point at this design doc.
7. Document the contract in `web/docs/developer/formats.md`.

## 10. Decisions

- **Code attrs**: **typed Go fields** (`CodeAttrs` struct as defined in §3). Full DX for downstream tools (QA, MT, segmentation) at the cost of a deeper change.
- **`<cp hex="X"/>` on read**: **resolve to the actual code point** in `Run.Text`. Loses the `<cp>` marker but produces a clean text stream for tools. The writer re-encodes invalid XML characters as `<cp>` on output per spec §3.7.2 processing requirement.
- **Skeleton**: **treated as a separate flow** — out of scope for the v1 reader/writer. External `<skeleton href>` is captured in `Layer.Properties["xliff2:skeleton-href"]` for future consumers; inline `<skeleton>` is preserved opaquely. The kapi merge step will gain a dedicated skeleton-aware path later.
- **Patching strategy**: round-trip mode walks the etree subtree and surgically replaces TextRun content within `<target>`; generation mode re-serializes the full `<target>` from the model. (Refined during implementation as needed.)
