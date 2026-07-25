# core/kbf

Go implementation of the canonical Block / Run model — the Kapi
Bundle Format. Paired with
[`packages/kapi-format/`](../../packages/kapi-format/) (`@neokapi/kapi-format`) —
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
- `File` — top-level `.kbf.json` JSON envelope.

### JSON reader/writer

```go
file, err := kbf.Unmarshal(data)        // parse .kbf.json JSON
data, err := kbf.Marshal(file)          // deterministic .kbf.json JSON
```

The writer is deterministic (2-space indent, no HTML escaping,
trailing newline) — the byte output is stable for hashing and
git diffing.

### Validator (`validator.go`)

```go
errs := kbf.ValidateBlock(block)                          // well-formed runs
errs := kbf.ValidateTargetAgainstSource(block, target)    // placeholder preservation
```

Returns typed `ValidationError` with machine-readable `Kind`
constants. Covers missing placeholders, unclosed / unmatched /
duplicate paired codes, and malformed runs.

### Preview (`preview.go`)

```go
html := kbf.RenderBlockHTML(block, kbf.DefaultJSXVocabulary())
```

Reference Level-1 preview renderer that walks a block's Runs and
emits the same `<kat-block>` HTML envelope neokapi's existing HTML
and Markdown preview builders produce. Byte-for-byte compatible with
`packages/kapi-format/src/preview.ts`'s `renderBlockHtml`.

### Annotations (`annotation.go`)

```go
file, err := kbf.DecodeAnnotationFile(reader)  // parse .overlays.jsonl JSON-Lines
err := kbf.EncodeAnnotationFile(writer, file)  // emit .overlays.jsonl JSON-Lines

res := kbf.ResolveAnchor(block, anchor)        // resolve an annotation anchor
ve  := kbf.ValidateAnchor(block, annotation)   // orphan-check
```

Four anchor kinds — `AnchorBlock`, `AnchorRun`, `AnchorRange`,
`AnchorForm` — cover block-level metadata, run-scoped metadata,
character-range hits inside a text run, and per-plural-form /
per-select-case metadata. `ResolveAnchor` returns one of six
machine-readable failure reasons (`ReasonBlockNotFound`,
`ReasonPathOutOfBounds`, `ReasonPathWrongKind`, `ReasonRunIDMismatch`,
`ReasonRangeOutOfBounds`, `ReasonFormNotFound`), matching
`packages/kapi-format/src/annotation.ts` exactly.

## Relationship to packages/kapi-format

The package mirrors the TypeScript reference in
[`packages/kapi-format/`](../../packages/kapi-format/). Drift is prevented by
shared golden fixtures: every Go test that round-trips a block also
renders it through `RenderBlockHTML` and compares against the
hand-computed expected HTML from `packages/kapi-format/examples/*`. Any
schema change must land on both sides in the same PR.

## See also

- [`core/formats/jsx`](../formats/jsx/) — DataFormatReader / Writer
  + PreviewBuilder that wraps this package.
- [`core/blockstore`](../blockstore/) — the block-addressed store
  that a kapi project persists KBF-shaped blocks into.
