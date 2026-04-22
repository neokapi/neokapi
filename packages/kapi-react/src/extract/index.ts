/**
 * Public extraction API for @neokapi/kapi-react.
 *
 * Consumers:
 *   - CLI (`kapi-react extract`) walks a glob, calls `extractDocument`
 *     per file, and writes one `.klf` per source document via
 *     @neokapi/kapi-format.
 *   - Tests call `extractDocument` with a single-source fixture and
 *     assert on `Block[]` / `Run[]` structure.
 *
 * Everything that was exported from the legacy `src/extract.ts` has
 * moved here. The runtime contract (hash values, placeholder names)
 * stays identical so this module's Block.hash lines up with whatever
 * the plugin transform stamps into `__t()` / `__tx()` at build time.
 */

export { extractDocument } from "./walker.ts";
export type { ExtractOptions, WalkerOptions } from "./walker.ts";
export { createWarningCollector, formatWarning } from "./warnings.ts";
export type { Warning, WarningCollector, WarningKind } from "./warnings.ts";
