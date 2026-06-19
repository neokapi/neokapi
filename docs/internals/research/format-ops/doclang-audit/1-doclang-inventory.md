# DocLang Capability Inventory (Task 1 — completeness yardstick)

Source: `/Users/asgeirf/src/doclang-project/doclang/spec.md` (v0.6, 3734 lines, read in full).
Purpose: exhaustive enumeration of everything DocLang can express, so the gap analysis (vs neokapi `core/model/structure.go`, 276 lines) and the **Structure & Geometry G0–G4** ladder can be measured against a complete list. Line refs are `spec.md` line numbers. "FUTURE" = Appendix C (planned, non-normative for v0.6 body but published in the spec).

DocLang is XML (UTF-8 default; XML prolog may override; entities or CDATA for reserved chars; §Content Encoding L134–140). Namespace `https://www.doclang.ai/ns/v0`, root `<doclang version="0.6">` (L1965–1994). Design intent: a *controlled, token-efficient* vocabulary for LLM/VLM generation (≤~1000 syntax tokens, L62–73), AI-native (semantics + geometry), supporting lossless round-trip of **content** (L40–49). Differentiator vs OCR formats (PageXML/ALTO/hOCR): adds native semantic roles + rich table structure, not just bbox (L88–93).

## 0. Cross-cutting structural model (referenced by all groups below)

- **Property Semantics** (L118–132): properties expressed as *leading child elements* of an element's body (LLM-token-friendly) rather than XML attributes, except for strictly-bounded enums which stay attributes. E.g. `<size>250</size>` style. Drives the "element head" design.
- **Element head / element body** (§Head and Body Areas L142–199). Every *semantic element* may begin with an **element head** — a fixed-order property sequence: `<label>` → `<thread>` → (`<xref>` | `<href>`, mutually exclusive) → `<layer>` → **exactly 4 `<location>`** (x_min,y_min,x_max,y_max) → `<caption>` → `<custom>` (all optional) (L149–156). Remaining content = **element body** (payload).
- **Document head / document body** (L162–168): `<doclang>` may start with optional `<head>` (global metadata); the rest is the document body.
- **Subclasses** (§Subclasses L201–217): two extensibility levers — `<label value="...">` (free, **not validated**, conveys fine subclass) and a bounded `class` attribute on some elements (validated enum, carries structural semantics, e.g. `picture class="chart"` enables `<tabular>` chart data).
- **Version management** (§L219–245): `version` = `MAJOR.MINOR`; SemVer forward-compat within a major; `0.x` every minor is breaking. XSD may carry patch (L243).
- **Conformance**: machine-checkable via the DocLang reference validator / XSD (L255). Whitespace: app-decided default (`xml:space="default"`); `<content>` forces preserve (L134–140, L2653–2671).
- **Two non-content sinks of content**: virtual `<text>` (unwrapped text acting as if wrapped in `<text>`) inside list items and table cells (L478–481, L2048, L2170, L2192).

---

## (a) Document / page / section structure

| Construct | Element(s) | Spec |
|---|---|---|
| Document root | `<doclang>` (attrs `xmlns`, `version`) | L1965–1994 |
| Global metadata container | `<head>` (first child of doclang only) | L1996–2014 |
| Pagination / page boundary | `<page_break/>` (empty; child of doclang only; each page = valid standalone DocLang body) | L2016–2040; §L1868–1916 |
| Repeated page furniture | `<page_header>`, `<page_footer>` | L2108–2146 |
| Hierarchical section headings | `<heading level="N">` (positive int, default 1; level-1..6 tokenized) | L2066–2087; tokens L3104–3110 |
| Generic grouping container | `<group>` (encapsulates multiple semantic elements; **no raw text, no location at group level**) | L2312–2330; L275 |
| Form/field scoping container | `<field_region>` | L2148–2166 |
| Default coordinate grid | `<default_resolution width height>` (head; default 512×512) | L3025–3042 |
| Per-page physical size | `<page_size width height [page_no]>` (head, **FUTURE**; no page_no = default, page_no from 1) | L3222, L3253–3254 |

Notes: there is **no dedicated `<section>`/`<chapter>`/`<article>` element** — sectioning is *implied* by `<heading>` level hierarchy + reading order + optional `<group>`. Pages are delimited by `<page_break/>`, not wrapped in a page element. Document order = logical order.

## (b) Logical roles (every role / element type)

