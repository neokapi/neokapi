---
id: 022-inline-code-model
sidebar_position: 22
title: "AD-022: Inline Code Model"
---
# AD-022: Semantic inline code model with format-independent rendering

## Context

Localization pipelines must process documents that contain inline formatting
(bold, italic, links, images, variables, placeholders) embedded within
translatable text. The fundamental challenge is that every source format
represents these constructs differently:

| Concept | HTML | Markdown | DOCX (OpenXML) | IDML | XLIFF 2.0 | ICU MF2 |
|---|---|---|---|---|---|---|
| Bold | `<b>` | `**` | `<w:b/>` | `FontStyle="Bold"` | `<pc type="fmt" subType="xlf:b">` | `{#bold}` |
| Link | `<a href="…">` | `[text](url)` | `<w:hyperlink>` | `HyperlinkTextDestination` | `<pc type="link">` | `{#link url=\|…\|}` |
| Line break | `<br/>` | two spaces + newline | `<w:br/>` | `<Br/>` | `<ph type="fmt" subType="xlf:lb"/>` | `{#lb /}` |
| Placeholder | — | — | — | — | `<ph>` | `{$var}` |

A localization framework must make these constructs processable in a
format-agnostic way — TM matching, AI translation, QA checks, and terminology
lookup should not need to know whether the bold text came from HTML or
Markdown. At the same time, perfect roundtrip fidelity to the original format
is required: a `<b class="emphasis">` must roundtrip as exactly that, not as
a generic bold tag.

### Industry precedent

XLIFF 2.0 solves this with a two-layer model: abstract inline elements
(`<pc>`, `<ph>`, `<sc>`/`<ec>`) carry a semantic `type`/`subType` vocabulary
while the native markup is stored separately in `<originalData>` entries
referenced by `dataRef`. This cleanly separates what a code **means** (bold,
link, variable) from what a code **is** in the native format (`<b>`, `**`,
`<w:b/>`). The same pattern appears in memoQ (numbered `{1}`/`{/1}`
placeholders with metadata), Trados (`ITagPair`/`IPlaceholderTag` with
`FormattingGroup`), and ICU MessageFormat 2.0 (markup elements
`{#tag}`/`{/tag}` with application-defined semantics).

Commercial MT APIs operate at the native markup level:

- **DeepL**: `tag_handling=xml` with `non_splitting_tags` / `ignore_tags`
  controls, or `tag_handling=html`
- **Google Cloud Translation**: `mime_type=text/html` with `translate="no"`
  attribute support
- **Amazon Translate**: HTML `translate="no"` attribute, plus native XLIFF 1.2
  batch support

LLMs (Claude, GPT-4) handle inline codes best when presented as semantic
XML-like tags (`<bold>text</bold>`) or numbered placeholders (`{1}text{/1}`).
Research shows XML-Match scores of 86–88% with specialized training (FormatRL,
2024), with the main failure modes being tag consistency over long documents
and overlapping/nested code structures.

### The separation principle

The industry has converged on a layered model. From bottom to top:

1. **Native markup** — exact bytes from the source format (for roundtrip)
2. **Abstract identity** — sequential ID + structural type
   (opening/closing/placeholder)
3. **Semantic type** — format-independent meaning (bold, link, variable)
4. **Display text** — what a translator sees in an editor
5. **Text equivalent** — plain text representation for non-aware tools
6. **Editing constraints** — can the code be copied, deleted, reordered?

gokapi's Fragment and Span model implements all six layers.

## Decision

### Fragment and Coded Text

Text with inline formatting is represented as a `Fragment` using coded text.
Unicode Private Use Area (PUA) markers replace inline markup within the text
string. Each marker maps positionally to a `Span` that records the code's
full metadata. This allows any tool to process the text as a simple string
(skipping marker runes) while preserving complete inline code information.

```go
const (
    MarkerOpening     rune = '\uE001'
    MarkerClosing     rune = '\uE002'
    MarkerPlaceholder rune = '\uE003'
)

type Fragment struct {
    CodedText string  // Text with PUA markers at span positions
    Spans     []*Span // Metadata for each marker, indexed positionally
}
```

The Nth marker character in `CodedText` corresponds to `Spans[N]`. Three
marker runes distinguish opening, closing, and placeholder (standalone) codes.

### Span: the six-layer inline code

Each `Span` carries all six layers of the inline code model:

