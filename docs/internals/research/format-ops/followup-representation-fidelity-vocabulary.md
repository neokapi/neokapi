# Representation fidelity as a measurable maturity axis: prior art and design

Research date: 2026-06-11. Question: how to systematically enumerate, per format, the semantic
constructs the format can express, and score whether neokapi's Block/Run/Overlay model represents
each losslessly, lossily, or not at all — turning "vocabulary fidelity" into a measurable axis of
the L0–L4 format-maturity model.

## Findings

### 1. W3C ITS 2.0 — the canonical, format-independent localization-semantics vocabulary

ITS 2.0 (W3C Recommendation, 29 Oct 2013) defines exactly the construct list neokapi is looking
for, as **data categories** — format-independent semantic concepts that are then *mapped* into
concrete formats. Section 2.1 of the spec enumerates the full set
(https://www.w3.org/TR/its20/):

1. Translate (is content for translation / do-not-translate)
2. Localization Note (translator notes/comments, with type alert|description)
3. Terminology (term flags + definition/reference pointers)
4. Directionality (base writing direction / bidi)
5. Language Information (language of content)
6. Elements Within Text (inline vs block vs nested-flow semantics — i.e. neokapi's "is this a
   sub-flow or inline code" question)
7. Domain (subject-matter for MT/TM routing)
8. Text Analysis (entity/concept annotation)
9. Locale Filter (content applies only to certain locales — conditional text)
10. Provenance (who/what touched the content — human/MT/reviser agents)
11. External Resource (references to external localizable resources — embedded media)
12. Target Pointer (where the target lives relative to source in the same doc — e.g. bilingual
    key-value files)
13. Id Value (stable unique ID / context key for a unit)
14. Preserve Space (whitespace semantics)
15. Localization Quality Issue (QA findings, typed + severity)
16. Localization Quality Rating (doc/unit-level quality score)
17. MT Confidence (per-segment MT score)
18. Allowed Characters (character-class constraints on target)
19. Storage Size (max storage size with encoding — length constraints)
20. (+ ITS Tools Annotation, processor provenance, §2.6)

Three properties make ITS the right seed taxonomy:

- **Per-category conformance as a coverage matrix.** ITS conformance is claimed *per data
  category*: a processor must implement at least one, and "must list all data categories they
  implement, and for each data category, which type of selection they support" (global rules vs
  local markup) — Conformance clause 2-1, §4.2 (https://www.w3.org/TR/its20/). The conformance
  statement *is* a per-construct support matrix; this is precisely the publication shape
  vocabulary.yaml needs.
- **Format mapping discipline.** ITS shows how one abstract category surfaces differently per
  format: in HTML5 each local data category is realized as an `its-*` attribute (Appendix I), and
  four categories are deliberately mapped onto *native* HTML markup (`lang`, `id`, `translate`,
  phrasing-content ⇒ `withinText="yes"`) rather than duplicated (§2.5.3). The lesson: a
  vocabulary entry has an abstract ID plus a per-format *realization* note ("expressed natively
  by X").
- **Executable conformance.** The ITS 2.0 test suite (https://github.com/w3c/its-2.0-testsuite)
  is organized per data category with `inputdata/`, `expected/` (reference outputs in a
  *normalized neutral dump format*), and `outputimplementers/` directories; a generated
  `testsuiteDashboard.html` matrix (from `testsuiteMaster.xml` via XSLT) marks each
  implementation × test as OK / error / fnf / N-A. The W3C exit criterion was "two or more
  independent implementations pass each test" (§4.2, https://www.w3.org/TR/its20/). Note the
  comparison trick: semantic conformance is checked by **canonicalizing into a neutral
  expected-output format and then comparing exactly** — semantic comparison reduced to byte
  comparison of a canonical dump.

ITS also closes the loop with XLIFF: the ITS Interest Group maintained a category-by-category
ITS→XLIFF mapping (https://www.w3.org/International/its/wiki/XLIFF_Mapping, with
https://www.w3.org/International/its/wiki/XLIFF_2.html for 2.0), which became the official
**ITS Module in XLIFF 2.1** — mostly reusing the W3C namespace `https://www.w3.org/2005/11/its/`,
with an OASIS namespace `urn:oasis:names:tc:xliff:itsm:2.1` only for attributes ITS itself lacks
(e.g. `itsm:domains`, `itsm:lang`)
(https://docs.oasis-open.org/xliff/xliff-core/v2.1/xliff-core-v2.1.html). So ITS categories are
already the lingua franca between source formats and the interchange format.

### 2. TTML feature designators — the cleanest machine-readable "format vocabulary" mechanism

TTML2 (https://www.w3.org/TR/ttml2/) defines ~254 **feature designators** in Appendix E — stable
URI fragments naming individual format constructs (`#ruby`, `#textOrientation`, `#animate`,
`#backgroundColor`, …) — plus Appendix F **extension designators** for constructs defined outside
the spec. A **profile definition document** (`ttp:profile` containing `ttp:feature` elements with
`value="required|optional|use"`) then declares, per feature, what a *document class* requires or
a *processor* supports. Two distinct profile kinds matter:

- **Content profiles** declare which features a document (class) may/shall use.
- **Processor profiles** declare which features an implementation must support; per the IMSC
  profiles, "presentation processors SHALL implement support for features designated as
  permitted … and MAY implement support for features designated as optional"
  (https://www.w3.org/TR/ttml-imsc1.1/, IMSC 1.2: https://www.w3.org/TR/ttml-imsc/).

A public **profile registry** (https://www.w3.org/TR/ttml-profile-registry/) assigns absolute
URIs to profiles so codecs/players can negotiate by designator. IMSC and DAPT are exactly
"vocabulary subsets as profile documents."

The structural insight for neokapi: TTML separates (a) the universe of constructs (flat list of
stable URIs anchored in the spec), (b) what a given document/format *can express* (content
profile), and (c) what a given implementation *supports* (processor profile). "Vocabulary
fidelity" is the delta between (b) and (c) — and only constructs in (b) should ever be scored
against an implementation.

### 3. XLIFF 2.x — core/modules as a vocabulary partition; SOUs as a cautionary tale

XLIFF 2.0 split the standard into a small Core plus independent modules: Translation Candidates,
Glossary, Format Style, Metadata, Resource Data, Change Tracking, Size and Length Restriction,
Validation (each with its own namespace, evolvable independently —
https://multilingual.com/articles/an-introduction-to-xliff-2-0/). XLIFF 2.1 added the ITS module
(https://docs.oasis-open.org/xliff/xliff-core/v2.1/xliff-core-v2.1.html). XLIFF 2.2 (Committee
Specification, 13 March 2025) restructured into Part 1 Core
(https://docs.oasis-open.org/xliff/xliff-core/v2.2/xliff-core-v2.2-part1.html) and Part 2
Extended (https://docs.oasis-open.org/xliff/xliff-core/v2.2/xliff-extended-v2.2-part2.html),
adding the **Plural, Gender, and Select module** (`urn:oasis:names:tc:xliff:pgs:1.0`) for message
variants — the direct interchange counterpart of neokapi's Plural/Select runs.

The modules are themselves a ready-made construct taxonomy chunk: each module ≈ one
localization-relevant capability family (alt-trans candidates, glossary/term links, HTML-ish
formatting hints via `fs:fs`/`fs:subFs`, custom metadata, embedded reference binaries, tracked
changes, size/length restriction with pluggable profiles, validation rules). Inline codes add a
second chunk: XLIFF 2 `<ph>/<pc>/<sc>/<ec>/<mrk>/<sm>/<em>` with `type`/`subType`, and `dir`,
`canCopy/canDelete/canReorder` editing constraints (Part 1 Core spec above) — neokapi's Run union
maps 1:1 onto this, so the inline-code attribute vocabulary (type categories `fmt`, `ui`,
`quote`, `link`, `image`, `other` + `subType`) is the canonical inline-markup category list.

How support is *evidenced* in the XLIFF world is the cautionary tale. OASIS standardization
relies on **Statements of Use**: the "XLIFF 2.1 support in CAT tools" report (Morado Vázquez &
Filip, 2018, https://archive-ouverte.unige.ch/unige:105631; Semantic Scholar:
https://www.semanticscholar.org/paper/XLIFF-2.1-support-in-CAT-tools-V%C3%A1zquez-David/3f1f0ba3b44d72c93c3b5e74aca924ef927f4cd7)
documents just five voluntary SOUs collected Aug–Oct 2017 — self-declared, no shared test
harness, no per-module matrix. The earlier "XLIFF Version 2.0 Support in CAT Tools" (Morado
Vázquez & Filip, 2014, https://archive-ouverte.unige.ch/unige:75409) did build a small test-file-
driven matrix. Net: self-declaration without executable evidence produced support claims nobody
can verify — the anti-pattern vocabulary.yaml must avoid.

### 4. Feature-support matrices that work: BCD, caniuse, web-features/Baseline, WPT linkage

These four artifacts converge on one record shape and add three distinct governance mechanisms.

**MDN browser-compat-data** (schema:
https://github.com/mdn/browser-compat-data/blob/main/schemas/compat-data-schema.md). Per feature,
a `__compat` object holds `description`, `spec_url` (mandatory when `standard_track: true` —
*every feature must cite its spec*), `status: {experimental, standard_track, deprecated}`, and
per-browser support statements: `version_added` (mandatory; string version or `false`),
`version_removed`, `prefix`/`alternative_name`, `flags`, `impl_url` (link to implementation
tracking), `notes` (markdown), and crucially **`partial_implementation: true` which by
convention requires an accompanying note explaining the divergence** (e.g. `{"version_added":
"6", "partial_implementation": true, "notes": "The event handler is supported, but the event
never fires."}`). Partiality is never a bare flag — it always carries prose explaining *what* is
lossy. A `"mirror"` value lets derivative browsers inherit upstream data (maintenance economy).

**caniuse** (https://github.com/Fyrd/caniuse/blob/main/CONTRIBUTING.md) encodes per-version
support as compact letter codes — `y` yes, `a` almost/partial, `n` no, `p` polyfill-only, `u`
unknown, `x` prefixed, `d` behind-flag — with `#n` note references into a `notes_by_num` map, so
every `a` (partial) is resolvable to a specific explanation. Feature metadata carries `spec`,
`status` (ls/wd/cr/pr/rec/unoff), `links`, `bugs`. The takeaway is the **closed status enum with
mandatory note linkage for the partial state**, plus `u` (unknown) as an honest first-class state
distinct from `n`.

**web-features / Baseline** (https://github.com/web-platform-dx/web-features,
https://github.com/web-platform-dx/web-features/blob/main/docs/baseline.md; feature-complete
March 2025 per https://www.w3.org/2025/03/26-web-baseline-minutes.html). A "feature" is a
*human-curated grouping* of many fine-grained BCD compat keys (`compat_features`), at the
granularity developers think in. Baseline status is computed, not asserted: `low` ("newly
available") requires that "for each current stable release in the core browser set, the release
supports the feature (as reported by the current version of @mdn/browser-compat-data),
**excluding those features identified as having partial_implementation**"; `high` ("widely
available") requires the keystone date (last browser to ship) to be ≥30 months old. Features
whose spec "contains discouraging language, such as a deprecation notice" are barred from
Baseline, and an owners group can apply documented **editorial overrides**. Three reusable
mechanisms: (1) partial support *never* counts as support for status computation; (2) status is
derived from evidence data by a published algorithm; (3) curation/overrides are explicit and
attributed, not silent.

**WPT ↔ web-features linkage.** WPT directories carry `WEB_FEATURES.yml` files mapping feature
IDs to test-file globs, e.g.
https://github.com/web-platform-tests/wpt/blob/master/css/css-transforms/WEB_FEATURES.yml uses
`features: [{name: transforms2d, files: ["*", "!*3d*", "!perspective-*", …]}]`; wpt.fyi then
filters live test results by `feature:transforms3d`
(https://github.com/tc39/test262/issues/4043 documents the same mechanism being copied to
Test262). This is the **claims-must-bind-to-tests** pattern: the feature inventory and the test
suite are joined by a small, reviewable mapping file kept *next to the tests*.

**Synthesized record shape** recurring across all four:

```
{feature-id, spec-citation(URL+fragment), support-status(enum incl. partial+unknown),
 since-version, evidence(test-ids/impl_url), notes(mandatory when partial), curation-override?}
```

### 5. Lossiness taxonomies in converter ecosystems

**pandoc** is the most honest large-scale converter. The manual states up front: "Because
pandoc's intermediate representation of a document is less expressive than many of the formats
it converts between, one should not expect perfect conversions between every format and every
other… Conversions from formats more expressive than pandoc's Markdown can be expected to be
lossy," while "conversions from pandoc's Markdown to all formats aspire to be perfect"
(https://pandoc.org/MANUAL.html). Two patterns to steal:

- **A declared fidelity anchor.** Pandoc names its native model (Pandoc-Markdown/AST) and makes
  loss *directional and predictable* relative to it, instead of promising per-pair fidelity.
- **Per-construct loss policy, user-selectable.** For docx tracked changes — a construct its AST
  cannot represent natively — `--track-changes accept|reject|all` offers three policies: resolve
  (accept), discard (reject), or lossy-preserve as spans with `insertion`/`deletion`/
  `comment-start`/`comment-end` classes (https://pandoc.org/MANUAL.html). The failure mode is
  also documented in issues: the preserved spans "clutter the output" of other writers
  (https://github.com/jgm/pandoc/issues/4301) and writer support regressed independently of the
  reader (https://github.com/jgm/pandoc/issues/4303) — proof that **reader-side and writer-side
  support for the same construct must be tracked as separate cells**, or asymmetries rot
  silently.

**Sanity Portable Text** (https://github.com/portabletext/portabletext) is a spec'd minimal
rich-text vocabulary: blocks with `children` spans; marks split into **decorators** (plain
strings, e.g. `"emphasis"`) and **annotations** (keys into a stand-off `markDefs` array carrying
data) — structurally the same split as neokapi's Run flags vs Overlays. Custom constructs ride on
`_type`, and serializers are expected to handle unknown `_type`s. **Contentful Rich Text**
(https://www.contentful.com/developers/docs/concepts/rich-text/) sits at the opposite pole: a
closed node/mark vocabulary where "Custom node types and marks are not allowed" and invalid
structures are *rejected by validation* rather than degraded. The spectrum — reject / drop /
lossy-preserve / extend — is the policy enum a vocabulary entry needs when support ≠ lossless.

### 6. Okapi Framework's own approach

Okapi documents capability per filter in two ways, neither machine-checked: (1)
`IFilter.QueryProperty()` lets callers probe a handful of coarse runtime capabilities
(https://okapi.sourceforge.net/IFilter.html); (2) per-filter wiki pages carry hand-written
support/limitation bullets — e.g. the XLIFF-2 filter page lists "Basic support for XLIFF 2.x
core," support for inline codes, notes, groups and the **Metadata module only**, and limitations
like "Skeleton not supported," "Comments are lost in the merged document," "Original XML
formatting lost in merged document," "Attributes can be reordered"
(https://okapiframework.org/wiki/index.php/XLIFF-2_Filter; filter index:
https://okapiframework.org/wiki/index.php/Filters). The limitation lists are genuinely useful
prose, but they are unversioned, untested, and not enumerable across filters — exactly the
"vibes" state neokapi wants to leave. (Okapi's MultilingualWeb-LT deliverable D3.1.4 also shows
its ITS-categories-onto-filters work: https://www.w3.org/International/multilingualweb/lt/wiki/images/8/8b/D3.1.4.pdf.)

### 7. Measuring semantic round-trip beyond byte equality

- **Canonical-dump comparison (ITS test suite).** Conformance outputs are serialized into a
  normalized, line-oriented neutral format and compared exactly
  (https://github.com/w3c/its-2.0-testsuite). Generalization: define a canonical *construct
  dump* (every construct instance found, normalized, ordered), and compare dumps across
  read→write→read round-trips. Byte-inequality of the file with byte-equality of the dump =
  "semantically faithful, serialization divergent" — a measurable middle state.
- **SCORE-Bench** (Unstructured, 2 Dec 2025,
  https://unstructured.io/blog/introducing-score-bench-an-open-benchmark-for-document-parsing)
  measures parsing fidelity on three axes: content fidelity via **Adjusted CCT** (word-weighted
  fuzzy alignment of clean concatenated text, tolerant of structural re-representation),
  hallucination control (percent tokens found vs added), and structural understanding
  (cell-level table accuracy). Its core principle: "distinguish legitimate representational
  variation from actual data loss." For neokapi this maps to: text-content equality (strict),
  construct-set equality (canonical dump), and skeleton/byte equality (strictest) as three
  nested fidelity tiers.
- Generic round-trip-testing framing (convert A→B→A, diff) is widely described
  (e.g. https://en.wikipedia.org/wiki/Round-trip_format_conversion); the academic work found is
  mostly domain-specific (math expressions: https://arxiv.org/pdf/1906.11485). No off-the-shelf
  "localization construct preservation" benchmark exists — neokapi would be first.

## Design implications for neokapi

**(a) Adopt the convergent record schema in a per-format `vocabulary.yaml`.** Next to each
format's `spec.yaml`:

```yaml
format: docx
constructs:
  - id: its.localization-note            # namespaced construct ID (taxonomy below)
    realization: "w:comment / commentRangeStart"   # how THIS format expresses it
    spec: https://…#sect                 # format-spec citation (BCD: spec_url mandatory)
    expressible: true                    # TTML content-profile dimension
    read:  {status: lossy, evidence: [spec.yaml#comments-roundtrip], notes: "comment anchors collapse to block scope"}
    write: {status: none,  notes: "comments dropped on merge", policy: drop}
    since: v1.2.0
```

Key fields copied from prior art: `spec` citation per construct (BCD), separate `read`/`write`
cells (pandoc issue #4301/#4303 asymmetry), `status` enum
`lossless | lossy | dropped | rejected | unknown` (caniuse `y/a/n/u` + Contentful "reject"),
mandatory `notes` whenever status ≠ lossless ∧ ≠ none (BCD `partial_implementation` rule),
`policy` for non-lossless constructs (`preserve-as-skeleton | preserve-as-overlay | drop |
error`, the pandoc `--track-changes` pattern), `since` version, and `evidence` pointing at
spec.yaml test IDs.

**(b) Canonical construct taxonomy** — a single repo-level `constructs.yaml` (the "feature
designator registry," TTML Appendix-E style, with stable IDs and spec URLs), seeded from:

- **ITS 2.0's 19 data categories** (translate flags, loc-notes, terminology, directionality,
  language, elements-within-text, domain, text-analysis/entities, locale-filter/conditional
  text, provenance, external-resource/embedded media, target-pointer, id-value/context-key,
  preserve-space, LQ issue, LQ rating, MT confidence, allowed-characters, storage-size)
  (https://www.w3.org/TR/its20/);
- **XLIFF 2.x modules** (candidates, glossary, format-style, metadata, resource-data,
  change-tracking/tracked changes, size-length-restriction, validation, ITS, plural-gender-select)
  (https://docs.oasis-open.org/xliff/xliff-core/v2.2/xliff-extended-v2.2-part2.html);
- **XLIFF 2 inline-code vocabulary** (placeholder vs paired codes, `type`/`subType` categories,
  `dir`, `canCopy/canDelete/canReorder` constraints, sub-flows)
  (https://docs.oasis-open.org/xliff/xliff-core/v2.2/xliff-core-v2.2-part1.html);
- plus a small residue ITS/XLIFF lack: ruby annotations (TTML2 `#ruby`,
  https://www.w3.org/TR/ttml2/), timing (subtitles), message references/linked strings.

Most entries map per-format to either a Run kind, an Overlay kind, Block/Layer properties, or
skeleton — recording *which* (`model_mapping`) makes gaps in the Block/Run/Overlay model itself
visible (e.g. if many formats map a construct to `skeleton`, the model is missing a vocabulary
item — the pandoc "IR less expressive" confession, made queryable).

**(c) Make every claim executable — the WPT pattern.** Any `status: lossless|lossy` must cite
≥1 passing spec.yaml case (`evidence:`); a drift gate (like the existing
`make check-contract-types`) fails CI when (1) a construct claim has no evidence, (2) an
evidence ID doesn't exist or doesn't pass, or (3) a spec.yaml test exercises a construct absent
from vocabulary.yaml (coverage probe). This is exactly WPT `WEB_FEATURES.yml` → wpt.fyi
`feature:` filtering
(https://github.com/web-platform-tests/wpt/blob/master/css/css-transforms/WEB_FEATURES.yml),
and avoids the XLIFF SOU failure mode (unverifiable self-declaration,
https://archive-ouverte.unige.ch/unige:105631).

**(d) Publish partial/lossy support honestly.** Generate the per-format docs "Supported
constructs" table and the /format-maturity dashboard from vocabulary.yaml (never hand-written
Okapi-wiki-style bullets); render `lossy`/`dropped` rows with their mandatory notes (BCD
pattern); state the pandoc-style directional anchor once ("the Block/Run/Overlay model is the
fidelity anchor; conversions from formats more expressive than it are lossy in these enumerated
ways"); record intentional divergences as Baseline-style **editorial overrides** — explicit,
attributed entries (`divergence: {reason, link}`) rather than silently downgraded scores
(https://github.com/web-platform-dx/web-features/blob/main/docs/baseline.md). Keep `unknown` as
a first-class status (caniuse `u`) so unaudited constructs aren't silently counted as
unsupported or supported.

**(e) Score it as a maturity axis.**

- **Two-level matrix first (TTML insight):** per format, partition the registry into
  expressible vs not-expressible (content profile); score only expressible constructs
  (processor profile). Fidelity% = lossless-with-evidence / expressible, with lossy counting
  partially (e.g. 0.5) and *never* counting toward "full" (Baseline excludes
  `partial_implementation` from support computation).
- **Tie statuses to L-levels:** e.g. L2 = vocabulary.yaml exists, zero `unknown` rows
  (inventory complete); L3 = all ITS-core categories the format can express are ≥lossy with
  passing evidence, read/write asymmetries documented; L4 = every expressible construct is
  lossless **or** carries an attributed divergence note + policy. Promotion is then computed
  from the data by a published rule, like Baseline low/high, not asserted.
- **Three nested round-trip tiers as the test vocabulary for evidence cases:** text-content
  equality (Adjusted-CCT-style tolerance,
  https://unstructured.io/blog/introducing-score-bench-an-open-benchmark-for-document-parsing) ⊂
  canonical construct-dump equality (ITS test-suite normalization trick,
  https://github.com/w3c/its-2.0-testsuite) ⊂ byte equality. spec.yaml assertions should declare
  which tier they prove, so "lossless" formally means "construct-dump-equal under round-trip,"
  decoupled from the existing byte-equality parity tests.
- **Aggregate view:** a caniuse-style cross-format × construct matrix (statuses + note popovers)
  becomes the headline artifact of the /format-maturity dashboard — and doubles as the honest
  competitive matrix vs Okapi (whose own per-filter support is prose-only,
  https://okapiframework.org/wiki/index.php/XLIFF-2_Filter).
