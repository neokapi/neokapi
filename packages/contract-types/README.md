# @neokapi/contract-types

Shared TypeScript contract types for the [neokapi](https://github.com/neokapi/neokapi)
engine: the single source of truth for the flow/tool IO contract, the schema
language, the content-model payload shapes, the review model and the change
contract, consumed by every neokapi frontend package (including `@neokapi/engine`).

Five layers, re-exported from the package root:

- **`./contract.gen`**: IO-contract atoms generated from the Go sources of
  truth (`core/schema`, `core/format/schema`, `core/model`): tool/format
  metadata, categories, IO ports, overlay and annotation vocabularies.
- **`./content.gen`**: content-model types generated from the canonical
  `neokapi.content.v1` proto descriptors (wire shapes, as encoded by protojson)
  and the Go projection structs (`model.Run` JSON, the `ContentTree` family).
- **`./review.gen`**: the review model (`core/review`): the five layers a
  review decision is made in, read by every review client and taken as props
  by the shared review cards.
- **`./change.gen`**: the change contract (`core/change`). `ChangeSet` and the
  `ChangeOp` union are rendered from the JSON Schema of `kapi.change/v1`, so
  they refuse the structural mistakes the decoder refuses: an operation is
  discriminated by `op`, a choice of exactly one field (`text` or `runs`;
  `find`, `start` with `end`, or `range`) refuses both, a run holds one kind
  (text, one code, a plural or a select), and a runs payload carries no native
  `data`. A pattern, a bound or a rule between fields (a revision's form,
  `occurrence` only beside `find`, at least one operation) is documented on
  the field and checked by the service. `ChangeResult`
  (`kapi.change-result/v1`), the read page (`ReadPage`, `BlockRead`), a
  format's description (`FormatDescription`) and an edition's recorded changes
  (`EditionHistory`, `HistoryEntry`) are reflected from the Go structs the
  service marshals; a change set refused as a whole is a `ChangeResult`
  whose `error` says why, with empty `docs` and `ops`. The unions of
  operation kinds, statuses and error codes come with frozen lists, and
  `CHANGE_ERROR_HTTP_STATUS` and `CHANGE_ERROR_EXIT_CODE` map each error code
  as the transports do.
- **`./manual`**: hand-authored superset envelope types the UI extends beyond
  Go (`ComponentSchema`, `PropertySchema`, `ConditionExpr`, …).

## Generated package: do not edit

The `*.gen.ts` sources are **generated** from the Go definitions in the neokapi
repository and must not be edited by hand. Regenerate with:

```bash
make generate-contract-types
```

CI enforces a drift gate (`make check-contract-types`): the committed types are
regenerated and compared against Go on every change, so the published package
cannot drift from the engine. `make test-contract-types` runs the generator's
tests, compiles the package with `tsc` (`vp run typecheck:generated`; the
workspace's `vp check` skips `*.gen.ts`) and type-checks `tests/`, where each
structural mistake the service refuses is asserted to fail compilation.

## Usage

```ts
import type { ToolMeta, ContentTree, Run } from "@neokapi/contract-types";
```

A read hands an operation the reference and revision it needs:

```ts
import type { BlockRead, ChangeSet, ChangeResult } from "@neokapi/contract-types";

function retitle(read: BlockRead, text: string): ChangeSet {
  return { ops: [{ op: "set_content", at: read.ref, if_match: read.rev, text }] };
}

function landed(result: ChangeResult): boolean {
  return result.status === "applied";
}

function why(result: ChangeResult): string | undefined {
  return result.error?.message ?? result.ops.find((op) => op.error)?.error?.message;
}
```

In-repo consumers resolve the TypeScript source directly; the published package
ships transpiled ESM plus declarations from `dist/` (see the `exports` vs
`publishConfig` split in `package.json`).

## License

Apache-2.0