```go
type Span struct {
    // Layer 2: Abstract identity
    SpanType    SpanType  // Opening, Closing, or Placeholder
    ID          string    // Sequential per-fragment: "1", "2", "3"

    // Layer 3: Semantic type
    Type        string    // From SemanticType vocabulary (see below)
    SubType     string    // Refinement within type (e.g., "xlf:b" for bold)

    // Layer 1: Native markup (originalData)
    Data        string    // Native format markup (e.g., "<b>", "**", "<w:b/>")
    OuterData   string    // Outer context when Data alone is insufficient

    // Layer 4: Display
    DisplayText string    // What translators see: "[B]", "[/B]", "[IMG]"

    // Layer 5: Text equivalent
    EquivText   string    // Plain text equivalent (e.g., "" for bold, "\n" for <br>)

    // Layer 6: Editing constraints
    Deletable   bool      // Whether translators may remove this code
    Cloneable   bool      // Whether translators may duplicate this code
    CanReorder  bool      // Whether this code can move relative to others

    // Internal tracking
    OriginalID  string    // ID before merge/split operations
    Flags       int       // SpanFlagHasRef, SpanFlagAdded, SpanFlagMerged, etc.
    Annotations map[string]Annotation
}
```

### Semantic type vocabulary

The `Type` field uses a defined vocabulary of format-independent semantic
types. Types are grouped into categories:

**Formatting** — visual text formatting:

| Type | Meaning | HTML | Markdown | DOCX |
|---|---|---|---|---|
| `fmt:bold` | Bold text | `<b>`, `<strong>` | `**` | `<w:b/>` |
| `fmt:italic` | Italic text | `<i>`, `<em>` | `*`, `_` | `<w:i/>` |
| `fmt:underline` | Underlined text | `<u>` | — | `<w:u/>` |
| `fmt:strikethrough` | Struck-through text | `<s>`, `<del>` | `~~` | `<w:strike/>` |
| `fmt:subscript` | Subscript text | `<sub>` | — | `<w:vertAlign w:val="subscript"/>` |
| `fmt:superscript` | Superscript text | `<sup>` | — | `<w:vertAlign w:val="superscript"/>` |
| `fmt:code` | Inline code | `<code>` | `` ` `` | — |
| `fmt:highlight` | Highlighted text | `<mark>` | — | `<w:highlight/>` |

**Linking** — references and hyperlinks:

| Type | Meaning | HTML | Markdown |
|---|---|---|---|
| `link:hyperlink` | Hyperlink | `<a href="…">` | `[text](url)` |
| `link:crossref` | Cross-reference | `<a href="#id">` | `[text](#anchor)` |
| `link:email` | Email link | `<a href="mailto:…">` | — |

**Media** — embedded content:

| Type | Meaning | HTML | Markdown |
|---|---|---|---|
| `media:image` | Inline image | `<img>` | `![alt](url)` |
| `media:video` | Inline video | `<video>` | — |
| `media:audio` | Inline audio | `<audio>` | — |

**Structure** — structural inline codes:

| Type | Meaning | HTML | Markdown |
|---|---|---|---|
| `struct:break` | Line break | `<br>` | `  \n` |
| `struct:pagebreak` | Page break | — | — |
| `struct:footnote` | Footnote reference | — | `[^id]` |
| `struct:ruby` | Ruby annotation | `<ruby>` | — |

**Code** — non-translatable inline tokens:

| Type | Meaning | Examples |
|---|---|---|
| `code:variable` | Named variable | `{name}`, `$name`, `%s` |
| `code:placeholder` | Positional placeholder | `{0}`, `%1$s` |
| `code:function` | ICU function | `{count, plural, …}` |
| `code:markup` | Generic preserved markup | arbitrary format-specific tags |

**Entity** — named entities (also used by entity annotation, see
[AD-010](./010-terminology.md)):

| Type | Meaning |
|---|---|
| `entity:person` | Person name |
| `entity:organization` | Organization name |
| `entity:product` | Product name |
| `entity:location` | Place name |
| `entity:date` | Date value |
| `entity:time` | Time value |
| `entity:currency` | Currency amount |
| `entity:measurement` | Measurement value |

The `SubType` field provides format-specific refinement using a prefix
convention. Reserved prefixes:

- `xlf:` — XLIFF 2.0 subtypes (`xlf:b`, `xlf:i`, `xlf:u`, `xlf:lb`,
  `xlf:pb`, `xlf:var`)
- `html:` — HTML element names (`html:span`, `html:div`, `html:em`)
- `md:` — Markdown constructs (`md:emphasis`, `md:strong`)
- `docx:` — OpenXML run properties (`docx:w:b`, `docx:w:i`)

Custom subtypes use a reverse-domain prefix: `com.acme:custom-tag`.

### Span ID assignment

Format readers assign sequential numeric IDs to spans within each Fragment.
Opening and closing spans that form a pair share the same ID. Placeholder
spans get their own ID. IDs are assigned in document order starting from `"1"`:

```
Input:  Click <b>here</b> for <a href="x">info</a>.
Spans:  [Opening id="1"] [Closing id="1"] [Opening id="2"] [Closing id="2"]
```

This produces stable structural keys for TM matching: `Click {1}here{/1}
for {2}info{/2}.` — two different documents with the same inline code
structure produce the same structural key regardless of format.

### Fragment text projections

Fragment provides multiple views of its content, each serving a different
processing need:

```go
// Plain text — markers stripped, spans ignored.
// Use for: word count, search, QA text comparison.
func (f *Fragment) Text() string

// Structural text — markers replaced by numbered placeholders.
// {1}text{/1}, {2/} for placeholders.
// Use for: TM exact matching (structural tier).
func (f *Fragment) StructuralText() string

// Generalized text — entity spans become typed placeholders ({PERSON}),
// structural spans become numbered.
// Use for: TM generalized matching (entity-agnostic tier).
func (f *Fragment) GeneralizedText() string

// Semantic HTML — renders spans as semantic HTML elements.
// Use for: MT APIs (DeepL tag_handling=html, Google mime_type=text/html),
// translation editor preview, AI translation with context.
func (f *Fragment) SemanticHTML() string

// Placeholder text — numbered XML placeholders for LLM prompts.
// <x id="1"/>text<x id="/1"/> format.
// Use for: LLM translation prompts where tag preservation is critical.
func (f *Fragment) PlaceholderText() string
```

`SemanticHTML()` uses the semantic type vocabulary to produce format-agnostic
HTML:

```
Fragment: "Click \uE001here\uE002 for info"
  Span[0]: {SpanOpening, Type:"fmt:bold", ID:"1", Data:"<b class='x'>"}
  Span[1]: {SpanClosing, Type:"fmt:bold", ID:"1", Data:"</b>"}

Text():            "Click here for info"
StructuralText():  "Click {1}here{/1} for info"
SemanticHTML():    "Click <b>here</b> for info"
PlaceholderText(): "Click <x id=\"1\"/>here<x id=\"/1\"/> for info"
```

The semantic type → HTML mapping is defined by a `SemanticHTMLMap`:

| Semantic type | HTML open | HTML close | HTML placeholder |
|---|---|---|---|
| `fmt:bold` | `<b>` | `</b>` | — |
| `fmt:italic` | `<i>` | `</i>` | — |
| `fmt:underline` | `<u>` | `</u>` | — |
| `fmt:code` | `<code>` | `</code>` | — |
| `link:hyperlink` | `<a>` | `</a>` | — |
| `media:image` | — | — | `<img/>` |
| `struct:break` | — | — | `<br/>` |
| `code:variable` | — | — | `<span class="code"/>` |

Unknown types fall back to `<span data-type="…">` for opening/closing
and `<span data-type="…"/>` for placeholders, ensuring every span renders.

### Format reader contract

Format readers populate all six span layers when producing Fragments:

1. **ID** — sequential numeric, paired opening/closing share the same ID
2. **Type** — from the semantic type vocabulary
3. **Data** — verbatim native markup string for roundtrip fidelity
4. **DisplayText** — short human-readable label (e.g., `"[B]"`, `"[IMG]"`)
5. **EquivText** — plain text equivalent where applicable (e.g., `"\n"` for
   line breaks, empty string for formatting)
6. **Editing constraints** — `Deletable`, `Cloneable`, `CanReorder` based on
   format semantics (formatting codes are typically deletable and reorderable;
   structural codes like page breaks may not be)

Example: the HTML reader processing `<b class="emphasis">`:

```go
frag.AppendSpan(&Span{
    SpanType:    SpanOpening,
    ID:          "1",
    Type:        "fmt:bold",
    Data:        `<b class="emphasis">`,
    DisplayText: "[B]",
    EquivText:   "",
    Deletable:   true,
    Cloneable:   true,
    CanReorder:  true,
})
```

The Markdown reader processing `**`:

```go
frag.AppendSpan(&Span{
    SpanType:    SpanOpening,
    ID:          "1",
    Type:        "fmt:bold",
    Data:        "**",
    DisplayText: "[B]",
    EquivText:   "",
    Deletable:   true,
    Cloneable:   true,
    CanReorder:  true,
})
```

Both produce `Type: "fmt:bold"` — the TM, AI tools, and QA checks see
identical semantic content regardless of source format.

### Format writer contract

Format writers reconstruct output using `Span.Data` (the native markup),
not the semantic type. This ensures perfect roundtrip fidelity — the writer
replays exactly what the reader captured. The semantic type is never used for
output generation; it exists solely for format-independent processing.

Writers iterate `CodedText` rune by rune. When a marker rune is encountered,
the writer emits `Spans[idx].Data` and advances the span index:

```go
func renderFragment(frag *Fragment) string {
    var buf strings.Builder
    spanIdx := 0
    for _, r := range frag.CodedText {
        if isMarker(r) && spanIdx < len(frag.Spans) {
            buf.WriteString(frag.Spans[spanIdx].Data)
            spanIdx++
        } else {
            buf.WriteRune(r)
        }
    }
    return buf.String()
}
```

### AI and MT translation with inline codes

AI tools and MT providers use the appropriate Fragment projection based on
the backend's tag-handling capability:

**Commercial MT APIs** — use `SemanticHTML()`:

```go
func (t *MTTranslateTool) handleBlock(ctx context.Context, block *Block) (*Block, error) {
    frag := block.FirstFragment()
    // Render as semantic HTML for the MT API
    sourceHTML := frag.SemanticHTML()
    // MT API preserves HTML tags natively
    targetHTML, err := t.provider.Translate(ctx, sourceHTML, src, tgt)
    // Parse the response HTML back into a Fragment
    targetFrag := ParseSemanticHTML(targetHTML, frag.Spans)
    block.SetTargetFragment(tgt, targetFrag)
}
```

`ParseSemanticHTML` maps the response HTML tags back to the original Spans
(matched by ID or position), restoring `Data` and all metadata from the
source spans. The MT API preserves the semantic HTML tags; the framework
restores the native markup from the original Span data.

**LLM translation** — use `PlaceholderText()` or `SemanticHTML()`:

```go
func (t *AITranslateTool) handleBlock(ctx context.Context, block *Block) (*Block, error) {
    frag := block.FirstFragment()
    if frag.HasSpans() {
        // Use placeholder text in the prompt
        sourceText := frag.PlaceholderText()
        prompt := fmt.Sprintf(
            "Translate to %s. Preserve all XML tags exactly:\n%s",
            targetLocale, sourceText,
        )
        response, _ := t.provider.Chat(ctx, prompt)
        targetFrag := ParsePlaceholderText(response, frag.Spans)
        block.SetTargetFragment(targetLocale, targetFrag)
    } else {
        // No spans — plain text translation
        sourceText := block.SourceText()
        response, _ := t.provider.Translate(ctx, sourceText, src, tgt)
        block.SetTargetText(targetLocale, response)
    }
}
```

`ParsePlaceholderText` reconstructs a Fragment from the LLM response by
matching placeholder tags (`<x id="1"/>`) back to source Spans. Unmatched
or missing tags produce warnings in the QA pipeline.

**Pseudo-translation** — the reference implementation for span-preserving
tools:

```go
func pseudoTranslate(frag *Fragment) *Fragment {
    target := frag.Clone()
    // Transform only non-marker runes in CodedText
    target.CodedText = transformText(target.CodedText, skipMarkerRunes)
    return target
}
```

`Clone()` deep-copies both `CodedText` and the `Spans` slice. The
pseudo-translate function modifies only text runes, skipping marker
characters. The Spans array is untouched — the target Fragment has identical
inline code structure.

### TM matching with inline codes

The three-tier TM matching pipeline ([AD-009](./009-translation-memory.md))
uses Fragment projections to match at different granularities:

| Tier | Key | Match type | Example key |
|---|---|---|---|
| 1 | `GeneralizedText()` | Generalized exact | `{PERSON} works at {ORGANIZATION}` |
| 2 | `StructuralText()` | Structural exact | `{1}Click{/1} {2}here{/2}` |
| 3 | `Text()` | Plain exact | `Click here` |
| 4–6 | Same keys | Fuzzy (Levenshtein) | Scored matches |

Sequential numeric IDs ensure structural keys are stable across formats:
HTML `<b>Click</b>` and Markdown `**Click**` both produce `{1}Click{/1}` —
a TM entry created from HTML matches a Markdown source at the structural
tier.

### Editor display

The translation editor ([AD-020](./020-collaborative-editor.md)) uses
`SemanticHTML()` for source/target preview rendering and `DisplayText` for
inline code chips in the editing surface. The editor shows codes as inline
badges (`[B]`, `[/B]`, `[IMG]`) that translators can drag to reorder but
cannot edit. The `Deletable` and `CanReorder` flags control which operations
the editor permits.

### JSON serialization

Fragments serialize to JSON for storage, APIs, and the Content Store:

```json
{
  "coded_text": "Click \uE001here\uE002 for info",
  "spans": [
    {
      "span_type": "opening",
      "type": "fmt:bold",
      "sub_type": "html:b",
      "id": "1",
      "data": "<b class=\"emphasis\">",
      "display_text": "[B]",
      "equiv_text": "",
      "deletable": true,
      "cloneable": true,
      "can_reorder": true
    },
    {
      "span_type": "closing",
      "type": "fmt:bold",
      "sub_type": "html:b",
      "id": "1",
      "data": "</b>",
      "display_text": "[/B]",
      "equiv_text": "",
      "deletable": true,
      "cloneable": true,
      "can_reorder": true
    }
  ]
}
```

All six layers round-trip through JSON. The `SubType` field preserves
format-specific provenance, enabling downstream tools to distinguish
`html:b` from `html:strong` when needed.

## Alternatives Considered

- **Code simplification step** (Okapi pattern): a separate processing step
  that collapses redundant codes (e.g., adjacent `</b><b>` pairs). This
  introduces a bug-prone intermediate representation and is the source of
  many inline code issues in Okapi. Rejected: the semantic type abstraction
  eliminates the need for simplification — tools process at the semantic
  level, writers replay native markup.

- **Single type field with free strings** (no vocabulary): format readers
  would use whatever string they want (`"b"`, `"bold"`, `"strong"`,
  `"emphasis"`). This prevents cross-format TM matching and makes AI
  prompting inconsistent. Rejected in favor of a defined vocabulary with
  SubType for format-specific refinement.

- **Embed format-specific data in Span struct fields**: add HTML-specific
  or DOCX-specific fields to Span. Rejected: the `Data` field plus `SubType`
  provide format-specific storage without coupling the model to any format.

- **Strip inline codes for AI/MT, reinsert by alignment**: common in older
  systems. Loses context (the LLM doesn't know what's bold), and reinsertion
  by word alignment is error-prone with reordering languages (EN→JA, EN→AR).
  Rejected: sending semantic HTML or placeholder tags gives the AI format
  awareness while preserving structure.

- **XLIFF as canonical intermediate representation**: force all content
  through XLIFF serialization before processing. Rejected: XLIFF is an
  optional interchange format, not a mandatory intermediate. The
  Fragment/Span model is the canonical representation; XLIFF is one of many
  serialization options.

## Consequences

- Format readers are responsible for semantic type classification at
  extraction time. The vocabulary is small and well-defined, making
  classification straightforward for most formats.
- TM matching across formats works because structural and generalized keys
  use semantic types, not native markup strings.
- AI translation preserves inline codes because tools send structured
  representations (semantic HTML or placeholders) rather than stripping
  codes. Response parsing reconstructs the full Span metadata.
- Writers are decoupled from semantic processing — they replay `Span.Data`
  verbatim, ensuring roundtrip fidelity without needing to understand the
  semantic type vocabulary.
- New formats only need to map their native constructs to the existing
  semantic types. New semantic types can be added to the vocabulary when a
  format introduces a genuinely new concept.
- The six-layer model matches XLIFF 2.0's design, making XLIFF
  serialization a natural mapping rather than a lossy conversion.
- Display text and editing constraints drive the translation editor UI
  without coupling the content model to any specific frontend implementation.
