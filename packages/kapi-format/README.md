# @neokapi/kapi-format — canonical Block/Run schema

TypeScript port of the **Kapi Bundle Format (KBF)** content
model. Paired with `core/kbf` in Go so both languages round-trip the
same bytes through the same shared golden fixtures (`examples/`).

This package is the home of:

- The canonical `Block` / `Edition` / `Run` / `Placeholder` /
  `ExtractedDocument` types ([`src/block.ts`](src/block.ts))
- The edition accessors and the reader for either schema
  ([`src/editions.ts`](src/editions.ts))
- The deterministic serializer ([`src/kbf.ts`](src/kbf.ts))
- The JSX vocabulary + span-type template expander
  ([`src/vocabulary.ts`](src/vocabulary.ts))
- The Level-1 preview renderer + target validator
  ([`src/preview.ts`](src/preview.ts))
- The annotation overlay resolver + orphan validator
  ([`src/annotation.ts`](src/annotation.ts))

## Editions and the two schemas

A block holds its content as peer editions under their edition keys: the
edition it was read in under `SourceEdition` (the empty key), every
translation and every tone or channel edition under its own key (`nb`,
`fr;tone=formal`, `en;channel=short`). Each edition carries its `runs` and,
where recorded, its `status`, `origin`, `score` and `derived` (the edition it
was made from and that edition's revision). Read them through `sourceRuns`,
`editionRuns` and `targetKeys` rather than through the map.

```ts
import { readFileSync, writeFileSync } from "node:fs";
import { editionRuns, flattenRuns, marshalFile, parseFile, sourceRuns } from "@neokapi/kapi-format";

const file = parseFile(readFileSync("i18n-nb/page.kbf.json", "utf-8"));
for (const block of file.documents[0].blocks) {
  console.log(flattenRuns(sourceRuns(block)), "→", flattenRuns(editionRuns(block, "nb") ?? []));
}
writeFileSync("out.kbf.json", marshalFile(file)); // schema 2.0
```

`parseFile` reads schema 2.0 and schema 1.0, whose blocks carried `source`
runs beside `targets` keyed by locale, and returns either as editions;
`marshalFile` writes 2.0. Go `core/kbf` reads and writes the same way, and the
tests in `tests/editions.test.ts` hold the two to the same bytes over the Go
format fixtures.

## Running the examples

```bash
node --experimental-strip-types examples/validate.ts
```

Renders each example Block to HTML and diffs against hand-computed
expected output, runs four content-validator scenarios
(valid / missing-required / optional-drop / plural-preserves-pivot),
and resolves eight annotation anchors (four real + four synthetic
orphans). Exits non-zero on any mismatch.

The Go side — `core/kbf` — loads the same fixtures via
`go:embed` in `core/kbf/fixtures_test.go` and renders them through
`RenderBlockHTML`. The byte output of both implementations must
match.

## Relationship to `core/kbf` (Go)

| Layer            | TypeScript (this package)                     | Go (`core/kbf`)                                                 |
| ---------------- | --------------------------------------------- | --------------------------------------------------------------- |
| Types            | `src/block.ts`                                | `core/kbf/schema.go`                                            |
| Editions, reader | `src/editions.ts`                             | `core/kbf/edition.go`, `core/kbf/reader.go`                     |
| Vocabulary       | `src/vocabulary.ts`                           | `core/model/vocabularies/rich-jsx.json` + `core/kbf/preview.go` |
| Preview renderer | `src/preview.ts::renderBlockHtml`             | `core/kbf/preview.go::RenderBlockHTML`                          |
| Validator        | `src/preview.ts::validateTargetAgainstSource` | `core/kbf/validator.go`                                         |
| Annotations      | `src/annotation.ts`                           | `core/kbf/annotation.go`                                        |

Any schema change must land in both languages in the same PR, with
the golden fixtures in `examples/` updated accordingly. The fixtures
are the contract.

## Files

| File                        | Purpose                                                                                     |
| --------------------------- | ------------------------------------------------------------------------------------------- |
| `src/block.ts`              | Core types: Block, Edition, Run (including structured plural/select), Placeholder, ExtractedDocument |
| `src/editions.ts`           | Edition accessors, `parseFile` and `upgradeBlock` for schema 1.0 and 2.0                    |
| `src/kbf.ts`                | Deterministic serializer (`marshalFile`, `marshalBlock`)                                    |
| `src/vocabulary.ts`         | Vocabulary entries + template expander + default JSX vocabulary                             |
| `src/preview.ts`            | Level-1 Run renderer + target validator                                                     |
| `src/annotation.ts`         | Annotation overlay types + anchor resolution + orphan validator                             |
| `src/index.ts`              | Re-exports                                                                                  |
| `examples/files-heading.ts` | Nested inline `<span>` + `{count}` variable                                                 |
| `examples/tag-chip.ts`      | Three conditional JSX placeholders + `optional` flag                                        |
| `examples/shopping-cart.ts` | `<Plural>` component → multi-segment block                                                  |
| `examples/annotations.ts`   | Example annotation file covering all four anchor shapes                                     |
| `examples/validate.ts`      | Renderer + content validator + anchor-resolution tests                                      |