**Semantic elements** (§Semantic Elements L2042–2454). *Primary* = may appear at doclang top level; *secondary* = only nested:
- `<text>` — cohesive paragraph text; also "virtual text" form in lists/cells (L2046–2064).
- `<heading level>` — heading, depth via `level` (L2066–2087).
- `<footnote>` (L2088–2106).
- `<page_header>`, `<page_footer>` (L2108–2146).
- `<field_region>` — form container (L2148–2166); raw text not allowed.
- `<list class="unordered|ordered">` — list; body must begin with `<ldiv>` (L2168–2188).
- `<table>` — OTSL table; body must begin with a cell-structural element (L2190–2208).
- `<index>` — TOC / glossary, OTSL-based (same cell model as table) (L2210–2228).
- `<formula>` — raw LaTeX, no delimiters (L2230–2248).
- `<code>` — code block or inline; language via `<label>` (L2250–2268).
- `<picture class="undefined|chart">` — image; body starts with optional `<src>` then optional `<tabular>` (chart only) then any semantic content (L2270–2290).
- `<marker>` — visible list/field glyph or number; can have own element head (L2292–2310).
- `<group>` — container (L2312–2330).
- `<field_heading level>` — heading scoped inside field_region (L2332–2352).
- `<field_item>` — scopes 0–1 `<key>` + 0–N `<value>` (L2354–2372).
- `<key>` — field key (descendant of field_item) (L2374–2392).
- `<value class="read_only|fillable">` — field value (L2394–2414).
- `<hint>` — guidance for a (fillable) field (L2416–2434).
- `<caption>` — associated caption; **part of element head**, not body (L2436–2454).

**Table / index / tabular cell roles** (OTSL structural elements §L2837–3019; rectangular-grid rule L702):
- `<fcel/>` full/regular cell (L2841); `<ecel/>` empty cell (L2857).
- `<ched/>` column header (L2873); `<rhed/>` row header (L2889); `<corn/>` corner cell / top-left header intersection (L2905); `<srow/>` section row header (L2921).
- **Spanning**: `<lcel/>` left-merge = colspan continuation (L2937); `<ucel/>` up-merge = rowspan continuation (L2953); `<xcel/>` combined cross (both axes) span (L2969).
- `<nl/>` end-of-row delimiter (L2985).
- Cells may hold any semantic element sequence incl. nested `<table>`, `<list>`, `<picture>` or virtual text (L645–698, L703).

**List item role**: `<ldiv/>` list-item start, optionally containing only `<marker>` (L3001–3019).

**Markers / selection**: `<marker>` (glyph/number, L2292); `<checkbox class="selected|unselected"/>` (form/selection state, L2635–2651).

**Picture subclasses** (recommended `label@value`, not validated; Appendix B L3050–3063):
- chart subclass: `bar_chart, box_plot, flow_chart, line_chart, pie_chart, scatter_plot`.
- general (`class="undefined"`): `full_page_image, page_thumbnail, photograph, chemistry_structure, bar_code, icon, logo, qr_code, signature, stamp, engineering_drawing, screenshot_from_computer, screenshot_from_manual, geographical_map, topographical_map, calendar, crossword_puzzle, music`; plus `other`, `undefined`.

**Code subclass**: `label@value` = GitHub Linguist v9.5.0 language keys (e.g. `Python`); `other`, `undefined` (L3065–3075).

## (c) Inline / run-level constructs

**Formatting elements** (§L2673–2835; inline, nestable, allowed in any raw-text context; **no attributes**):
- `<bold>`, `<italic>`, `<underline>`, `<strikethrough>` (L2677–2755).
- `<superscript>`, `<subscript>` (L2757–2795).
- `<handwriting>` — handwritten text/annotation (L2797–2815).
- `<rtl>` — right-to-left direction marker (L2817–2835). (Only RTL; LTR is the implicit default — no `<ltr>`.)

Other inline constructs:
- Inline `<code>` and inline `<formula>` (same tags as block; tag conveys context) (L343–350, L407–417).
- `<checkbox/>` inline empty element (L2635).
- `<content>` — whitespace-preserving inline text container (`xml:space="preserve"`); also for code/whitespace-sensitive runs (L2653–2671); CDATA alternative for special chars.
- `<marker>` — inline glyph (L2292).
- Inline cross-reference: nested `<text><xref thread_id="N"/>…</text>` (L1779–1799).
- Inline hyperlink: `<text><href uri="…"/>label</text>` (L1801–1813).
- Sub/superscript used for chemistry/footnote markers etc.

