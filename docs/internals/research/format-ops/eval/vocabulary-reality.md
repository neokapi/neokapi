# Vocabulary Reality — how richly format semantics map into the shared content model

Scope: worktree `/Users/asgeirf/src/neokapi/neokapi/.claude/worktrees/format-process` (branch lab-ux-846).
Headline: **a canonical cross-format inline vocabulary already exists and is load-bearing** (registry + 4 embedded JSON packs + semantic-HTML projection + editor chips), but only **5 of ~49 native formats emit it**, block-level semantics are free-form strings, and there is **no per-format mapping artifact, no coverage test, and live Go↔TS vocabulary drift**.

---

## 1. core/model: what the Run union actually carries

### Inline runs carry typed semantics, not just opaque bytes

`core/model/run.go:127-135` — the union:

```go
type Run struct {
	Text    *TextRun        `json:"text,omitempty"`
	Ph      *PlaceholderRun `json:"ph,omitempty"`
	PcOpen  *PcOpenRun      `json:"pcOpen,omitempty"`
	PcClose *PcCloseRun     `json:"pcClose,omitempty"`
	Sub     *SubRun         `json:"sub,omitempty"`
	Plural  *PluralRun      `json:"plural,omitempty"`
	Select  *SelectRun      `json:"select,omitempty"`
}
```

Ph and Pc runs have **first-class `Type` / `SubType` fields** plus raw bytes, equivalents, display text and editing constraints — `core/model/run.go:68-97`:

