---
sidebar_position: 17
title: Content-Fidelity Surfacing
description: Implementation note for AD-031 — how a format author surfaces non-translatable contextual content (alt-text, code, formulas, comments, metadata) for ingestion while keeping the byte-exact round-trip and Okapi parity, via the inverted disableNonTranslatableContent toggle, the getter/setter/ApplyMap/schema trio, the duck-typed parity force-off, and the two surfacing channels (non-translatable Block vs Data/NoteAnnotation).
keywords: [content fidelity, surfacing, non-translatable content, ExtractNonTranslatableContent, SemanticRole, skeleton, parity, format reader, neokapi]
---

# Content-Fidelity Surfacing

Tactical guide for a format author adding *content-fidelity surfacing* — making
previously skeleton-only context (image and shape alt-text, code blocks,
captions, formulas, do-not-translate strings, config-excluded values, comments,
document metadata) visible to ingestion and LLM/RAG consumers while machine
translation continues to skip it and the round-trip stays byte-exact. Parent AD:
[AD-031: Content-Fidelity Surfacing](/contribute/architecture/031-content-fidelity-surfacing).
For the surrounding format-system contracts see
[AD-005: Format System](/contribute/architecture/005-format-system), the general
port recipe in
[Implementing Formats](/contribute/notes-internal/implementing-formats), and the
skeleton mechanics in
[Skeleton Store and Streaming HTML](/contribute/notes-internal/skeleton-store).

Surfacing is a richer *default*, not a new structural type. The roles it leans
on (`RoleCode`, `RoleFormula`, `RoleCaption`, … in `core/model/structure.go`)
already exist; what a format adds is a reader that *uses* them to emit content
that previously lived only in the skeleton.

## The surfacing toggle: an inverted private field

