# @neokapi/kapi-format — canonical Block/Run schema

TypeScript port of the **Kapi Localization Format (KLF)** content
model. Paired with `core/klf` in Go so both languages round-trip the
same bytes through the same shared golden fixtures (`examples/`).

This package is the home of:

- The canonical `Block` / `Run` / `Placeholder` / `ExtractedDocument`
  types ([`src/block.ts`](src/block.ts))
- The JSX vocabulary + span-type template expander
  ([`src/vocabulary.ts`](src/vocabulary.ts))
- The Level-1 preview renderer + target validator
  ([`src/preview.ts`](src/preview.ts))
- The annotation overlay resolver + orphan validator
  ([`src/annotation.ts`](src/annotation.ts))

## Running the examples

```bash
node --experimental-strip-types examples/validate.ts
```

Renders each example Block to HTML and diffs against hand-computed
expected output, runs four content-validator scenarios
(valid / missing-required / optional-drop / plural-preserves-pivot),
and resolves eight annotation anchors (four real + four synthetic
orphans). Exits non-zero on any mismatch.

The Go side — `core/klf` — loads the same fixtures via
`go:embed` in `core/klf/fixtures_test.go` and renders them through
`RenderBlockHTML`. The byte output of both implementations must
match.

## Relationship to `core/klf` (Go)

| Layer            | TypeScript (this package)       | Go (`core/klf`)                        |
| ---------------- | ------------------------------- | -------------------------------------- |
| Types            | `src/block.ts`                  | `core/klf/schema.go`                   |
| Vocabulary       | `src/vocabulary.ts`             | `core/model/vocabularies/rich-jsx.json` + `core/klf/preview.go` |
| Preview renderer | `src/preview.ts::renderBlockHtml` | `core/klf/preview.go::RenderBlockHTML` |
| Validator        | `src/preview.ts::validateTargetAgainstSource` | `core/klf/validator.go` |
| Annotations      | `src/annotation.ts`             | `core/klf/annotation.go`               |

Any schema change must land in both languages in the same PR, with
the golden fixtures in `examples/` updated accordingly. The fixtures
are the contract.

## Files

| File                          | Purpose                                                                             |
| ----------------------------- | ----------------------------------------------------------------------------------- |
| `src/block.ts`                | Core types: Block, Run (including structured plural/select), Placeholder, ExtractedDocument |
| `src/vocabulary.ts`           | Vocabulary entries + template expander + default JSX vocabulary                     |
| `src/preview.ts`              | Level-1 Run renderer + target validator                                             |
| `src/annotation.ts`           | Annotation overlay types + anchor resolution + orphan validator                     |
| `src/index.ts`                | Re-exports                                                                          |
| `examples/files-heading.ts`   | Nested inline `<span>` + `{count}` variable                                         |
| `examples/tag-chip.ts`        | Three conditional JSX placeholders + `optional` flag                                |
| `examples/shopping-cart.ts`   | `<Plural>` component → multi-segment block                                          |
| `examples/annotations.ts`     | Example annotation file covering all four anchor shapes                             |
| `examples/validate.ts`        | Renderer + content validator + anchor-resolution tests                              |