```go
type PlaceholderRun struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`              // semantic kind, e.g. "fmt:bold", "media:image"
	SubType     string          `json:"subType,omitempty"` // format refinement, e.g. "html:b", "openxml:b"
	Data        string          `json:"data"`              // raw source bytes (the original tag/token)
	Equiv       string          `json:"equiv"`
	Disp        string          `json:"disp,omitempty"`
	Constraints *RunConstraints `json:"constraints,omitempty"`
}
// PcOpenRun: identical fields (run.go:79-87); PcCloseRun: ID/Type/SubType/Data/Equiv (run.go:91-97)
```

`RunConstraints` (`run.go:55-59`, Deletable/Cloneable/Reorderable) is "driven by vocabulary plus any Run-level override" — and is actually enforced: `core/model/text_edit.go:36-38, 214-222` resolves deletability from the vocabulary by semantic type (`vocabDeletable(r.Ph.Type)`) when the run carries no constraints of its own.

Writers, however, round-trip on `Data`, not `Type`: `core/model/run.go:355-368` `RenderRunsWithData` is "the canonical rendering path for format writers … HTML, XML, and markdown writers all use this helper" — Type exists for tools/editors/MT, Data carries byte fidelity. (Exception: the openxml writer IS Type-aware — see §3.)

### Block level: free-form strings, no canonical vocabulary

`core/model/block.go:10-28`: `Block.Type` is a free-form `string`; semantics ride on `Properties map[string]string`, typed `Annotations map[string]Payload`, and an (almost unused) `DisplayHint`:

- HTML maps elements → block types via a private map, `core/formats/html/reader.go:76-83`: `blockTypeMap = {"p": "paragraph", "h1".."h6": "heading", "li": "listitem", "td"/"th": "cell", "title", "caption", "quote", …}`.
- Markdown: `core/formats/markdown/reader.go:844-845` `block.Type = "heading"; block.Properties["level"] = strconv.Itoa(n.Level)`; also `"admonition-body"` (line 1102), `Properties["language"]` for code (1671).
- OpenXML does **not** surface `pStyle` (Heading1 etc.) as block semantics at all — `core/formats/openxml/styles.go` uses paragraph styles only for run-property inheritance.
- The only typed block-kind enum is JSX-specific: `core/model/run.go:30-41` `BlockContentType` (`jsx:element`, `jsx:attribute`, `js:t`).
- `core/model/displayhint.go:1-9` defines `DisplayHint{Preview, Context, MaxLength, ContentType}` ("heading", "button", "paragraph") — but **no format reader sets it**; only `bowrain/connector/figma.go:164` and the sync converters (`bowrain/core/sync/convert.go:157`, `bowrain/core/client/sync_convert.go:219`).

### Layers: structure only

`core/model/layer.go:6-20` — Layer carries Format/Locale/Encoding/MimeType + `Properties map[string]string`, `Overlays []Overlay`, `Annotations map[string]Payload`. No style semantics; nothing vocabulary-typed at layer level.

---

## 2. The canonical vocabulary EXISTS: core/model/vocabulary.go + embedded packs

This is the direct answer to "does any canonical cross-format vocabulary exist": **yes — a real registry, not just constants.**

- `core/model/vocabulary.go:69-127` — `VocabularyRegistry` with `Load(json)`, `LoadDefaults()`, `Lookup/LookupOrFallback`, `IsEntityType`, `Categories()`. Each type resolves to a `SpanTypeInfo` (`vocabulary.go:12-22`): `Category, Label, HTML{open,close,placeholder}, Display, ChipLabel, Color{bg,border,text}, Equiv, Constraints`.
- Embedded packs in `core/model/vocabularies/` (`embed.go` + 4 JSON files), loaded by `LoadDefaults()` (`vocabulary.go:110-117`):
  - `common-formatting.json` — `fmt:bold, fmt:italic, fmt:underline, fmt:code, fmt:strikethrough, fmt:subscript, fmt:superscript, link:hyperlink, media:image, struct:break, struct:footnote, struct:tab`; `entity_prefix: "entity:"`; a `{type}`-templated fallback chip.
  - `rich-html.json` — `fmt:highlight, struct:ruby` (+ dup strikethrough/sub/sup).
  - `rich-jsx.json` — `jsx:element, jsx:node, jsx:var`.
  - `code-tokens.json` — `code:function, code:markup, code:placeholder, code:variable`.
- Cross-format **rendering projection**: `core/model/run_semantic_html.go:70` `RunsSemanticHTML(runs, reg)` renders typed runs as semantic HTML "for MT APIs that handle HTML natively (DeepL, Google, Amazon)"; `ParseRunsSemanticHTML` (line 126) reconstructs runs from translated HTML, copying source-run metadata back. Consumed by `core/mt/tools/translate.go:101,108`.
- Entity-typed Ph runs: `core/model/run_keys.go:63-135` (`RunsGeneralizedText`, `RunsEntityValues`) and `core/redaction/redaction.go:101` key off the `entity:` prefix.
- **TS mirrors** (declared mirrors, but hand-copied): `packages/kapi-format/src/vocabulary.ts` ("Mirrors neokapi's core/model/vocabulary.go", `JSX_VOCABULARY`); Go-side mirror back at `core/klf/preview.go:9-55` (`JSXVocabulary`, "Kept in sync by a round-trip test"); KLF files can even declare vocab deps — `core/klf/schema.go:117-129` `Vocabulary` field; copied JSON packs at `packages/ui/src/vocabularies/` and `bowrain/packages/ui/src/vocabularies/`.
- Docs: `web/docs/framework/vocabularies.md` (concept: "`<b>` (HTML), `**` (Markdown), and `<w:b/>` (DOCX)" → one type) and `web/docs/contribute/vocabularies.md` (authoring the JSON format).

**Found drift (load-bearing for any V-axis):** `core/model/vocabularies/common-formatting.json` contains `fmt:strikethrough`/`fmt:superscript`/`fmt:subscript` entries that BOTH TS copies lack (`diff` confirms; the TS side happens to recover them from `rich-html.json`), and the TS `index.ts` loads only 3 packs — `rich-jsx.json` is not shipped to the frontend registries. Three hand-maintained copies, **no drift gate** (contrast: `make check-contract-types` exists for IO-contract types).

---

## 3. How 4 representative readers populate runs

### HTML — typed + raw, vocabulary-driven (the best case)

- Mapping table `core/formats/html/reader.go:92-105`:
  ```go
  var htmlSemanticTypes = map[string]string{
  	"b": "fmt:bold", "strong": "fmt:bold",
  	"i": "fmt:italic", "em": "fmt:italic", ...
  	"a": "link:hyperlink", "img": "media:image", "button": "ui:button",
  }
  ```
- Reader owns a registry (`reader.go:111,123-124` `vocab.LoadDefaults()`). Run construction `core/formats/html/tokenreader.go:816-852`: `b.AddPcOpen(spanID, semType, "html:"+childTag, string(rewriteInlineTagWithRefs(tokenRaw,…)), info.Display.Open, info.Equiv, constraints-from-vocab)` — **Type = canonical, SubType = `html:b`, Data = exact raw tag bytes**, display/equiv/constraints pulled from the vocabulary. Untranslatable inline markup degrades to `"code:markup"` Ph (tokenreader.go:806-813).

### Markdown — typed + raw

`core/formats/markdown/reader.go:2460-2478` `buildEmphasisRuns`: level 2 → `semType="fmt:bold", subType="md:strong", data="**"`; level 1 → `fmt:italic`/`md:emphasis`. Links `reader.go:2587-2629`: `link:hyperlink` with subtypes `md:link`, `md:link-ref`, `md:link-title`, Data holding the literal `[`, `](dest)` pieces. All via `r.vocab.LookupOrFallback(semType)`.

### OpenXML — the richest typed mapping in the tree

- A whole constants file `core/formats/openxml/vocabulary.go`: canonical types (`TypeBold = "fmt:bold"`, `TypeItalic`, `TypeUnderline`, `TypeStrikethrough`, `TypeSuper/Subscript`, `TypeHyperlink = "link:hyperlink"`, `TypeImage = "media:image"`, plus openxml-specific `struct:field`, `struct:sdt`, `struct:revision-ins`, `struct:commentRange`, …) and ~30 `SubType*` refinements (`openxml:b`, `openxml:fldChar`, `openxml:br:standalone`, …) each with ECMA-376 + upstream-Okapi citations.
- `rPr` → runs: `core/formats/openxml/runprops.go:1338-1366` `appendOpeningRuns` — `if rp.bold { emit(TypeBold, SubTypeBold, boldOnXML(rp)) }` etc.; bold/italic/underline/strike/vertAlign become **paired codes typed `fmt:*` with the raw `<w:b/>`/`<w:u w:val="...">` XML in Data**, closing runs in reverse order (1370-1392).
- The openxml **writer is Type-aware** (unique among writers): `core/formats/openxml/writer.go:4203-4273` branches on `TypeBold`/`TypeItalic`; `writer.go:2643,2678` branch on `TypeHyperlink|TypeSmartTag|TypeRevisionIns|TypeSDT`. Semantics here are bidirectional, not just annotation.

### XLIFF 1.2 — explicit ctype↔vocabulary mapping table

`core/formats/xliff/reader.go:1713-1732`:

```go
func ctypeToSpanType(ctype string) string {
	switch ctype {
	case "bold", "x-bold":       return "fmt:bold"
	case "italic", "x-italic":   return "fmt:italic"
	case "underlined", ...:      return "fmt:underline"
	case "link", "x-link":       return "link:hyperlink"
	case "lb", "x-lb":           return "struct:break"
	case "image", "x-image":     return "media:image"
	case "":                     return ""
	default:                     return "xliff:" + ctype   // namespaced passthrough
	}
}
```
Applied to every inline element — bpt/ept/ph/x/bx/ex/g/it (`reader.go:1449-1532`, and the native IR path `native_parse.go:218-276`), with inner bytes captured in Data and `equiv-text` in Equiv.

### Counter-example: XLIFF 2 deliberately does NOT canonicalize

`core/formats/xliff2/reader.go:488-548` `inlinesToRuns` copies the **raw XLIFF 2 attributes** into runs: `Type: in.Pc.Type` is the spec enum `"fmt"|"ui"|"quote"|"link"|"image"|"other"` and `SubType` is e.g. `"xlf:b"` (`core/formats/xliff2/inline.go:135-136`) — never mapped to `fmt:bold`. Comment: "Lossy by design — Run is a simpler abstraction; the lossless path is the SourceBodyAnnotation/TargetBodyAnnotation IR." So a `<pc type="fmt" subType="xlf:b">` and an HTML `<b>` land as *different* Type strings in the same model.

### The long tail (coverage survey)

Grep survey over `core/formats/*/` (reader code, tests excluded): **canonical vocab emitters = html, markdown, openxml, xliff, jsx** (jsx emits `jsx:element/var/node`, see `core/formats/jsx/jsx.go:79` and `spec.yaml:133`). Roughly 23 other formats emit Ph/Pc runs but with generic or format-local types:

- `core/formats/json/reader.go:544-548`, `po/reader.go:1146`, `properties/reader.go:882`: placeholders get literal `Type: "code"` — not even the vocabulary's `code:placeholder`.
- `core/formats/odf/reader.go:465,711`: Okapi-style `"x-"+name.Local` types.
- tmx, ts, xcstrings, androidxml, resx, yaml, csv, dtd, tex, …: opaque or absent types; semantics live only in `Data` bytes.
- A remediation tool exists precisely because of this gap: `core/tools/span_classify.go:14-50` reclassifies `code:markup` runs by sniffing `Data`/`SubType`, with mapping tables `htmlToSemanticType` and `okapiSubTypeMap` (`"okapi:bold" → "fmt:bold"`, …) "designed to run after bridge readers (Okapi, XLIFF) that produce generic code:markup spans".

---

## 4. Cross-format vocabulary: what exists vs. what's scattered

Exists today:
- The registry + JSON packs (§2) — single canonical namespace `category:name` (`fmt:`, `link:`, `media:`, `struct:`, `code:`, `jsx:`, `entity:`, `ui:`).
- A model→HTML→model projection usable by any consumer (`run_semantic_html.go`).
- Per-format mapping tables: `html/reader.go:92-105`, markdown's inline switch, `openxml/vocabulary.go`, `xliff/reader.go:1713`, `tools/span_classify.go:24-50`.

Missing:
- The mapping tables are **5 disconnected Go literals** — no shared data artifact, no completeness check ("does this format map every vocab type it can express?"), no reverse (write-side) table except openxml's hand-rolled writer switches and the HTML-tag heuristic in `buildHTMLToTypeMap` (`run_semantic_html.go:28-51`, which reverse-engineers tags out of the vocab's HTML strings with a regex).
- `core/model/vocabulary_test.go` covers registry mechanics only; **no test anywhere asserts cross-format equivalence** (e.g. `<b>x</b>`, `**x**`, `<w:b/>` produce the same `Type`).
- No Go↔TS single-sourcing (real drift found, §2).

---

## 5. What the editors render

Three distinct tiers exist:

1. **Vocabulary-driven chip editor — `@neokapi/ui-primitives` (packages/ui), used by bowrain.**
   `packages/ui/src/components/editor/`: Lexical-based `InlineCodeEditor.tsx` with `TagChipNode.tsx` / `TagChipComponent.tsx`; `tagSemantics.ts` resolves everything through the vocabulary — `semanticLabel()` returns the chip label ("B>", "/I"), `tagColors()` the per-type ColorScheme, `semanticCategory()`, constraint locks (`tagConstraints.ts`), plus `TagPalette.tsx` and `InlineCodeLegend.tsx`. `InlinePreview.tsx` renders a **real WYSIWYG strip** ("bold appears bold") via whitelist `codedTextToHtml`.
2. **Bowrain web/desktop editor — actually WYSIWYG.**
   `bowrain/packages/ui/src/components/TranslationEditor.tsx` → `UnifiedTargetEditor.tsx` (wraps `InlineCodeEditor`, re-exported as `TargetCellEditor` in `bowrain/packages/ui/src/index.ts:256`); `FormattedSourceDisplay.tsx` maps each span's vocabulary `html.open` to CSS (`htmlTagStyle: {"<b>": {fontWeight:700}, "<i>": {fontStyle:"italic"}, …}` lines 18-29) — "bold appears bold, links underlined" with category-tinted backgrounds; plus `VocabularyExplorer.tsx`, `FormatVocabularyBadge.tsx`, `VisualEditorToolbar.tsx`. Mounted from `bowrain/apps/web/src/routes/workspace/translate.tsx` and `bowrain/apps/bowrain/frontend/src/components/DesktopTranslateView.tsx`. Caveat: this editor still authors coded-text and bridges through `bowrain/apps/bowrain/frontend/src/api/codedToRuns.ts` (tracked #695).
3. **Kapi desktop + kapi-lab — opaque kind-colored chips, vocabulary unused.**
   `apps/kapi-desktop/frontend/src/components/FilePreview.tsx:10` uses `DocumentViewer` → `BlockInspector` → `RunSequence` (`packages/ui/src/components/preview/RunSequence.tsx`): chips colored by **run kind only** (amber=ph, blue=pcOpen/pcClose, violet=plural), label = `equiv || data || #id`, `title` shows the raw type string. No bold-as-bold, no vocab chip labels/colors. `packages/kapi-lab/src/` (BlockResults, PartInspector, AnatomyExplorer) likewise has zero vocabulary imports.

---

## 6. Overlays: could a "style"/"vocabulary" overlay attach today?

Yes — with zero model changes. `core/model/overlay.go:18-33`:

```go
type OverlayType string
const (
	OverlaySegmentation OverlayType = "segmentation"
	OverlayTerm         OverlayType = "term"
	OverlayEntity       OverlayType = "entity"
	OverlayQA           OverlayType = "qa"
	OverlayAlignment    OverlayType = "alignment"
	OverlayTermCandidate OverlayType = "term-candidate"
)
```
…and the doc comment is explicit: "formats and plugins may use any string for their own run-anchored state." A Span (`overlay.go:63-68`) carries `Range RunRange` (run-index + rune-offset anchored, survives edits via `overlay_remap.go`), free `Props map[string]string`, and a typed `Value Payload` rehydrated through the open payload registry (`core/model/annotation_registry.go:27-40`, `RegisterPayload(typeName, factory)`). Per-variant overlays (`Variant *VariantKey`) mean a style overlay could annotate source and each target independently. The plugin wire already round-trips arbitrary overlay types (`core/plugin/protoconvert/protoconvert.go:114-138`).

So a stand-off `style` overlay (e.g. for formats whose styling can't be losslessly paired-coded, like ODF style spans, or for editor-authored emphasis) is architecturally trivial today — what's missing is a registered payload type and producers/consumers.

---

## 7. Conclusion: a Vocabulary maturity axis (V0–V3) measurable from today's artifacts

The existing format-maturity rubric (`docs/internals/format-maturity.md`, L0–L4, "9 dimensions") has **no vocabulary dimension** — the word appears only once, about spec assertions. A V-axis is independently measurable right now:

- **V0 — Opaque codes.** Inline markup survives as Ph/Pc runs with raw `Data` only; `Type` empty/format-local/generic (`"code"`, `"x-span"`). *Detectable:* reader constructs `PlaceholderRun`/`PcOpenRun` without canonical-namespace Type. Today: json, po, properties, yaml, odf, tmx, ts, xliff2, and ~15 more.
- **V1 — Typed inline kinds.** Reader maps its core inline semantics to canonical types (`fmt:*`, `link:*`, `media:*`, `struct:*`, `code:*`) with a format `SubType` and raw `Data` retained. *Detectable:* canonical types in reader code / `spec.yaml` run expectations. Today: html, markdown, openxml, xliff (1.2), jsx — 5/49.
- **V2 — Actionable semantics.** Type information is consumed, not just carried: type-aware writer paths (only openxml today, `writer.go:4203+`), semantic-HTML MT round-trip compatibility (`RunsSemanticHTML`/`ParseRunsSemanticHTML`), vocab-driven editing constraints (`text_edit.go`), **plus block-level semantics** (canonical Block.Type, heading level, DisplayHint.ContentType populated). Today: openxml inline-only; block-level nobody (html/markdown set free-form strings; openxml drops pStyle).
- **V3 — Verified vocabulary contract.** Per-format mapping table exists as a data artifact; a vocab parity test proves the same semantic content in two formats yields identical Types; Go↔TS packs single-sourced with a drift gate; editor chip/WYSIWYG rendering exercised in Storybook for the format's types. Today: nobody.

### Missing engineering artifacts (ranked)

1. **Per-format mapping table as data** — replace the 5 scattered Go map literals (+ span_classify's retro-tables) with a declarative per-format `vocab-map` (native element ⇄ canonical type ⇄ subtype), usable by readers, writers, the bridge, and a coverage report.
2. **Vocab coverage & parity tests** — none exist; the obvious shape is a cross-format fixture ("bold/italic/link/image sentence") asserted to produce identical `Type` sequences in html/markdown/openxml/xliff, and a per-format completeness check against the mapping table. `core/formats/maturity_test.go` has no vocab guardrail.
3. **Go↔TS vocabulary single-sourcing + drift gate** — actual drift today (Go `common-formatting.json` ⊃ TS copies; `rich-jsx.json` absent from both TS registries); mirror the `make check-contract-types` pattern.
4. **Block-level vocabulary** — a canonical block-kind registry (paragraph/heading/listitem/cell/title/quote + level property), adoption of `DisplayHint.ContentType` by readers (currently set only by the figma connector), and an openxml pStyle→block-kind mapping.
5. **XLIFF 2 canonicalization** — map `type="fmt"`+`subType="xlf:b"` ⇄ `fmt:bold` in `inlinesToRuns` (and back on write), so the interchange format speaks the same vocabulary as the native readers.
6. **Generic-type cleanup in the long tail** — json/po/properties emit literal `"code"` instead of the vocabulary's `code:placeholder`/`code:variable`; ODF's `x-*`; the bridge's `okapi:*` (currently laundered post-hoc by span-classify).
7. **Editor parity for kapi surfaces** — make `RunSequence`/BlockInspector (kapi-desktop, kapi-lab) vocabulary-aware (chip labels/colors/categories already exported from `packages/ui` `tagSemantics.ts`); today only bowrain renders bold-as-bold.
8. **Rubric integration** — add the V-axis to `docs/internals/format-maturity.md` + the `/format-triage` scorer so vocabulary coverage is tracked like round-trip fidelity.
