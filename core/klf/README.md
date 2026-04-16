# core/klf

Go implementation of the canonical Block / Run model specified in
[AD-045](../../docs/ad/045-klf-klz-spec.md). Paired with
[`packages/format/`](../../packages/format/) (`@neokapi/format`) —
the TypeScript port — via shared golden fixtures so both languages
render the same bytes.

## Package entry points

### Types (`schema.go`)

- `Block` — unit of translation tracking, carries `Source []Run`,
  optional per-locale `Targets map[LocaleID][]Run`, and metadata.
- `Run` — discriminated union: `Text`, `Ph`, `PcOpen`, `PcClose`,
  `Sub`, `Plural`, `Select`. Strict marshal/unmarshal rejects
  zero- or multi-discriminator records.
- `Placeholder` — named placeholder metadata (`jsType`, `sourceExpr`,
  `optional`, `icu-pivot` flag).
- `PluralRun` / `SelectRun` — structured constructs. Plural forms
  are keyed by `PluralForm` (`zero`/`one`/`two`/`few`/`many`/`other`).
- `ExtractedDocument` (`Document`) — top-level wrapper with
  `schemaVersion`, `sourceLocale`, `file`, and `blocks[]`.
- `File` — top-level `.klf` JSON envelope.

### JSON reader/writer

```go
file, err := klf.Unmarshal(data)        // parse .klf JSON
data, err := klf.Marshal(file)          // deterministic .klf JSON
```

The writer is deterministic (2-space indent, no HTML escaping,
trailing newline) — the raw bytes feed into the .klz manifest hash
that `core/klz` content-addresses cache entries by.

### Validator (`validator.go`)

```go
errs := klf.ValidateBlock(block)                          // well-formed runs
errs := klf.ValidateTargetAgainstSource(block, target)    // placeholder preservation
```

Returns typed `ValidationError` with machine-readable `Kind`
constants. Covers missing placeholders, unclosed / unmatched /
duplicate paired codes, and malformed runs.

### Preview (`preview.go`)

```go
html := klf.RenderBlockHTML(block, klf.DefaultJSXVocabulary())
```

Reference Level-1 preview renderer that walks a block's Runs and
emits the same `<kat-block>` HTML envelope neokapi's existing HTML
and Markdown preview builders produce. Byte-for-byte compatible with
`packages/format/src/preview.ts`'s `renderBlockHtml`.

### Annotations (`annotation.go`)

```go
file, err := klf.DecodeAnnotationFile(reader)  // parse .klfl JSON-Lines
err := klf.EncodeAnnotationFile(writer, file)  // emit .klfl JSON-Lines

res := klf.ResolveAnchor(block, anchor)        // resolve an annotation anchor
ve  := klf.ValidateAnchor(block, annotation)   // orphan-check
```

Four anchor kinds — `AnchorBlock`, `AnchorRun`, `AnchorRange`,
`AnchorForm` — cover block-level metadata, run-scoped metadata,
character-range hits inside a text run, and per-plural-form /
per-select-case metadata. `ResolveAnchor` returns one of six
machine-readable failure reasons (`ReasonBlockNotFound`,
`ReasonPathOutOfBounds`, `ReasonPathWrongKind`, `ReasonRunIDMismatch`,
`ReasonRangeOutOfBounds`, `ReasonFormNotFound`), matching
`packages/format/src/annotation.ts` exactly.

## Relationship to packages/format

The package mirrors the TypeScript reference in
[`packages/format/`](../../packages/format/). Drift is prevented by
shared golden fixtures: every Go test that round-trips a block also
renders it through `RenderBlockHTML` and compares against the
hand-computed expected HTML from `packages/format/examples/*`. Any
schema change must land on both sides in the same PR.

## See also

- [AD-044: KLF / KLZ Format Integration](../../docs/ad/044-klf-klz-integration.md)
- [AD-045: KLF / KLZ Format Specification](../../docs/ad/045-klf-klz-spec.md)
- [`core/klz`](../klz/) — .klz archive reader/writer
- [`core/formats/jsx`](../formats/jsx/) — DataFormatReader / Writer
  + PreviewBuilder that wraps this package
