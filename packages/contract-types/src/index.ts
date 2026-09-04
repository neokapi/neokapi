// @neokapi/contract-types — the single source of truth for the flow/tool IO
// contract and schema-language types shared across every frontend package
// (issue #817). Apache-2.0; safe for both the Apache `packages/*` zone and the
// AGPL `bowrain/*` zone to import.
//
// Four layers:
//   - ./contract.gen — IO-contract atoms generated from Go (core/schema,
//     core/format/schema, core/model). DO NOT EDIT; regenerate with
//     `make generate-contract-types`.
//   - ./content.gen — content-model types generated from the canonical
//     neokapi.content.v1 proto descriptors and the Go projection structs
//     (AD-034). DO NOT EDIT; regenerate with `make generate-contract-types`.
//   - ./review.gen — the review model (S-07) generated from core/review and
//     the structs it carries, read by every review client and by the shared
//     review cards. DO NOT EDIT; regenerate with `make generate-contract-types`.
//   - ./manual — the superset envelope types (ComponentSchema, PropertySchema,
//     ConditionExpr, ToolDoc, ToolDocParam) the UI extends beyond Go.

export * from "./contract.gen.ts";
export * from "./content.gen.ts";
export * from "./review.gen.ts";
export * from "./manual.ts";
