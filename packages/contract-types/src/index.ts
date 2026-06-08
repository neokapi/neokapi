// @neokapi/contract-types — the single source of truth for the flow/tool IO
// contract and schema-language types shared across every frontend package
// (issue #817). Apache-2.0; safe for both the Apache `packages/*` zone and the
// AGPL `bowrain/*` zone to import.
//
// Two layers:
//   - ./contract.gen — IO-contract atoms generated from Go (core/schema,
//     core/format/schema, core/model). DO NOT EDIT; regenerate with
//     `make generate-contract-types`.
//   - ./manual — the superset envelope types (ComponentSchema, PropertySchema,
//     ConditionExpr, ToolDoc, ToolDocParam) the UI extends beyond Go.

export * from "./contract.gen";
export * from "./manual";
