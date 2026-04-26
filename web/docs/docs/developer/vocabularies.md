---
sidebar_position: 10
title: Vocabularies
---

# Vocabularies

Vocabularies are the semantic type system that drives format-independent inline code handling across neokapi. They define how each inline code type (bold, links, variables, etc.) is rendered, validated, and constrained in the translation editor — regardless of the source file format.

## Overview

When neokapi parses a document, each inline code element (like `<b>`, `**`, or `{userName}`) is mapped to a **semantic type** from a vocabulary. This semantic type carries:

| Layer               | What it provides    | Example                             |
| ------------------- | ------------------- | ----------------------------------- |
| **Category**        | Logical grouping    | `"formatting"`, `"code"`            |
| **Label**           | Human-readable name | `"Bold"`, `"Variable"`              |
| **HTML rendering**  | Preview output      | `<b>`, `</b>`                       |
| **Display text**    | Editor chip label   | `[B]`, `[/B]`                       |
| **Color scheme**    | Visual styling      | Blue for bold, orange for variables |
| **Constraints**     | Editing rules       | Deletable, cloneable, reorderable   |
| **Text equivalent** | Plain text fallback | `"\n"` for line breaks              |

Because `<b>` (HTML), `**` (Markdown), and `<w:b/>` (DOCX) all map to `fmt:bold`, translators see the same visual experience regardless of file format.

## Built-in Vocabularies

neokapi ships three vocabulary files. They form a layered system where each vocabulary can extend another:

### common-formatting (base)

The foundational vocabulary with types used across all formats:

| Type             | Category   | Label      | Constraints                                       |
| ---------------- | ---------- | ---------- | ------------------------------------------------- |
| `fmt:bold`       | formatting | Bold       | Deletable, cloneable, reorderable                 |
| `fmt:italic`     | formatting | Italic     | Deletable, cloneable, reorderable                 |
| `fmt:underline`  | formatting | Underline  | Deletable, cloneable, reorderable                 |
| `fmt:code`       | formatting | Code       | Deletable, cloneable, reorderable                 |
| `link:hyperlink` | linking    | Hyperlink  | Deletable, cloneable, reorderable                 |
| `media:image`    | media      | Image      | Deletable, cloneable, reorderable                 |
| `struct:break`   | structure  | Line Break | **Non-deletable, non-cloneable, non-reorderable** |

### rich-html (extends common-formatting)

Additional types for HTML-rich content:

| Type                | Category   | Label           | Constraints                                       |
| ------------------- | ---------- | --------------- | ------------------------------------------------- |
| `fmt:strikethrough` | formatting | Strikethrough   | Deletable, cloneable, reorderable                 |
| `fmt:subscript`     | formatting | Subscript       | Deletable, cloneable, reorderable                 |
| `fmt:superscript`   | formatting | Superscript     | Deletable, cloneable, reorderable                 |
| `fmt:highlight`     | formatting | Highlight       | Deletable, cloneable, reorderable                 |
| `struct:ruby`       | structure  | Ruby Annotation | **Non-deletable, non-cloneable, non-reorderable** |
| `struct:footnote`   | structure  | Footnote        | **Non-deletable, non-cloneable**                  |

### code-tokens (extends common-formatting)

Types for code elements and i18n placeholders:

| Type               | Category | Label       | Constraints                                       |
| ------------------ | -------- | ----------- | ------------------------------------------------- |
| `code:variable`    | code     | Variable    | **Non-deletable, non-cloneable**, reorderable     |
| `code:placeholder` | code     | Placeholder | **Non-deletable, non-cloneable**, reorderable     |
| `code:function`    | code     | Function    | **Non-deletable, non-cloneable, non-reorderable** |
| `code:markup`      | code     | Markup      | Deletable, cloneable, reorderable                 |

## Vocabulary File Format

Each vocabulary is a JSON file with the following schema:

```json
{
  "name": "my-vocabulary",
  "version": "1.0",
  "extends": "common-formatting",
  "entity_prefix": "entity:",
  "types": {
    "category:type-name": {
      "category": "category-name",
      "label": "Human Readable Label",
      "html": {
        "open": "<tag>",
        "close": "</tag>",
        "placeholder": "<tag/>"
      },
      "display": {
        "open": "[TAG]",
        "close": "[/TAG]",
        "placeholder": "[TAG/]"
      },
      "chipLabel": {
        "open": "tag>",
        "close": "/tag",
        "placeholder": "tag"
      },
      "color": {
        "bg": "rgba(59,130,246,0.15)",
        "border": "rgba(59,130,246,0.5)",
        "text": "rgb(59,130,246)"
      },
      "equiv": "",
      "constraints": {
        "deletable": true,
        "cloneable": true,
        "reorderable": true
      }
    }
  },
  "fallback": {
    "html": { "open": "<span>", "close": "</span>", "placeholder": "<span/>" },
    "display": { "open": "[?]", "close": "[/?]", "placeholder": "[?/]" },
    "chipLabel": { "open": "?>", "close": "/?", "placeholder": "?" },
    "color": {
      "bg": "rgba(156,163,175,0.15)",
      "border": "rgba(156,163,175,0.5)",
      "text": "rgb(107,114,128)"
    },
    "constraints": { "deletable": true, "cloneable": true, "reorderable": true }
  }
}
```

