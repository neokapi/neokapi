module github.com/neokapi/neokapi/scripts/gen-contract-types

go 1.26.0

// gen-contract-types emits the shared TypeScript contract types
// (packages/contract-types) from the framework's Go source of truth
// (core/schema, core/format/schema, core/model). It needs only the framework
// module — no cli/Cobra — so it lives in its own module like the other
// scripts/* generators. Local modules resolve via go.work.
require github.com/neokapi/neokapi v0.0.0-00010101000000-000000000000
