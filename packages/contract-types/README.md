# @neokapi/contract-types

Shared TypeScript contract types for the [neokapi](https://github.com/neokapi/neokapi)
engine: the single source of truth for the flow/tool IO contract, the schema
language, the content-model payload shapes and the review model consumed by
every neokapi frontend package (including `@neokapi/engine`).

Four layers, re-exported from the package root:

- **`./contract.gen`** — IO-contract atoms generated from the Go sources of
  truth (`core/schema`, `core/format/schema`, `core/model`): tool/format
  metadata, categories, IO ports, overlay and annotation vocabularies.
- **`./content.gen`** — content-model types generated from the canonical
  `neokapi.content.v1` proto descriptors (wire shapes, as encoded by protojson)
  and the Go projection structs (`model.Run` JSON, the `ContentTree` family).
- **`./review.gen`** — the review model (`core/review`): the five layers a
  review decision is made in, read by every review client and taken as props
  by the shared review cards.
- **`./manual`** — hand-authored superset envelope types the UI extends beyond
  Go (`ComponentSchema`, `PropertySchema`, `ConditionExpr`, `ToolDoc`, …).

## Generated package — do not edit

The `*.gen.ts` sources are **generated** from the Go definitions in the neokapi
repository and must not be edited by hand. Regenerate with:

```bash
make generate-contract-types
```

CI enforces a drift gate (`make check-contract-types`): the committed types are
regenerated and compared against Go on every change, so the published package
cannot drift from the engine.

## Usage

```ts
import type { ToolMeta, ContentTree, Run } from "@neokapi/contract-types";
```

In-repo consumers resolve the TypeScript source directly; the published package
ships transpiled ESM plus declarations from `dist/` (see the `exports` vs
`publishConfig` split in `package.json`).

## License

Apache-2.0