### Field Reference

| Field           | Required | Description                                        |
| --------------- | -------- | -------------------------------------------------- |
| `name`          | Yes      | Unique vocabulary name                             |
| `version`       | Yes      | Semver version string                              |
| `extends`       | No       | Parent vocabulary name (types are merged)          |
| `entity_prefix` | No       | Prefix for entity-type spans (default `"entity:"`) |
| `types`         | Yes      | Map of type name → `SpanTypeInfo`                  |
| `fallback`      | No       | Default rendering for unknown types                |

### Type Name Convention

Type names follow the `category:name` pattern:

- `fmt:bold` — formatting category, bold type
- `link:hyperlink` — linking category, hyperlink type
- `code:variable` — code category, variable type
- `struct:break` — structure category, break type

### Constraint Semantics

| Constraint    | `true`                                | `false`                                   |
| ------------- | ------------------------------------- | ----------------------------------------- |
| `deletable`   | Translator may remove the tag         | Tag must appear in translation (enforced) |
| `cloneable`   | Translator may duplicate the tag      | Tag count must not exceed source count    |
| `reorderable` | Translator may rearrange tag position | Tag position relative to others is locked |

## Using Vocabularies in a Format Reader

Every format reader initializes a `VocabularyRegistry` and uses it to populate span metadata:

```go
package myformat

import (
    "github.com/neokapi/neokapi/core/model"
)

type Reader struct {
    vocab *model.VocabularyRegistry
}

func NewReader() *Reader {
    vocab := model.NewVocabularyRegistry()
    _ = vocab.LoadDefaults()  // Loads common-formatting + rich-html + code-tokens
    return &Reader{vocab: vocab}
}
```

When creating spans, look up the vocabulary entry and populate all six layers:

```go
func (r *Reader) createSpan(semType, subType, id, nativeMarkup string, st model.SpanType) *model.Span {
    info := r.vocab.LookupOrFallback(semType)

    var displayText, equivText string
    switch st {
    case model.SpanOpening:
        displayText = info.Display.Open
    case model.SpanClosing:
        displayText = info.Display.Close
    case model.SpanPlaceholder:
        displayText = info.Display.Placeholder
    }
    equivText = info.Equiv

    return &model.Span{
        SpanType:    st,
        Type:        semType,       // "fmt:bold"
        SubType:     subType,       // "html:b" or "md:strong"
        ID:          id,
        Data:        nativeMarkup,  // "<b class='emphasis'>" — original markup for roundtrip
        DisplayText: displayText,   // "[B]"
        EquivText:   equivText,     // "" (or "\n" for struct:break)
        Deletable:   info.Constraints.Deletable,
        Cloneable:   info.Constraints.Cloneable,
        CanReorder:  info.Constraints.Reorderable,
    }
}
```

### Mapping Native Elements to Semantic Types

Each format defines a mapping from native constructs to semantic types. For example, the HTML reader:

```go
var htmlSemanticTypes = map[string]string{
    "b": "fmt:bold",      "strong": "fmt:bold",
    "i": "fmt:italic",    "em":     "fmt:italic",
    "u": "fmt:underline",
    "s": "fmt:strikethrough",
    "a": "link:hyperlink",
    "code": "fmt:code",
    "br": "struct:break",
    "img": "media:image",
    "sub": "fmt:subscript", "sup": "fmt:superscript",
    "mark": "fmt:highlight",
}
```

The Markdown reader maps differently but resolves to the same types:

```go
var markdownSemanticTypes = map[string]string{
    "strong":   "fmt:bold",
    "emphasis": "fmt:italic",
    "code":     "fmt:code",
    "link":     "link:hyperlink",
    "image":    "media:image",
    "softbreak": "struct:break",
}
```

### SubType Conventions

The `SubType` field provides format-specific provenance using a prefix convention:

| Prefix  | Format    | Examples                         |
| ------- | --------- | -------------------------------- |
| `html:` | HTML      | `html:b`, `html:em`, `html:span` |
| `md:`   | Markdown  | `md:strong`, `md:emphasis`       |
| `xlf:`  | XLIFF 2.0 | `xlf:b`, `xlf:i`, `xlf:var`      |
| `docx:` | DOCX      | `docx:w:b`, `docx:w:i`           |