A surfacing-capable config carries one boolean whose **zero value means
surfacing is ON**. The field is private and inverted so the rich default falls
out of `Config{}` without a `Reset()` assignment — the `nonFoo` convention
documented in [Implementing Formats](/contribute/notes-internal/implementing-formats)
("`bool` defaults to `false`, so use `nonFoo` naming when you want the default
behavior to be `foo`").

```go
// core/formats/openxml/config.go
type Config struct {
    // … common toggles …

    // disableNonTranslatableContent, when set, keeps non-translatable
    // contextual content in opaque skeleton / verbatim parts instead of
    // surfacing it. Zero value = surfacing ON (the opt-out default).
    disableNonTranslatableContent bool
}
```

Naming the field `disableNonTranslatableContent` (rather than a positive
`extractNonTranslatableContent bool` that `Reset()` must remember to set `true`)
keeps the on-by-default behavior true even for a `Config{}` literal that never
runs `Reset()` — for example a reader constructed in a test, or any caller that
mutates one field and forgets the rest. `Reset()` still pins it explicitly for
documentation value:

```go
func (c *Config) Reset() {
    // … other defaults …
    c.disableNonTranslatableContent = false // surfacing ON
}
```

## The getter / setter / ApplyMap / schema quartet

The private field is exposed through four coordinated surfaces. Keep the names
and the public key (`extractNonTranslatableContent`, positive sense)
byte-identical across all of them — the duck-typed parity hook (below) and the
generated reference both bind to them by name.

**Getter and setter** (`core/formats/openxml/config.go`) present the positive
sense and absorb the inversion, so no caller ever sees `disable…`:

```go
func (c *Config) ExtractNonTranslatableContent() bool {
    return !c.disableNonTranslatableContent
}

func (c *Config) SetExtractNonTranslatableContent(v bool) {
    c.disableNonTranslatableContent = !v
}
```

**ApplyMap key** (same file) decodes the positive public key — recipes, presets,
and the CLI speak `extractNonTranslatableContent`, never the private field:

```go
case "extractNonTranslatableContent":
    c.disableNonTranslatableContent = !toBool(val)
```

**`schema.Prop`** (`core/formats/openxml/schema.go`) advertises the toggle to the
generated `/reference/formats` page and to schema-driven config UIs, with
`Default: true` to match the zero value's behavior. The field is also listed in a
`ParameterGroup` (the `"general"` group) so it renders:

```go
"extractNonTranslatableContent": schema.Prop(coreschema.PropertySchema{
    Type:    "boolean",
    Title:   "Extract non-translatable content",
    Default: true,
    Description: "If true (default), non-translatable contextual content is " +
        "surfaced — image/shape alt-text (descr/title on docPr/cNvPr) as " +
        "content blocks (visible to ingestion/LLM consumers, skipped by " +
        "machine translation), and PowerPoint/Excel comment text as data " +
        "parts — instead of being hidden in opaque skeleton. Disable to keep " +
        "it opaque.",
}),
```

## The duck-typed extension point

There is no `format.DataFormatConfig` method for surfacing — it is an optional
capability discovered structurally. Any consumer that needs to force the toggle
type-asserts the anonymous interface, so a config that lacks the setter is simply
left at its own default:

```go
// cli/parity/spec/runner.go (runNative)
if c := reader.Config(); c != nil {
    if d, ok := c.(interface{ SetExtractNonTranslatableContent(bool) }); ok {
        d.SetExtractNonTranslatableContent(false)
    }
}
```

The parity runner uses this to force surfacing **off** for the head-to-head: the
okapi-bridge has no notion of surfacing, so the native reader must reproduce the
bridge's opaque-skeleton output when handed the matching semantic config. This
is the concrete realization of the parity contract — *same semantic config →
same results* — while letting native readers pick the richer default. Exposing
exactly `SetExtractNonTranslatableContent(bool)` (no more, no less) is therefore
the contract a new surfacing format must honor to participate in parity (see
[AD-018: Parity Testing](/contribute/architecture/018-parity-testing)).

## Channel 1 — renderable content as a non-translatable Block

Content that is *rendered* in the document but must not be translated (alt-text,
code, captions, formulas, do-not-translate strings, config-excluded values) is
surfaced as a `model.Block` with `Translatable: false`, tagged with a
`SemanticRole`, and referenced from the skeleton so it round-trips by ID. Because
the block holds the value, an untranslated read replays the original bytes and a
translated one splices the new text in place — the structure is never disturbed.

```go
// core/formats/openxml/dml.go (emitDrawingProp) — image/shape alt-text
block := &model.Block{
    ID:           id,
    Type:         "property",
    Translatable: false,
    Source:       []model.Run{{Text: &model.TextRun{Text: a.Value}}},
    Targets:      make(map[model.VariantKey]*model.Target),
    Properties:   map[string]string{"partPath": partPath, "element": element},
}
block.SetSemanticRole(model.RoleCaption, 0) // func (b *Block) SetSemanticRole(role string, level int)
emitBlock(block)
return id // caller writes a skeleton Ref to this id
```

The byte-exact guarantee comes from the skeleton ref, not from the block. The
writer emits skeleton text for the literal surrounding markup and a `SkeletonRef`
where the value belongs:

```go
// core/model/skeleton.go
type SkeletonRef struct {
    ResourceID string // the surfaced block's ID
    Property   string // which property to reference, e.g. "target" / "source"
    Locale     string // target locale for locale-specific references
}
```

Pick the closest existing role from `core/model/structure.go` —
`RoleCaption` for alt-text/object titles, `RoleCode` for code, `RoleFormula` for
equations, and so on. Do not invent a role for a surfacing case; the role is the
stable handle that semantic export, the editor, and ingestion use to recognize
the content without treating it as MT input. MT skips it because `Translatable`
is `false` (AD-012); RAG sees it because it is now a part in the stream rather
than buried in the skeleton.

Equations are the richer instance of this channel: an *inline* formula surfaces
as a placeholder run carrying its portable rendering, a *standalone* equation as
a detached `RoleFormula` block, and the natural-language prose embedded inside an
equation is written back through a **sub-skeleton**. See
[OMML Math Conversion](/contribute/notes-internal/omml-math) for the
formula-specific surfacing and its byte-exact splice.

## Channel 2 — comments and metadata as Data or a note

Context that is *about* the content rather than rendered alongside it — authoring
comments, reviewer notes, developer metadata — surfaces through the second
channel and never becomes a translatable surface:

- **`model.Data` (`PartData`)** for free-standing informational context whose
  source part is already captured verbatim in the skeleton, so the Data part is
  purely additive and cannot perturb the round-trip. OpenXML uses this for PPTX
  comment bodies (`<p:text>`) via `emitPPTXCommentData` in
  `core/formats/openxml/dml.go` — the comment part is parsed verbatim for
  skeleton, and the Data part is emitted on the side — and, per the config
  documentation, for XLSX comment text (`<comment><text>`) the same way.
- **`NoteAnnotation`** when the context is *anchored to a specific block* —
  attach it with `Block.AddNote` so it travels with that block (this is the
  natural home for PO-style translator/extracted comments):

```go
// core/model/annotation.go
type NoteAnnotation struct {
    Text      string `json:"text"`                // note text content
    From      string `json:"from,omitempty"`      // who wrote it ("developer", "translator", …)
    Priority  int    `json:"priority,omitempty"`  // priority level (1 = highest)
    Annotates string `json:"annotates,omitempty"` // "source", "target", "general"
}

// core/model/annotation_access.go
func (b *Block) AddNote(n *NoteAnnotation)
```

Choose Data for document-level or free-standing context, and a `NoteAnnotation`
when the context belongs to one block.

## Composing formats: opt the inner reader out

A format that *composes* another reader inherits that reader's surfacing default.
When the composed reader's notion of "non-translatable content" is wrong for the
outer format, the outer config must turn the inner surfacing off explicitly.

Design Tokens is the canonical case: it drives the generic JSON reader, but a
token's `$value`/`$type`/`$extensions` are structured machine data, not prose,
so surfacing them as content blocks would be noise. The only translatable surface
is `$description`. `applyToJSON` therefore disables the inner reader's surfacing
through the same setter the parity runner uses:

```go
// core/formats/designtokens/config.go (applyToJSON)
func (c *Config) applyToJSON(jc *jsonfmt.Config) {
    jc.Reset()
    // Design-token values are structured machine data (colours, dimensions,
    // numbers), not contextual prose — do not surface the excluded values as
    // non-translatable content blocks. (The JSON reader's default is to surface
    // them for ingestion.)
    jc.SetExtractNonTranslatableContent(false)
    // … ExtractAllPairs=false + $description-only extraction rule …
}
```

The outer format may still offer its own positive toggle to the user
(`ExtractDescriptions` here) without re-exposing the inner reader's toggle: the
two concerns are decoupled, and the embedded reader stays non-surfacing
regardless.

## Parity safety of the carriers

The canonical parity projection (`cli/parity/normalize.go`, `CanonicalPart` /
`Canonicalize`) compares only part type, identity, `Translatable`, and rendered
source/target text. It *omits* `SemanticRole`/structure annotations,
`Properties`, block annotations (including notes), and the placeholder
`Equiv`/`Disp` carriers — so attaching any of those to an existing part is
invisible to parity and needs no special handling. The thing that is **not** free
is emitting a *new* part: a surfaced `Translatable:false` Block adds a `BlockID`
row, and a comment `Data` part adds a `DataID` row, to the canonical stream and
will diverge from the bridge. That divergence is exactly what the duck-typed
force-off neutralizes — with surfacing off, the extra parts are not emitted and
the stream matches byte-for-byte. This is why a surfacing format *must* wire the
setter rather than emitting unconditionally.

## Checklist for a new surfacing format

1. Add a private inverted field `disableNonTranslatableContent bool`; leave its
   zero value (surfacing ON) and pin it in `Reset()` for documentation.
2. Add the `ExtractNonTranslatableContent() bool` getter and
   `SetExtractNonTranslatableContent(bool)` setter (both absorbing the
   inversion) — the exact setter signature is the parity contract.
3. Decode the positive public key `extractNonTranslatableContent` in
   `ApplyMap`.
4. Declare a `schema.Prop` with `Default: true` and place the field in a
   `ParameterGroup` so it reaches `/reference/formats`.
5. Surface rendered context via channel 1: `Block{Translatable:false}` +
   `SetSemanticRole(<closest existing role>, level)` + a `SkeletonRef` so the
   round-trip is byte-exact; surface comments/metadata via channel 2
   (`model.Data` or `Block.AddNote(&NoteAnnotation{…})`).
6. If the format composes another reader, call
   `SetExtractNonTranslatableContent(false)` on the inner config when its
   surfacing is wrong for your content (cf. Design Tokens → JSON).
7. Add a `*noncontent*` test asserting (a) surfaced parts appear by default and
   (b) `SetExtractNonTranslatableContent(false)` yields the opaque,
   bridge-identical stream.

## Related

- [AD-031: Content-Fidelity Surfacing](/contribute/architecture/031-content-fidelity-surfacing) — the parent decision and rationale.
- [AD-005: Format System](/contribute/architecture/005-format-system) — reader/writer contracts and skeleton strategies.
- [AD-002: Content Model](/contribute/architecture/002-content-model) — `Block`, `Translatable`, and the semantic-role taxonomy this note leans on.
- [AD-018: Parity Testing](/contribute/architecture/018-parity-testing) — the *same semantic config → same results* contract the duck-typed force-off honors.
- [Implementing Formats](/contribute/notes-internal/implementing-formats) — the `nonFoo` default convention and the general port recipe.
- [Skeleton Store and Streaming HTML](/contribute/notes-internal/skeleton-store) — how skeleton refs reconstruct documents byte-exactly.
- [OMML Math Conversion](/contribute/notes-internal/omml-math) — formula surfacing (`RoleFormula` blocks, placeholder renderings) and the sub-skeleton write-back for equation prose.
