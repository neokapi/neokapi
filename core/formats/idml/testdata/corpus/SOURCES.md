# IDML corpus — provenance

Real-world Adobe InDesign Markup Language (`.idml`) packages vendored verbatim
from permissively-licensed open-source projects. Each file is byte-identical to
its pinned source. IDML is a ZIP container whose story XML is whitespace- and
entity-fragile, so the corpus contract is **semantic**, not byte-exact:
`corpus_test.go` asserts an untouched read→write→re-read preserves the
translatable surface (every `<Content>` Block, in order, with identical source
text), that the reconstructed package is a well-formed ZIP with the same entry
set, and that a pseudo-translation re-reads into the same block sequence.

All files satisfy the semantic round-trip. One file
(`simpleidml-magazineA-template.idml`) is a layout-only template with no story
copy and therefore extracts nothing translatable; it is retained to exercise the
story-less round-trip path and is documented under **Feature coverage** below.

## Provenance

| File | Upstream repo | License | Commit |
| --- | --- | --- | --- |
| `simpleidml-article-1photo.idml` | [Starou/SimpleIDML](https://github.com/Starou/SimpleIDML) | BSD-3-Clause | `f72114d42c0def3cbe0e52b2733f9e18cdda44fd` |
| `simpleidml-magazineA-edito.idml` | [Starou/SimpleIDML](https://github.com/Starou/SimpleIDML) | BSD-3-Clause | `f72114d42c0def3cbe0e52b2733f9e18cdda44fd` |
| `simpleidml-magazineA-template.idml` | [Starou/SimpleIDML](https://github.com/Starou/SimpleIDML) | BSD-3-Clause | `f72114d42c0def3cbe0e52b2733f9e18cdda44fd` |
| `simpleidml-4-pages.idml` | [Starou/SimpleIDML](https://github.com/Starou/SimpleIDML) | BSD-3-Clause | `f72114d42c0def3cbe0e52b2733f9e18cdda44fd` |
| `simpleidml-page-9modules.idml` | [Starou/SimpleIDML](https://github.com/Starou/SimpleIDML) | BSD-3-Clause | `f72114d42c0def3cbe0e52b2733f9e18cdda44fd` |
| `batchidml-business_cards_template.idml` | [goinvo/BatchIDMLGenerator](https://github.com/goinvo/BatchIDMLGenerator) | MIT | `bb0704ca6065f600420f312046920732599c929b` |
| `okapi-06-hello-world-12.idml` | Okapi Framework, Apache-2.0, gitlab.com/okapiframework/Okapi @ `509d8f567c03` | Apache-2.0 | `509d8f567c03` |
| `okapi-09-footnotes.idml` | Okapi Framework, Apache-2.0, gitlab.com/okapiframework/Okapi @ `509d8f567c03` | Apache-2.0 | `509d8f567c03` |
| `okapi-10-tables.idml` | Okapi Framework, Apache-2.0, gitlab.com/okapiframework/Okapi @ `509d8f567c03` | Apache-2.0 | `509d8f567c03` |
| `okapi-03-hyperlink-and-table-content.idml` | Okapi Framework, Apache-2.0, gitlab.com/okapiframework/Okapi @ `509d8f567c03` | Apache-2.0 | `509d8f567c03` |
| `okapi-11-xml-structures.idml` | Okapi Framework, Apache-2.0, gitlab.com/okapiframework/Okapi @ `509d8f567c03` | Apache-2.0 | `509d8f567c03` |

The SimpleIDML files are drawn from `tests/regressiontests/IDML/`; the
BatchIDMLGenerator file from `example/`; the Okapi files from
`okapi/filters/idml/src/test/resources/`. Licenses were confirmed against each
repository's `LICENSE` file at the pinned commit (SimpleIDML: BSD-3-Clause;
BatchIDMLGenerator: MIT; Okapi: Apache-2.0).

## Feature coverage

- **`simpleidml-article-1photo.idml`** — a magazine article with a placed photo;
  multiple threaded stories. Exercises multi-story packages and image-frame
  layout alongside translatable copy.
- **`simpleidml-magazineA-edito.idml`** — a real magazine editorial page with
  three stories of body copy. Exercises a content-rich multi-page magazine
  layout (the same magazine family as the template below, but with copy).
- **`simpleidml-magazineA-template.idml`** — a layout-only magazine template:
  master spreads, paragraph/character styles, preferences, and an empty
  `XML/BackingStory.xml` (its only `<Content>` is a U+FEFF BOM), but **no
  `Stories/` directory**. It extracts nothing translatable and is retained to
  exercise the **story-less round-trip path**: a valid IDML package with no
  placed copy must read without error (Okapi folds the BackingStory into its
  part-name list and yields no translatable units —
  `DesignMapFragments.java:165,335-336`) and the writer must copy it through.
  The semantic round-trip therefore asserts an empty translatable surface for
  this file (it is excluded from the pseudo-translation invariant, which has
  nothing to translate). See `corpusZeroTranslatable` in `corpus_test.go` and
  `TestStoryLessPackageYieldsEmptyDocument` in `reader_test.go`.
- **`simpleidml-4-pages.idml`** — a four-page document with several stories.
  Exercises multi-page, multi-story packages.
- **`simpleidml-page-9modules.idml`** — a single page composed of nine modules;
  one consolidated story. Exercises a modular grid layout.
- **`batchidml-business_cards_template.idml`** — a business-card template whose
  stories carry placeholder fields. Exercises template/placeholder copy.
- **`okapi-06-hello-world-12.idml`** — minimal multi-paragraph hello-world
  stories; the canonical smoke fixture for `<Content>` extraction.
- **`okapi-09-footnotes.idml`** — footnote bodies as translatable `<Content>`.
- **`okapi-10-tables.idml`** — table cell content (`<Cell>` → `<Content>`).
- **`okapi-03-hyperlink-and-table-content.idml`** — hyperlink text sources plus
  table content in one package (the densest corpus member, 43 blocks).
- **`okapi-11-xml-structures.idml`** — XML-structured stories (`<XMLElement>`
  wrappers around `<Content>`).

## Exact fetch commands

Commit-pinned. SimpleIDML / BatchIDMLGenerator are fetched from GitHub raw; the
Okapi fixtures from the pinned GitLab raw URL.

```sh
# Starou/SimpleIDML — BSD-3-Clause @ f72114d42c0def3cbe0e52b2733f9e18cdda44fd
SI=f72114d42c0def3cbe0e52b2733f9e18cdda44fd
curl -sSL -o simpleidml-article-1photo.idml \
  "https://raw.githubusercontent.com/Starou/SimpleIDML/$SI/tests/regressiontests/IDML/article-1photo.idml"
curl -sSL -o simpleidml-magazineA-edito.idml \
  "https://raw.githubusercontent.com/Starou/SimpleIDML/$SI/tests/regressiontests/IDML/magazineA-edito.idml"
curl -sSL -o simpleidml-magazineA-template.idml \
  "https://raw.githubusercontent.com/Starou/SimpleIDML/$SI/tests/regressiontests/IDML/magazineA-template.idml"
curl -sSL -o simpleidml-4-pages.idml \
  "https://raw.githubusercontent.com/Starou/SimpleIDML/$SI/tests/regressiontests/IDML/4-pages.idml"
curl -sSL -o simpleidml-page-9modules.idml \
  "https://raw.githubusercontent.com/Starou/SimpleIDML/$SI/tests/regressiontests/IDML/page-9modules.idml"

# goinvo/BatchIDMLGenerator — MIT @ bb0704ca6065f600420f312046920732599c929b
BG=bb0704ca6065f600420f312046920732599c929b
curl -sSL -o batchidml-business_cards_template.idml \
  "https://raw.githubusercontent.com/goinvo/BatchIDMLGenerator/$BG/example/business_cards_template.idml"

# Okapi Framework — Apache-2.0 @ 509d8f567c03
OK=509d8f567c03
OKBASE="https://gitlab.com/okapiframework/Okapi/-/raw/$OK/okapi/filters/idml/src/test/resources"
curl -sSL -o okapi-06-hello-world-12.idml             "$OKBASE/06-hello-world-12.idml"
curl -sSL -o okapi-09-footnotes.idml                  "$OKBASE/09-footnotes.idml"
curl -sSL -o okapi-10-tables.idml                     "$OKBASE/10-tables.idml"
curl -sSL -o okapi-03-hyperlink-and-table-content.idml "$OKBASE/03-hyperlink-and-table-content.idml"
curl -sSL -o okapi-11-xml-structures.idml             "$OKBASE/11-xml-structures.idml"
```