**Explicit absences (run-level)**: no color, font-size, font-family, font-weight numeric, highlight/background, letter-spacing, or arbitrary CSS. The `size`/`color` shown in §Property Semantics (L121–128) is *illustrative of the encoding pattern only*, not part of the controlled vocabulary. Inline styling is limited to the 8 formatting elements above (token-efficiency constraint).

## (d) Geometry / layout

- **Bounding box**: `<location resolution value/>` — sequence of **exactly 4**, interpreted alternating-axis as x0,y0,x1,y1, top-left origin; constraint x0_norm≤x1_norm, y0_norm≤y1_norm (L2556–2573; §L150–156; examples L275–299). Coordinates live **only on semantic elements** (and head sub-parts), **never on `<group>`** (L275).
- **Per-axis resolution / normalization**: `location@resolution` (exclusive axis bound), defaults to `default_resolution@width|height` or 512 (L2568). `<default_resolution width height>` sets the grid (L3025–3042). Coordinates are integer pixel-grid values normalized to resolution. Tokenizer pre-mints `<location value="0..511"/>` (L3093, L3201).
- **Sub-element geometry**: `<marker>`, `<caption>` may carry their own element head incl. `<location>` (e.g. bullet glyph bbox L499–503; caption bbox L373, L633–638).
- **Conceptual layer / z-order proxy**: `<layer value="body|background|furniture">` — body = main content, background = watermarks/etc., furniture = navigation/decoration (L2575–2591). This is a 3-value enum, **not** a numeric z-index.
- **Page physical size**: `<page_size>` head element (FUTURE) (L3222).

**Explicit geometry absences**: bbox is **axis-aligned rectangle only** — no rotated/oriented boxes, no polygons/quad points, no skew angle, no baseline, **no per-glyph/per-character coordinates or glyph boxes**, no per-word boxes, no font metrics. Columns/regions are **not** expressed geometrically — multi-column flow is captured by `<thread>` linking (see (e)), not by region/column geometry. No explicit reading-region or page-margin geometry. z-order is the 3-bucket `layer` only.

## (e) Reading order / relations / cross-references

- **Reading order** = document (serialization) order; at page breaks, all open elements are closed in reading order before `<page_break/>` and reopened after (L1873, §L1868–1916).
- **`<thread thread_id="N"/>`** — logical-component linking (element head, required `thread_id` positive int). Joins fragments of one component split across bounding boxes / columns / pages. Constraint: all threads sharing an id must be under the **same host element type** (L2478–2494; §Split structure L1530–1588; cross-column L1548–1588; cross-page L1590–1660). A list and a broken list-item can each carry their own thread id (L1889–1915).
- **`<xref thread_id="N"/>`** — outgoing cross-reference (element head; mutually exclusive with `<href>`); target must be a `<thread>`-defined id in the doc (L2496–2512; example L1779–1799).
- **`<href uri="…"/>`** — hyperlink to a URI (element head; works from text or non-text elements e.g. picture) (L2514–2534; §L1801–1822).
- **`<h_thread h_thread_id>`** — horizontal threading for table content spanning pages sidewise (**FUTURE**, Appendix C L3207–3209; full commented example L1662–1771). When `ucel`/`lcel`/`h_thread` resolve linkage, `<thread>` is omitted as redundant.
- **Caption ↔ host** association via element-head `<caption>` (L2436).
- **Key ↔ value(s)** association via `<field_item>` scoping (L722–733).
- **Spanning relations** in tables via `lcel`/`ucel`/`xcel` (see (b)).

## (f) Provenance / authority / confidence / source-tier

- `<generated_by>` — upstream pipeline / VLM id (head, FUTURE) (L3224, L3251).
- `<language classifier="…" score="0..1">` — detection tool + confidence (L3223, L3245–3246).
- `<topic topic_taxonomy="…" score="0..1">` — topic classification + confidence; multiple allowed (L3225, L3247–3248).
- `<document_hash hash_function="…">` — integrity hash; multiple allowed (L3227, L3249).
- `training_provenance_required` governance flag (L3501).
- `pii_source_type` (provided/derived/third-party) (L3399).

**Authority / source-tier absences**: confidence (`score`) exists **only** for `language` and `topic` head classifiers — there is **no per-block, per-run, or per-cell confidence/authority/source-tier annotation** on document content. No "extracted vs authored", no OCR-confidence per span, no element-level provenance other than via free `<custom>`/governance overrides.

## (g) Metadata

