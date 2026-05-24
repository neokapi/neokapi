---
sidebar_position: 6
title: Authoring Vocabularies
description: How to implement and extend neokapi vocabularies — the JSON type-definition format, category:name identifiers, rendering and display metadata, and registering a vocabulary with the framework.
keywords: [vocabularies, authoring, inline codes, semantic types, JSON, vocabulary file, neokapi]
---

# Authoring Vocabularies

This guide covers implementing and extending vocabularies — the semantic type
system that classifies inline codes. For what vocabularies are and why they
exist, see the concept page: [Vocabularies](/framework/vocabularies).

## Vocabulary file format

Each vocabulary is a JSON file. Types are keyed by a `category:name` identifier
and carry rendering, display, color, and constraint metadata:

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

### Field reference

| Field           | Required | Description                                        |
| --------------- | -------- | -------------------------------------------------- |
| `name`          | Yes      | Unique vocabulary name                             |
| `version`       | Yes      | Semver version string                              |
| `extends`       | No       | Parent vocabulary name (types are merged)          |
| `entity_prefix` | No       | Prefix for entity-type spans (default `"entity:"`) |
| `types`         | Yes      | Map of type name → `SpanTypeInfo`                  |
| `fallback`      | No       | Default rendering for unknown types                |

### Type name convention

Type names follow the `category:name` pattern: `fmt:bold`, `link:hyperlink`,
`code:variable`, `struct:break`.

### Constraint semantics

| Constraint    | `true`                                | `false`                                   |
| ------------- | ------------------------------------- | ----------------------------------------- |
| `deletable`   | Translator may remove the tag         | Tag must appear in translation (enforced) |
| `cloneable`   | Translator may duplicate the tag      | Tag count must not exceed source count    |
| `reorderable` | Translator may rearrange tag position | Tag position relative to others is locked |

## Using vocabularies in a format reader

A format reader initializes a `VocabularyRegistry` and uses it to populate span
metadata as it builds fragments:

```go
package myformat

import "github.com/neokapi/neokapi/core/model"

type Reader struct {
    vocab *model.VocabularyRegistry
}

func NewReader() *Reader {
    vocab := model.NewVocabularyRegistry()
    _ = vocab.LoadDefaults() // common-formatting + rich-html + code-tokens
    return &Reader{vocab: vocab}
}
```

When creating spans, look up the vocabulary entry and populate the rendering and
constraint fields:

```go
func (r *Reader) createSpan(semType, subType, id, nativeMarkup string, st model.SpanType) *model.Span {
    info := r.vocab.LookupOrFallback(semType)

    var displayText string
    switch st {
    case model.SpanOpening:
        displayText = info.Display.Open
    case model.SpanClosing:
        displayText = info.Display.Close
    case model.SpanPlaceholder:
        displayText = info.Display.Placeholder
    }

    return &model.Span{
        SpanType:    st,
        Type:        semType,       // "fmt:bold"
        SubType:     subType,       // "html:b" or "md:strong"
        ID:          id,
        Data:        nativeMarkup,  // original markup for roundtrip
        DisplayText: displayText,   // "[B]"
        EquivText:   info.Equiv,    // "" (or "\n" for struct:break)
        Deletable:   info.Constraints.Deletable,
        Cloneable:   info.Constraints.Cloneable,
        CanReorder:  info.Constraints.Reorderable,
    }
}
```

### Mapping native elements to semantic types

Each format maps its native constructs to semantic types. The HTML and Markdown
readers map differently but resolve to the same types:

```go
var htmlSemanticTypes = map[string]string{
    "b": "fmt:bold", "strong": "fmt:bold",
    "i": "fmt:italic", "em": "fmt:italic",
    "u": "fmt:underline", "s": "fmt:strikethrough",
    "a": "link:hyperlink", "code": "fmt:code",
    "br": "struct:break", "img": "media:image",
    "sub": "fmt:subscript", "sup": "fmt:superscript", "mark": "fmt:highlight",
}

var markdownSemanticTypes = map[string]string{
    "strong": "fmt:bold", "emphasis": "fmt:italic",
    "code": "fmt:code", "link": "link:hyperlink",
    "image": "media:image", "softbreak": "struct:break",
}
```

### SubType conventions

The `SubType` field records format-specific provenance using a prefix
convention: `html:` (`html:b`, `html:span`), `md:` (`md:strong`), `xlf:`
(`xlf:var`), `docx:` (`docx:w:b`). Custom formats should use a reverse-domain
prefix: `com.acme:custom-tag`.

## Creating a custom vocabulary

### 1. Create the JSON file

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
      "constraints": { "deletable": false, "cloneable": false, "reorderable": true }
    }
  }
}
```

### 2. Load it into the registry

`LoadDefaults()` loads the embedded vocabularies. To add one at runtime:

```go
vocab := model.NewVocabularyRegistry()
vocab.LoadDefaults()

customData, _ := os.ReadFile("my-domain.json")
vocab.Load(customData)
```

### 3. Map it in your reader

Add the new type to your format reader's semantic type mapping:

```go
var myFormatSemanticTypes = map[string]string{
    "widget": "domain:widget",
}
```

## SpanClassify tool

For formats that do not perform full semantic classification (for example, when
content arrives via the Okapi bridge), the `span-classify` tool reclassifies
generic `code:markup` spans into proper semantic types:

```go
tool := tools.NewSpanClassifyTool(&tools.SpanClassifyConfig{})
```

It applies strategies in order: check `SubType` against known Okapi type strings,
parse `Data` for an HTML element name, look that name up in the semantic type
map, and otherwise leave the span as `code:markup`.

## Testing vocabularies

```go
func TestMyVocabulary(t *testing.T) {
    vocab := model.NewVocabularyRegistry()
    require.NoError(t, vocab.LoadDefaults())

    info := vocab.Lookup("fmt:bold")
    require.NotNil(t, info)
    assert.Equal(t, "formatting", info.Category)
    assert.True(t, info.Constraints.Deletable)

    unknown := vocab.LookupOrFallback("custom:unknown")
    assert.Equal(t, "generic", unknown.Category)
}
```

## Best practices

1. **Use existing types when possible.** Map to `fmt:bold` rather than creating `my-format:bold`.
2. **Set constraints conservatively.** Mark code tokens non-deletable; formatting fully flexible.
3. **Keep vocabularies small.** Only add types with distinct rendering or constraint needs.
4. **Test roundtrip fidelity.** Vocabulary types affect rendering, but `Span.Data` drives output — verify both.
5. **Extend rather than replace.** Use `extends` to build on `common-formatting`.

## Related reading

- [Vocabularies](/framework/vocabularies) — the concept and built-in vocabularies.
- [Implementing a Format](/contribute/formats) — building readers and writers.
- [Inline Formatting](/framework/inline-formatting) — the span model in the content model.
