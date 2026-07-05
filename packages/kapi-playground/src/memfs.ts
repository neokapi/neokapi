// The in-memory filesystem moved to @neokapi/engine (packages/engine/src/
// memfs.ts) with the rest of the wasm boot path. This re-export keeps the
// playground's historical `@neokapi/kapi-playground/memfs` subpath stable.

export * from "@neokapi/engine/memfs";