- **Document head `<head>`** (L1996; reserved core elements FUTURE §L3211–3263):
  - `<title>`; `<author_info>`/`<author>` with 0+ `<affiliation>`; `<date>` (ISO 8601); `<page_size>`; `<language>` (ISO 639-3, multi, with classifier/score); `<generated_by>`; `<topic>` (taxonomy, score, multi); `<summary>`; `<document_hash>` (multi); `<default_resolution>` (L3217–3254).
  - **Head-level custom elements** allowed (e.g. `<my_company_hap_filter_hate>`) — namespaced (L3256–3259).
- **Element-head custom metadata**: `<custom>` carrying namespaced/prefixed app-specific XML (e.g. SMILES) (L2536–2554; §Custom vocabularies L1917–1945; naming/namespacing guidance Appendix B L3077–3086).
- **Governance & compliance metadata** (FUTURE, §L3265–3712; head-level, MAY be overridden per-component L3270):
  - Licensing/rights: `<licenses>`/`<license>` (L3357, L3556).
  - Classification/privacy posture: `<data_classification>`/`<data_class>` (L3361).
  - Acceptable use: `<acceptable_use>`/`<purpose>` (L3367).
  - Stewardship/contact: `<stewardship>`/`<steward>` (name/contact/org) (L3371, L3570).
  - Access control: `<access_policy>`/`<policy>`/`<roles>`/`<role>` (L3375, L3578).
  - Retention: `<retention_policy>` (`<retention_period unit>`, `<deletion_method>`, `<documentation>`) (L3381, L3588).
  - Compliance frameworks: `<compliance_requirements>`/`<compliance_req>` (L3385, L3597).
  - **Privacy & PII controls** (19 elements, L3395–3416): `pii_status, pii_sensitivity_level, pii_source_type, controller_processor_role, pii_processing_purpose, pii_lawful_basis, special_category_condition, pii_minimisation_status, pii_transformation_level, reidentification_risk, access_control_level, ai_use_restriction, cross_border_transfer_status, transfer_mechanism, retention_category, dsr_impact_flag, dpia_required, children_pii_present, automated_decisioning_relevance, logging_monitoring_enabled`.
  - **Data extraction controls** (14, L3430–3445): `extraction_permitted, extraction_scope, extraction_purpose, extraction_granularity, pii_extraction_allowed, sensitive_data_extraction_allowed, extraction_transformation_required, extraction_output_constraints, downstream_sharing_permitted, downstream_usage_restrictions, extraction_audit_required, extraction_audit_retention, human_in_the_loop_required, automated_decisioning_dependency`.
  - **RAG/retrieval controls** (15, L3459–3475): `rag_permitted, rag_indexing_allowed, rag_embedding_scope, rag_chunking_constraints, rag_query_restrictions, rag_output_attribution_required, rag_output_transformation_required, rag_pii_exposure_allowed, rag_sensitive_data_exposure_allowed, rag_downstream_sharing_permitted, rag_caching_allowed, rag_cache_retention, rag_audit_required, rag_audit_retention, rag_model_scope`.
  - **Training controls** (15, L3489–3505): `training_permitted, training_scope, training_purpose, training_model_type, training_data_retention, training_dataset_reuse_allowed, training_derivative_sharing_permitted, training_pii_included, training_sensitive_data_included, training_transformation_required, training_provenance_required, training_audit_required, training_audit_retention, model_output_usage_constraints, right_to_be_forgotten_applicability`.
  - **Governance profiles**: minimal (recommended baseline) vs full (L3510–3535). Standards alignment table (GDPR, EU AI Act, ISO 27001/27701/23894, HIPAA, PCI DSS, FedRAMP) L3287–3296.

## (h) Annotations / overlays (segmentation, comments, change-tracking)

**Largely ABSENT — no stand-off annotation layer.** DocLang carries structure as inline XML markup, not as offset-anchored overlays. Specifically:
- **No segmentation overlay** — no sentence/segment element or stand-off range annotation. (`<text>` = paragraph-level cohesive text; "virtual text" is structural, not segmentation.)
- **No comments / review notes** element.
- **No change-tracking / revision / redline / insertion-deletion** element (only `<strikethrough>` as visual formatting, L2737).
- **No highlight/markup annotation** layer.
- Nearest available carriers: `<handwriting>` (handwritten annotations as inline formatting, L2797), `<custom>` (free namespaced app metadata per element), governance head metadata. Threading (`<thread>`) is structural linking, not annotation.

## (i) Localization-specific constructs