Custom formats should use reverse-domain prefix: `com.acme:custom-tag`.

## Creating a Custom Vocabulary

To add a new vocabulary for a specialized format or domain:

### 1. Create the JSON File

Create a JSON file under `core/model/vocabularies/`:

```json
{
  "name": "my-domain",
  "version": "1.0",
  "extends": "common-formatting",
  "types": {
    "domain:widget": {
      "category": "domain",
      "label": "Widget",
      "html": { "placeholder": "<span class=\"widget\"/>" },
      "display": { "placeholder": "[WIDGET]" },
      "chipLabel": { "placeholder": "wgt" },
      "color": {
        "bg": "rgba(168,85,247,0.15)",
        "border": "rgba(168,85,247,0.5)",
        "text": "rgb(168,85,247)"
      },
      "equiv": "",
      "constraints": {
        "deletable": false,
        "cloneable": false,
        "reorderable": true
      }
    }
  }
}
```

### 2. Register in the Go Vocabulary Registry

In `core/model/vocabulary.go`, the `LoadDefaults()` method loads embedded vocabularies. For custom vocabularies loaded at runtime, use:

```go
vocab := model.NewVocabularyRegistry()
vocab.LoadDefaults()

// Load additional vocabulary
customData, _ := os.ReadFile("my-domain.json")
vocab.Load(customData)
```

### 3. Map in Your Format Reader

Add the new type to your format reader's semantic type mapping:

```go
var myFormatSemanticTypes = map[string]string{
    "widget": "domain:widget",
    // ... other mappings
}
```

## Vocabulary-Driven Editing

The vocabulary system provides the metadata that editors need to present inline codes to translators:

- **Tag chip rendering**: Each vocabulary type defines chip labels and colors (e.g., `B>` for bold opening, `/B` for bold closing). Colors are category-specific (blue for formatting, orange for code, etc.).
- **Constraint enforcement**: Editors can use the `Deletable`, `Cloneable`, and `Reorderable` constraint fields to prevent translators from making invalid changes — for example, blocking deletion of required tags or preventing duplication of non-cloneable tags.
- **Inline code legend**: Vocabulary categories and constraint metadata allow editors to present a grouped reference of all tag types in a segment, with indicators for which tags are required, duplicatable, or position-locked.

These fields are part of the framework's content model. How they are rendered is up to the consuming editor or application.

## SpanClassify Tool

For formats that don't perform full semantic classification (e.g., when using the Okapi bridge), the `SpanClassifyTool` reclassifies `code:markup` spans into proper semantic types:

```go
tool := tools.NewSpanClassifyTool(&tools.SpanClassifyConfig{})
// Processes blocks and reclassifies code:markup spans
// based on SubType strings and HTML element parsing
```

The tool applies classification strategies in order:

1. Check `SubType` against known Okapi type strings
2. Parse `Data` to extract HTML element name
3. Look up element name in semantic type map
4. Leave as `code:markup` if no match found

## Vocabulary and TM Matching

Vocabularies enable format-independent TM matching through text projections:

| Projection      | Method                       | Use Case                                                    |
| --------------- | ---------------------------- | ----------------------------------------------------------- |
| **Generalized** | `Fragment.GeneralizedText()` | Maximum reuse — entities become typed placeholders          |
| **Structural**  | `Fragment.StructuralText()`  | Format-agnostic — inline codes become numbered placeholders |
| **Plain**       | `Fragment.Text()`            | Exact text matching — markers stripped                      |

Because HTML `<b>Click</b>` and Markdown `**Click**` both produce `{1}Click{/1}` at the structural level, TM entries created from one format match sources in another format.

## Testing Vocabularies

### Go-Side Tests

```go
func TestMyVocabulary(t *testing.T) {
    vocab := model.NewVocabularyRegistry()
    require.NoError(t, vocab.LoadDefaults())

    // Verify type exists
    info := vocab.Lookup("fmt:bold")
    require.NotNil(t, info)
    assert.Equal(t, "formatting", info.Category)
    assert.Equal(t, "Bold", info.Label)

    // Verify constraints
    assert.True(t, info.Constraints.Deletable)
    assert.True(t, info.Constraints.Cloneable)

    // Verify fallback for unknown types
    unknown := vocab.LookupOrFallback("custom:unknown")
    assert.Equal(t, "generic", unknown.Category)
}
```

## Best Practices

1. **Use existing types when possible.** Map to `fmt:bold` rather than creating `my-format:bold`.
2. **Set constraints conservatively.** Mark code tokens as non-deletable; formatting as fully flexible.
3. **Keep vocabularies small.** Only add types that have distinct rendering or constraint needs.
4. **Test roundtrip fidelity.** Vocabulary types affect rendering but `Span.Data` drives output — verify both.
5. **Extend rather than replace.** Use the `extends` field to build on `common-formatting`.