**Largely ABSENT — DocLang is a monolingual document representation.**
- `<language>` (ISO 639-3) in head, multiple entries allowed — but these are *document language detection* signals, not a multilingual content model (L3223).
- `<rtl>` text-direction marker (L2817) — bidi support.
- **No translation units, no source/target pairing, no variants, no locale-keyed content, no bilingual/multilingual alternation, no `xml:lang` per element, no segmentation for translation.** DocLang expresses one document in one language; translation state (targets, TM matches, segments, variants) is entirely out of scope and would be layered externally.

## (j) Anything else (assets/media, embedded content, versioning, profiles, extension mechanisms)

- **Assets / media**: `<picture>` with `<src uri="…"/>` — by reference (http/relative URI) **or** inline base64 `data:` URI (RFC 2397) (L2597–2613; §Pictures L301–335). `<href>` for link targets. (No `<video>`/`<audio>`/binary-media element beyond picture src.)
- **Embedded / nested content**: arbitrary nesting — semantic elements inline via nesting (L2044); nested tables/lists/pictures inside table cells (L645–698); inline code/formula; chart-structured data via `<tabular>` (OTSL) inside `<picture class="chart">` (L323–335, L2615–2633); foreign XML via `<custom>` + namespaces (SMILES example L1925–1945).
- **Whitespace / escaping**: `<content>` (preserve), CDATA `<![CDATA[…]]>`, XML entities (L134–140, L2653–2671).
- **Versioning**: `version` MAJOR.MINOR with SemVer compatibility semantics; 0.x all-breaking; XSD patch level (§L219–245).
- **Profiles / conformance levels**: governance minimal/full profiles (L3510–3535); machine conformance via reference validator + XSD (L255, L1958).
- **Extension mechanisms** (three): (1) `<custom>` element-head + head-level custom elements with XML namespaces / collision-resistant prefixes (L2536, L1917–1945, L3077–3086); (2) `<label value>` free-form subclass, explicitly **not validated** for extensibility (L203, L2472); (3) `class` attribute bounded enums for structurally-significant subtypes.
- **Token vocabulary** (Appendix B L3088–3201): defines the DocLang-compliant tokenizer token set — start/end tags per element, pre-minted heading/field_heading levels 1–6, `<location value="0">..511`, checkbox states, CDATA markers, custom SMILES tokens — a normative-ish artifact for LLM tokenizer alignment.
- **Property-as-element encoding** (L118–132) is itself a reusable mechanism for adding bounded properties token-efficiently.

---

## Compact element census (for cross-checking the gap analysis)

Special (3): `doclang, head, page_break`.
Semantic (20): `text, heading, footnote, page_header, page_footer, field_region, list, table, index, formula, code, picture, marker, group, field_heading, field_item, key, value, hint, caption`.
Property/head (7): `label, thread, xref, href, custom, location, layer`.
Payload (4): `src, tabular, checkbox, content`.
Formatting (8): `bold, italic, underline, strikethrough, superscript, subscript, handwriting, rtl`.
Structural cell/list (11): `fcel, ecel, ched, rhed, corn, srow, lcel, ucel, xcel, nl, ldiv`.
Doc-head metadata (1 normative + FUTURE set): `default_resolution`; FUTURE: `title, author_info, author, affiliation, date, page_size, language, generated_by, topic, summary, document_hash` + governance/PII/extraction/RAG/training elements (~80) + `h_thread`.
**Class attrs (bounded enums)**: `picture@class{undefined,chart}`, `list@class{unordered,ordered}`, `value@class{read_only,fillable}`, `checkbox@class{selected,unselected}`.

## Headline structural conclusions (for the G-axis yardstick)

1. DocLang reaches **G3 fully and G4 partially**: rich logical roles, header/spanned/section cells, list/field structure, reading-order via threads, captions, cross-refs — plus **axis-aligned bbox + resolution + 3-value layer** geometry. It does **NOT** carry rotation, polygons, per-glyph/per-word boxes, baselines, font metrics, or column/region geometry (columns are thread-linked, not geometric). So "G4 +geometry/bbox/glyphs" — DocLang has bbox but **not glyphs**.
2. Strong on **semantic structure (b)**, **tables (b)**, **forms/fields (b)**, **geometry-as-bbox (d)**, **governance/PII/RAG/training metadata (g)**, **provenance-as-classifier-scores (f)**.
3. Deliberately thin on **inline styling (c)** (8 formatting tags, no color/font), and **absent** on **annotation/overlay (h)** (no segmentation/comments/change-tracking) and **localization (i)** (monolingual; no targets/variants/TUs) — these are where neokapi's overlay + variant/target model *exceeds* DocLang and where DocLang offers no yardstick element.
